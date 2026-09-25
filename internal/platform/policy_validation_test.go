package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

func TestPolicyRejectsMalformedActionInsteadOfFallingBack(t *testing.T) {
	provider := malformedPlatformProvider{}
	policy := &Policy{Provider: provider}
	_, err := policy.Decide(context.Background(), DecisionCues{
		Go:   "RIGHT",
		Need: "NONE",
		Text: "need=NONE go=RIGHT ground=1",
	})
	if err == nil {
		t.Fatal("expected malformed action to be rejected")
	}
}

type malformedPlatformProvider struct{}

func (malformedPlatformProvider) Name() string    { return "malformed" }
func (malformedPlatformProvider) Available() bool { return true }

func (malformedPlatformProvider) Decide(_ context.Context, req decision.Request) (decision.Response, error) {
	move, _ := json.Marshal(map[string]any{
		"type":          "choice",
		"choice":        "RIGHT",
		"probabilities": map[string]float64{"RIGHT": 1, "IDLE": 0},
	})
	action, _ := json.Marshal(map[string]any{
		"type":          "choice",
		"choice":        "FLY",
		"probabilities": map[string]float64{"NONE": 0.5, "JUMP": 0.25, "CROUCH": 0.25},
	})
	return decision.Response{
		Provider: req.Provider,
		Model:    "malformed-test",
		Answers:  map[string]json.RawMessage{"move": move, "action": action},
	}, nil
}
