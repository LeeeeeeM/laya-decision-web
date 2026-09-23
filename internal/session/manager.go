package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
	"github.com/LeeeeeeM/laya-decision-web/internal/snake"
)

type Status string

const (
	StatusRunning  Status = "running"
	StatusPaused   Status = "paused"
	StatusCooldown Status = "cooldown"
	StatusFinished Status = "finished"
	StatusStopped  Status = "stopped"
	StatusError    Status = "error"
)

type Event struct {
	ID    string          `json:"id"`
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data"`
	Index int64           `json:"-"`
}

type CreateRequest struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model,omitempty"`
	FPS      float64 `json:"fps"`
	Seed     int64   `json:"seed"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Prompt   string  `json:"prompt"`
}

type ControlRequest struct {
	Action string  `json:"action"`
	FPS    float64 `json:"fps,omitempty"`
	Seed   *int64  `json:"seed,omitempty"`
}

type SnapshotPayload struct {
	SessionID string         `json:"session_id"`
	Status    Status         `json:"status"`
	Provider  string         `json:"provider"`
	Model     string         `json:"model"`
	FPS       float64        `json:"fps"`
	Prompt    string         `json:"prompt"`
	Game      snake.Snapshot `json:"game"`
}

type retryAfterer interface {
	GetRetryAfter() time.Duration
}

type Session struct {
	ID       string
	Provider decision.Provider
	Model    string
	FPS      float64
	Prompt   string
	MaxFPS   float64

	mu            sync.Mutex
	game          *snake.Game
	status        Status
	seed          int64
	width         int
	height        int
	lastDecision  *snake.DecisionResult
	cooldownUntil time.Time
	lastError     string
	eventSeq      int64
	events        []Event
	subs          map[chan Event]struct{}
	cancel        context.CancelFunc
	stopped       bool
}

type Manager struct {
	mu        sync.Mutex
	sessions  map[string]*Session
	providers map[string]decision.Provider
	maxJevFPS float64
}

func NewManager(providers map[string]decision.Provider, maxJevFPS float64) *Manager {
	return &Manager{
		sessions:  make(map[string]*Session),
		providers: providers,
		maxJevFPS: maxJevFPS,
	}
}

func (m *Manager) Capabilities() decision.Capabilities {
	caps := decision.Capabilities{}
	order := []string{decision.ProviderMock, decision.ProviderBochaJev, decision.ProviderLaya}
	seen := map[string]bool{}
	for _, id := range order {
		p, ok := m.providers[id]
		if !ok {
			continue
		}
		seen[id] = true
		info := decision.ProviderInfo{ID: id, Available: p.Available()}
		switch id {
		case decision.ProviderBochaJev:
			info.Model = "bocha-jev-v1"
		case decision.ProviderMock:
			info.Model = "mock-heuristic-v1"
		case decision.ProviderLaya:
			if lp, ok := p.(interface{ ModelInfo() decision.ModelInfo }); ok {
				info.Models = []decision.ModelInfo{lp.ModelInfo()}
			} else {
				info.Models = []decision.ModelInfo{}
			}
		}
		caps.Providers = append(caps.Providers, info)
	}
	for id, p := range m.providers {
		if seen[id] {
			continue
		}
		caps.Providers = append(caps.Providers, decision.ProviderInfo{ID: id, Available: p.Available()})
	}
	return caps
}

func (m *Manager) GetProvider(name string) (decision.Provider, bool) {
	p, ok := m.providers[name]
	return p, ok
}

func (m *Manager) Create(req CreateRequest) (*Session, error) {
	provider, ok := m.providers[req.Provider]
	if !ok || !provider.Available() {
		return nil, fmt.Errorf("provider unavailable")
	}
	width, height := req.Width, req.Height
	if width == 0 {
		width = 24
	}
	if height == 0 {
		height = 16
	}
	seed := req.Seed
	if seed == 0 {
		seed = 7
	}
	fps := req.FPS
	if fps <= 0 {
		if req.Provider == decision.ProviderBochaJev {
			fps = 1
		} else {
			fps = 8
		}
	}
	if req.Provider == decision.ProviderBochaJev && m.maxJevFPS > 0 && fps > m.maxJevFPS {
		return nil, fmt.Errorf("fps exceeds BOCHA_JEV_MAX_FPS")
	}
	prompt := req.Prompt
	if prompt == "" {
		prompt = "compact"
	}
	if prompt != "compact" && prompt != "detailed" {
		return nil, fmt.Errorf("prompt must be compact or detailed")
	}
	game, err := snake.NewGame(width, height, seed, 6)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("sess_%d", time.Now().UnixNano())
	runCtx, cancel := context.WithCancel(context.Background())
	model := req.Model
	if model == "" {
		if lp, ok := provider.(interface{ ModelInfo() decision.ModelInfo }); ok {
			model = lp.ModelInfo().ID
		}
	}
	s := &Session{
		ID:       id,
		Provider: provider,
		Model:    model,
		FPS:      fps,
		Prompt:   prompt,
		MaxFPS:   m.maxJevFPS,
		game:     game,
		status:   StatusRunning,
		seed:     seed,
		width:    width,
		height:   height,
		subs:     make(map[chan Event]struct{}),
		cancel:   cancel,
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	s.emit("status", map[string]any{"status": s.status})
	s.emit("snapshot", s.Snapshot())
	go s.loop(runCtx)
	return s, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) Delete(id string) bool {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return false
	}
	s.Stop()
	return true
}

func (s *Session) snapshotLocked() SnapshotPayload {
	model := s.Model
	if model == "" {
		switch s.Provider.Name() {
		case decision.ProviderBochaJev:
			model = "bocha-jev-v1"
		case decision.ProviderMock:
			model = "mock-heuristic-v1"
		default:
			model = s.Provider.Name()
		}
	}
	return SnapshotPayload{
		SessionID: s.ID,
		Status:    s.status,
		Provider:  s.Provider.Name(),
		Model:     model,
		FPS:       s.FPS,
		Prompt:    s.Prompt,
		Game:      s.game.Snapshot(),
	}
}

func (s *Session) Snapshot() SnapshotPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Session) Control(req ControlRequest) (SnapshotPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return SnapshotPayload{}, fmt.Errorf("session stopped")
	}
	switch req.Action {
	case "pause":
		if s.status == StatusRunning || s.status == StatusCooldown {
			s.status = StatusPaused
		}
	case "resume":
		if s.status == StatusPaused || s.status == StatusError || s.status == StatusCooldown {
			s.status = StatusRunning
			s.cooldownUntil = time.Time{}
			s.lastError = ""
		}
	case "set_speed":
		if req.FPS <= 0 || req.FPS > 60 {
			return SnapshotPayload{}, fmt.Errorf("fps out of range")
		}
		if s.Provider.Name() == decision.ProviderBochaJev && s.MaxFPS > 0 && req.FPS > s.MaxFPS {
			return SnapshotPayload{}, fmt.Errorf("fps exceeds BOCHA_JEV_MAX_FPS")
		}
		s.FPS = req.FPS
	case "reset":
		seed := s.seed
		if req.Seed != nil {
			seed = *req.Seed
		}
		game, err := snake.NewGame(s.width, s.height, seed, 6)
		if err != nil {
			return SnapshotPayload{}, err
		}
		s.game = game
		s.seed = seed
		s.status = StatusRunning
		s.lastDecision = nil
		s.cooldownUntil = time.Time{}
		s.lastError = ""
	case "stop":
		s.status = StatusStopped
		s.stopped = true
		if s.cancel != nil {
			s.cancel()
		}
	default:
		return SnapshotPayload{}, fmt.Errorf("unknown action")
	}
	snap := s.snapshotLocked()
	s.emitLocked("status", map[string]any{"status": s.status, "fps": s.FPS})
	s.emitLocked("snapshot", snap)
	return snap, nil
}

func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	s.status = StatusStopped
	if s.cancel != nil {
		s.cancel()
	}
	for ch := range s.subs {
		close(ch)
		delete(s.subs, ch)
	}
}

func (s *Session) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 16 {
		buffer = 16
	}
	ch := make(chan Event, buffer)
	s.mu.Lock()
	snap := s.snapshotLocked()
	s.subs[ch] = struct{}{}
	payload, _ := json.Marshal(snap)
	s.eventSeq++
	ev := Event{ID: fmt.Sprintf("%d", s.eventSeq), Type: "snapshot", Data: payload, Index: s.eventSeq}
	s.mu.Unlock()
	ch <- ev
	unsub := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
	}
	return ch, unsub
}

func (s *Session) emit(typ string, data any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitLocked(typ, data)
}

func (s *Session) emitLocked(typ string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	s.eventSeq++
	ev := Event{ID: fmt.Sprintf("%d", s.eventSeq), Type: typ, Data: payload, Index: s.eventSeq}
	s.events = append(s.events, ev)
	if len(s.events) > 200 {
		s.events = s.events[len(s.events)-200:]
	}
	for ch := range s.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (s *Session) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.mu.Lock()
		status := s.status
		fps := s.FPS
		cooldown := s.cooldownUntil
		stopped := s.stopped
		alive := s.game.Alive && !s.game.Won
		s.mu.Unlock()

		if stopped {
			return
		}
		if status == StatusFinished || !alive {
			s.mu.Lock()
			if s.status != StatusFinished && s.status != StatusStopped {
				s.status = StatusFinished
				s.emitLocked("status", map[string]any{"status": s.status})
				s.emitLocked("snapshot", s.snapshotLocked())
			}
			s.mu.Unlock()
			if sleepOrDone(ctx, 200*time.Millisecond) {
				return
			}
			continue
		}
		if status == StatusPaused || status == StatusError {
			if sleepOrDone(ctx, 100*time.Millisecond) {
				return
			}
			continue
		}
		if status == StatusCooldown {
			if time.Now().Before(cooldown) {
				if sleepOrDone(ctx, 100*time.Millisecond) {
					return
				}
				continue
			}
			s.mu.Lock()
			s.status = StatusRunning
			s.emitLocked("status", map[string]any{"status": s.status})
			s.mu.Unlock()
		}

		stepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.stepOnce(stepCtx)
		cancel()
		if err != nil {
			s.handleStepError(err)
		}

		interval := time.Second
		if fps > 0 {
			interval = time.Duration(float64(time.Second) / fps)
		}
		if sleepOrDone(ctx, interval) {
			return
		}
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-timer.C:
		return false
	}
}

func (s *Session) stepOnce(ctx context.Context) error {
	s.mu.Lock()
	if s.status != StatusRunning || s.stopped || !s.game.Alive || s.game.Won {
		s.mu.Unlock()
		return nil
	}
	game := s.game
	policy := &snake.Policy{Provider: s.Provider, Prompt: s.Prompt}
	s.mu.Unlock()

	decisionResult, err := policy.Decide(ctx, game)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != StatusRunning || s.stopped || s.game != game {
		return nil
	}
	if _, err := game.Step(decisionResult.Executed); err != nil {
		return err
	}
	s.lastDecision = &decisionResult
	s.emitLocked("decision", decisionResult)
	s.emitLocked("snapshot", s.snapshotLocked())
	if !s.game.Alive || s.game.Won {
		s.status = StatusFinished
		s.emitLocked("status", map[string]any{"status": s.status})
	}
	return nil
}

func (s *Session) handleStepError(err error) {
	retryAfter := time.Duration(0)
	code := "provider_error"
	if ra, ok := err.(retryAfterer); ok {
		retryAfter = ra.GetRetryAfter()
		code = "provider_rate_limited"
	}
	msg := err.Error()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = msg
	if retryAfter > 0 {
		s.status = StatusCooldown
		s.cooldownUntil = time.Now().Add(retryAfter)
		s.emitLocked("error", map[string]any{
			"code":                code,
			"message":             msg,
			"retry_after_seconds": retryAfter.Seconds(),
		})
		s.emitLocked("status", map[string]any{"status": s.status, "retry_after_seconds": retryAfter.Seconds()})
		return
	}
	s.status = StatusError
	s.emitLocked("error", map[string]any{"code": code, "message": msg})
	s.emitLocked("status", map[string]any{"status": s.status})
}
