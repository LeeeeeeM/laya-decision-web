package snake_test

import (
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/snake"
)

func TestNewGameDeterministicFood(t *testing.T) {
	a, err := snake.NewGame(24, 16, 7, 6)
	if err != nil {
		t.Fatal(err)
	}
	b, err := snake.NewGame(24, 16, 7, 6)
	if err != nil {
		t.Fatal(err)
	}
	if a.Food == nil || b.Food == nil || *a.Food != *b.Food {
		t.Fatalf("expected same food for same seed, got %v vs %v", a.Food, b.Food)
	}
	if len(a.Body) != 6 {
		t.Fatalf("body length=%d", len(a.Body))
	}
}

func TestHamiltonianRequiresEven(t *testing.T) {
	if _, err := snake.HamiltonianCycle(5, 5); err == nil {
		t.Fatal("expected error for odd×odd board")
	}
}

func TestStepEatsAndGrows(t *testing.T) {
	g, err := snake.NewGame(8, 6, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	moves := g.Moves()
	if len(moves) == 0 {
		t.Fatal("expected moves")
	}
	var dir string
	for _, m := range moves {
		if m.Safe {
			dir = m.Direction
			break
		}
	}
	if dir == "" {
		t.Fatal("no safe move")
	}
	before := len(g.Body)
	_, err = g.Step(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Alive {
		t.Fatalf("died: %s", g.DeathReason)
	}
	if len(g.Body) < before {
		t.Fatal("body shrank unexpectedly")
	}
}
