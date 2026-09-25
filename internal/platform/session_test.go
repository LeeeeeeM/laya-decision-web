package platform

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

func TestSessionPhysicsContinuesWhileDecisionIsSlow(t *testing.T) {
	provider := &delayedProvider{delay: 350 * time.Millisecond}
	mgr := NewManager(map[string]decision.Provider{"delayed": provider}, 2)
	s, err := mgr.Create(CreateRequest{
		Provider:   "delayed",
		Seed:       7,
		DecisionHz: 8,
		Mode:       "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Delete(s.ID)

	time.Sleep(550 * time.Millisecond)
	snap := s.Snapshot()
	if snap.Game.Ticks < 20 {
		t.Fatalf("physics stalled during provider call: ticks=%d", snap.Game.Ticks)
	}
	if got := provider.maxConcurrent(); got > 1 {
		t.Fatalf("decision requests overlapped: max concurrent=%d", got)
	}
}

type delayedProvider struct {
	delay time.Duration

	mu     sync.Mutex
	active int
	max    int
}

func (p *delayedProvider) Name() string    { return "delayed" }
func (p *delayedProvider) Available() bool { return true }

func (p *delayedProvider) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	p.mu.Lock()
	p.active++
	if p.active > p.max {
		p.max = p.active
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}()

	timer := time.NewTimer(p.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return decision.Response{}, ctx.Err()
	case <-timer.C:
	}

	move, _ := json.Marshal(map[string]any{
		"type":          "choice",
		"choice":        "RIGHT",
		"probabilities": map[string]float64{"RIGHT": 0.99, "IDLE": 0.01},
	})
	action, _ := json.Marshal(map[string]any{
		"type":          "choice",
		"choice":        "NONE",
		"probabilities": map[string]float64{"NONE": 1, "JUMP": 0, "CROUCH": 0},
	})
	return decision.Response{
		Provider: p.Name(),
		Model:    "delayed-test",
		Answers:  map[string]json.RawMessage{"move": move, "action": action},
	}, nil
}

func (p *delayedProvider) maxConcurrent() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.max
}
