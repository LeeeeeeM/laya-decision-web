package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
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
	Provider   string  `json:"provider"`
	Seed       int64   `json:"seed"`
	DecisionHz float64 `json:"decision_hz"`
	Mode       string  `json:"mode"` // human | agent
}

type ControlRequest struct {
	Action string `json:"action"` // pause|resume|reset|stop|input
	Seed   *int64 `json:"seed,omitempty"`
	Move   string `json:"move,omitempty"`
	Stance string `json:"stance,omitempty"` // NONE|JUMP|CROUCH when action=input
	Keys   *Keys  `json:"keys,omitempty"`
}

type Keys struct {
	Left   bool `json:"left"`
	Right  bool `json:"right"`
	Jump   bool `json:"jump"`
	Crouch bool `json:"crouch"`
}

type SessionSnapshot struct {
	SessionID  string   `json:"session_id"`
	Status     Status   `json:"status"`
	Provider   string   `json:"provider"`
	Model      string   `json:"model"`
	DecisionHz float64  `json:"decision_hz"`
	Mode       string   `json:"mode"`
	Game       Snapshot `json:"game"`
}

type decisionResult struct {
	generation uint64
	tick       int64
	decision   DecisionResult
}

type retryAfterer interface {
	GetRetryAfter() time.Duration
}

type Session struct {
	ID         string
	Provider   decision.Provider
	Model      string
	DecisionHz float64
	Mode       string

	mu             sync.Mutex
	game           *Game
	status         Status
	seed           int64
	lastDecision   *DecisionResult
	cooldownUntil  time.Time
	lastError      string
	eventSeq       int64
	events         []Event
	subs           map[chan Event]struct{}
	cancel         context.CancelFunc
	stopped        bool
	keys           Keys
	generation     uint64
	latestDecision *decisionResult
}

type Manager struct {
	mu        sync.Mutex
	sessions  map[string]*Session
	providers map[string]decision.Provider
	maxJevHz  float64
}

func NewManager(providers map[string]decision.Provider, maxJevHz float64) *Manager {
	if maxJevHz <= 0 {
		maxJevHz = 2
	}
	return &Manager{sessions: map[string]*Session{}, providers: providers, maxJevHz: maxJevHz}
}

func (m *Manager) Create(req CreateRequest) (*Session, error) {
	provider, ok := m.providers[req.Provider]
	if !ok || !provider.Available() {
		return nil, fmt.Errorf("provider unavailable")
	}
	mode := req.Mode
	if mode == "" {
		mode = "agent"
	}
	if mode != "human" && mode != "agent" {
		return nil, fmt.Errorf("mode must be human or agent")
	}
	hz := req.DecisionHz
	if hz <= 0 {
		if req.Provider == decision.ProviderBochaJev {
			hz = 1
		} else {
			hz = DefaultDHz
		}
	}
	if mode == "agent" {
		if req.Provider == decision.ProviderBochaJev && hz > m.maxJevHz {
			return nil, fmt.Errorf("decision_hz exceeds limit for jev")
		}
		if req.Provider != decision.ProviderBochaJev {
			if hz < 4 {
				hz = 4
			}
			if hz > 10 {
				hz = 10
			}
		}
	}
	seed := req.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	game := NewGame(seed)
	id := fmt.Sprintf("plat_%d", time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	model := req.Provider
	if lp, ok := provider.(interface{ ModelInfo() decision.ModelInfo }); ok {
		model = lp.ModelInfo().ID
	}
	s := &Session{
		ID: id, Provider: provider, Model: model, DecisionHz: hz, Mode: mode,
		game: game, status: StatusRunning, seed: seed, subs: map[chan Event]struct{}{}, cancel: cancel,
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	s.emit("status", map[string]any{"status": s.status})
	s.emit("snapshot", s.Snapshot())
	go s.loop(ctx)
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

func (s *Session) Snapshot() SessionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Session) snapshotLocked() SessionSnapshot {
	return SessionSnapshot{
		SessionID: s.ID, Status: s.status, Provider: s.Provider.Name(), Model: s.Model,
		DecisionHz: s.DecisionHz, Mode: s.Mode, Game: s.game.Snapshot(),
	}
}

func (s *Session) Control(req ControlRequest) (SessionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return SessionSnapshot{}, fmt.Errorf("session stopped")
	}
	switch req.Action {
	case "pause":
		s.status = StatusPaused
		s.latestDecision = nil
	case "resume":
		s.status = StatusRunning
		s.cooldownUntil = time.Time{}
		s.latestDecision = nil
	case "reset":
		seed := s.seed
		if req.Seed != nil {
			seed = *req.Seed
		}
		s.game = NewGame(seed)
		s.seed = seed
		s.status = StatusRunning
		s.lastDecision = nil
		s.latestDecision = nil
		s.generation++
	case "stop":
		s.status = StatusStopped
		s.stopped = true
		s.generation++
		s.latestDecision = nil
		if s.cancel != nil {
			s.cancel()
		}
	case "input":
		if req.Keys != nil {
			s.keys = *req.Keys
			s.applyKeysLocked()
		} else if req.Move != "" || req.Stance != "" {
			s.game.SetIntent(MoveIntent(req.Move), ActionIntent(req.Stance))
		}
	default:
		return SessionSnapshot{}, fmt.Errorf("unknown action")
	}
	snap := s.snapshotLocked()
	s.emitLocked("status", map[string]any{"status": s.status})
	s.emitLocked("snapshot", snap)
	return snap, nil
}

func (s *Session) applyKeysLocked() {
	move := MoveIdle
	if s.keys.Left && !s.keys.Right {
		move = MoveLeft
	} else if s.keys.Right && !s.keys.Left {
		move = MoveRight
	}
	action := ActionNone
	if s.keys.Crouch {
		action = ActionCrouch
	} else if s.keys.Jump {
		action = ActionJump
	}
	s.game.SetIntent(move, action)
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
	s.subs[ch] = struct{}{}
	payload, _ := json.Marshal(s.snapshotLocked())
	s.eventSeq++
	ev := Event{ID: fmt.Sprintf("%d", s.eventSeq), Type: "snapshot", Data: payload, Index: s.eventSeq}
	s.mu.Unlock()
	select {
	case ch <- ev:
	default:
	}
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
	if s.Mode == "agent" {
		go s.decisionLoop(ctx)
	}

	physTicker := time.NewTicker(time.Second / PhysicsHz)
	defer physTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-physTicker.C:
		}

		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return
		}
		status := s.status
		alive := s.game.Alive
		s.mu.Unlock()

		if status == StatusFinished || !alive {
			s.mu.Lock()
			if s.status != StatusFinished && s.status != StatusStopped {
				s.status = StatusFinished
				s.emitLocked("status", map[string]any{"status": s.status})
				s.emitLocked("snapshot", s.snapshotLocked())
			}
			s.mu.Unlock()
			continue
		}
		if status == StatusCooldown {
			s.mu.Lock()
			if time.Now().Before(s.cooldownUntil) {
				s.mu.Unlock()
				continue
			}
			s.status = StatusRunning
			s.lastError = ""
			s.emitLocked("status", map[string]any{"status": s.status})
			s.mu.Unlock()
			status = StatusRunning
		}
		if status == StatusPaused || status == StatusError {
			continue
		}

		s.mu.Lock()
		if s.Mode == "human" {
			s.applyKeysLocked()
		}
		s.applyLatestDecisionLocked()
		s.game.Step()
		// emit snapshot every 3 physics frames (~20Hz)
		if s.game.Ticks%3 == 0 {
			s.emitLocked("snapshot", s.snapshotLocked())
		}
		if !s.game.Alive {
			s.status = StatusFinished
			s.emitLocked("status", map[string]any{"status": s.status})
			s.emitLocked("snapshot", s.snapshotLocked())
		}
		s.mu.Unlock()
	}
}

