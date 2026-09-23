package snake_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/mockprovider"
	"github.com/LeeeeeeM/laya-decision-web/internal/snake"
)

func TestWallCollisionKills(t *testing.T) {
	g, err := snake.NewGame(8, 6, 7, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 128; i++ {
		if g.LegalReason("UP") == "wall" {
			_, err := g.Step("UP")
			if err != nil {
				t.Fatal(err)
			}
			if g.Alive || g.DeathReason != "wall" {
				t.Fatalf("alive=%v reason=%q", g.Alive, g.DeathReason)
			}
			return
		}
		moved := false
		for _, m := range g.Moves() {
			if m.Legal {
				if _, err := g.Step(m.Direction); err != nil {
					t.Fatal(err)
				}
				moved = true
				break
			}
		}
		if !moved {
			t.Fatal("stuck before wall")
		}
	}
	t.Fatal("did not hit wall")
}

func TestReverseIsIllegal(t *testing.T) {
	g, err := snake.NewGame(24, 16, 7, 6)
	if err != nil {
		t.Fatal(err)
	}
	var first string
	for _, m := range g.Moves() {
		if m.Legal {
			first = m.Direction
			break
		}
	}
	if first == "" {
		t.Fatal("no legal move")
	}
	if _, err := g.Step(first); err != nil {
		t.Fatal(err)
	}
	opp := map[string]string{"UP": "DOWN", "DOWN": "UP", "LEFT": "RIGHT", "RIGHT": "LEFT"}[first]
	if got := g.LegalReason(opp); got != "reverse" {
		t.Fatalf("expected reverse for %s after %s, got %s", opp, first, got)
	}
}

func TestEatFoodGrowsAndRespawns(t *testing.T) {
	g, err := snake.NewGame(24, 16, 7, 6)
	if err != nil {
		t.Fatal(err)
	}
	startLen := len(g.Body)
	for step := 0; step < 4000; step++ {
		moves := g.Moves()
		chosen := ""
		for _, m := range moves {
			if m.Eats && m.Safe {
				chosen = m.Direction
				break
			}
		}
		if chosen == "" {
			for _, m := range moves {
				if m.Safe {
					chosen = m.Direction
					break
				}
			}
		}
		if chosen == "" {
			t.Fatalf("no safe move at tick %d", g.Ticks)
		}
		ate, err := g.Step(chosen)
		if err != nil {
			t.Fatal(err)
		}
		if ate {
			if len(g.Body) != startLen+1 {
				t.Fatalf("length after eat=%d want %d", len(g.Body), startLen+1)
			}
			if g.Score != 1 {
				t.Fatalf("score=%d", g.Score)
			}
			if g.Food == nil {
				t.Fatal("food should respawn")
			}
			return
		}
	}
	t.Fatal("never ate food")
}

func TestSnapshotShape(t *testing.T) {
	g, err := snake.NewGame(24, 16, 7, 6)
	if err != nil {
		t.Fatal(err)
	}
	s := g.Snapshot()
	if s.Width != 24 || s.Height != 16 || s.Seed != 7 || !s.Alive || s.Won {
		t.Fatalf("%+v", s)
	}
	if len(s.Body) != 6 || len(s.Food) != 2 {
		t.Fatalf("body/food %+v", s)
	}
}

func TestPolicyGuardedIntervention(t *testing.T) {
	g, err := snake.NewGame(24, 16, 7, 6)
	if err != nil {
		t.Fatal(err)
	}
	unsafe := pickNonSafe(g)
	if unsafe == "" {
		t.Fatal("expected a non-safe direction")
	}
	p := &snake.Policy{Provider: &forcedProvider{dir: unsafe}, Guarded: true, Prompt: "compact"}
	dec, err := p.Decide(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Proposed != unsafe {
		t.Fatalf("proposed=%s want %s", dec.Proposed, unsafe)
	}
	if !dec.Intervened {
		t.Fatal("expected safety intervention")
	}
	found := false
	for _, d := range dec.SafeDirections {
		if d == dec.Executed {
			found = true
		}
	}
	if !found {
		t.Fatalf("executed %s not in safe %v", dec.Executed, dec.SafeDirections)
	}
}

func TestPolicyWithMockProvider(t *testing.T) {
	g, err := snake.NewGame(24, 16, 7, 6)
	if err != nil {
		t.Fatal(err)
	}
	p := &snake.Policy{Provider: mockprovider.New(), Guarded: true, Prompt: "compact"}
	dec, err := p.Decide(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range snake.Directions {
		if _, ok := dec.Probabilities[d]; !ok {
			t.Fatalf("missing prob %s", d)
		}
	}
	if dec.Executed == "" || dec.Provider != decision.ProviderMock {
		t.Fatalf("%+v", dec)
	}
}

func TestCriteriaJSONPreservesDirectionOrder(t *testing.T) {
	g, err := snake.NewGame(24, 16, 7, 6)
	if err != nil {
		t.Fatal(err)
	}
	cap := &captureProvider{}
	p := &snake.Policy{Provider: cap, Guarded: true, Prompt: "compact"}
	if _, err := p.Decide(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	q := cap.last.Questions["move"]
	dec := json.NewDecoder(bytes.NewReader(q.Criteria))
	tok, err := dec.Token()
	if err != nil {
		t.Fatal(err)
	}
	if tok != json.Delim('{') {
		t.Fatalf("want object, got %v", tok)
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k.(string))
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatal(err)
		}
	}
	want := snake.Directions
	if len(keys) != len(want) {
		t.Fatalf("keys=%v", keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("order=%v want %v", keys, want)
		}
	}
}

func pickNonSafe(g *snake.Game) string {
	for _, m := range g.Moves() {
		if !m.Safe {
			return m.Direction
		}
	}
	return ""
}

type forcedProvider struct{ dir string }

func (p *forcedProvider) Name() string    { return "forced" }
func (p *forcedProvider) Available() bool { return true }
func (p *forcedProvider) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	_ = ctx
	_ = req
	probs := map[string]float64{"UP": 0.01, "DOWN": 0.01, "LEFT": 0.01, "RIGHT": 0.01}
	probs[p.dir] = 0.97
	move, _ := json.Marshal(map[string]any{"type": "choice", "probabilities": probs, "confidence": 0.97})
	risk, _ := json.Marshal(map[string]any{"type": "noul", "noul": 0.9, "confidence": 0.9})
	food, _ := json.Marshal(map[string]any{"type": "noul", "noul": 0.8, "confidence": 0.8})
	return decision.Response{
		Provider: "forced",
		Model:    "forced",
		Answers:  map[string]json.RawMessage{"move": move, "risk": risk, "food": food},
	}, nil
}

type captureProvider struct{ last decision.Request }

func (p *captureProvider) Name() string    { return "capture" }
func (p *captureProvider) Available() bool { return true }
func (p *captureProvider) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	p.last = req
	return (&forcedProvider{dir: "DOWN"}).Decide(ctx, req)
}
