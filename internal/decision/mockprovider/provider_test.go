package mockprovider_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/mockprovider"
)

func TestMockDecideTypes(t *testing.T) {
	p := mockprovider.New()
	state, _ := json.Marshal("Safe route: yes. Food reachable through empty cells: yes.")
	choice, _ := json.Marshal(map[string]string{
		"UP": "Safe. Best route to food.", "DOWN": "Blocked. Collision.",
		"LEFT": "Safe. Slower route.", "RIGHT": "Unsafe. Traps the snake.",
	})
	score, _ := json.Marshal([]string{"a", "b", "c"})
	resp, err := p.Decide(context.Background(), decision.Request{
		State: state,
		Questions: map[string]decision.Question{
			"move":  {Type: decision.TypeChoice, Instructions: "move", Criteria: choice},
			"score": {Type: decision.TypeScore, Instructions: "score", Criteria: score},
			"risk":  {Type: decision.TypeNoul, Instructions: "safe?"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Provider != decision.ProviderMock || len(resp.Answers) != 3 {
		t.Fatalf("%+v", resp)
	}
	var move struct {
		Probabilities map[string]float64 `json:"probabilities"`
	}
	if err := json.Unmarshal(resp.Answers["move"], &move); err != nil {
		t.Fatal(err)
	}
	if move.Probabilities["UP"] <= move.Probabilities["DOWN"] {
		t.Fatalf("expected UP preferred over DOWN: %+v", move.Probabilities)
	}
}