// decisionLoop samples the game at decision_hz and runs exactly one provider
// request at a time. Provider latency never blocks the physics loop; the
// physics loop keeps consuming the last applied intent until a fresh result is
// available.
func (s *Session) decisionLoop(ctx context.Context) {
	interval := time.Duration(float64(time.Second) / s.DecisionHz)
	if interval <= 0 {
		interval = time.Second / 8
	}
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return
		}
		if s.status != StatusRunning || !s.game.Alive {
			s.mu.Unlock()
			timer.Reset(interval)
			continue
		}
		generation := s.generation
		tick := s.game.Ticks
		cues := s.game.DecisionCues()
		provider := s.Provider
		s.mu.Unlock()

		policy := &Policy{Provider: provider}
		dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		dec, err := policy.Decide(dctx, cues)
		cancel()

		s.mu.Lock()
		valid := !s.stopped && s.status == StatusRunning && s.game.Alive && s.generation == generation
		if valid {
			if err != nil {
				s.handleDecisionErrorLocked(err)
			} else {
				s.latestDecision = &decisionResult{
					generation: generation,
					tick:       tick,
					decision:   dec,
				}
			}
		}
		s.mu.Unlock()

		timer.Reset(interval)
	}
}

func (s *Session) applyLatestDecisionLocked() {
	result := s.latestDecision
	if result == nil {
		return
	}
	s.latestDecision = nil
	if result.generation != s.generation || s.status != StatusRunning || !s.game.Alive {
		return
	}
	if s.game.Ticks-result.tick > maxDecisionLagTicks(s.DecisionHz) {
		return
	}
	s.game.SetIntent(MoveIntent(result.decision.Move), ActionIntent(result.decision.Action))
	s.lastDecision = &result.decision
	s.emitLocked("decision", result.decision)
}

func maxDecisionLagTicks(hz float64) int64 {
	if hz <= 0 {
		hz = DefaultDHz
	}
	lag := int64(math.Ceil(float64(PhysicsHz) * 2 / hz))
	if lag < 1 {
		return 1
	}
	return lag
}

func (s *Session) handleDecisionErrorLocked(err error) {
	s.lastError = err.Error()
	if retry, ok := err.(retryAfterer); ok && retry.GetRetryAfter() > 0 {
		d := retry.GetRetryAfter()
		s.status = StatusCooldown
		s.cooldownUntil = time.Now().Add(d)
		s.emitLocked("error", map[string]any{
			"code":                "provider_rate_limited",
			"message":             s.lastError,
			"retry_after_seconds": d.Seconds(),
		})
		s.emitLocked("status", map[string]any{
			"status":              s.status,
			"retry_after_seconds": d.Seconds(),
		})
		return
	}
	s.status = StatusError
	s.emitLocked("error", map[string]any{
		"code":    "provider_error",
		"message": s.lastError,
	})
	s.emitLocked("status", map[string]any{"status": s.status})
}
