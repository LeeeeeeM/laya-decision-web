package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

type DecisionResult struct {
	Move        string             `json:"move"`
	Action      string             `json:"action"`
	ProbMove    map[string]float64 `json:"prob_move"`
	ProbAction  map[string]float64 `json:"prob_action"`
	Proposed    string             `json:"proposed"`
	Executed    string             `json:"executed"`
	Intervened  bool               `json:"intervened"`
	InferenceMS float64            `json:"inference_ms"`
	DecisionMS  float64            `json:"decision_ms"`
	Provider    string             `json:"provider"`
	Model       string             `json:"model"`
}

type Policy struct {
	Provider decision.Provider
}

// Decide runs multi-option inference from a cues snapshot (no Game mutation).
// Caller must apply Move/Action via SetIntent under its own lock.
func (p *Policy) Decide(ctx context.Context, cues DecisionCues) (DecisionResult, error) {
	start := time.Now()

	// Agent movement is forward-only: it may move right or hold position, never left.
	moveOptions := []string{string(MoveRight), string(MoveIdle)}
	moveTarget := cues.Go
	if !containsChoice(moveOptions, moveTarget) {
		moveTarget = string(MoveRight)
	}
	moveOrder := preferFirst(moveTarget, moveOptions)
	moveValues := map[string]string{}
	for _, m := range moveOrder {
		if m == moveTarget {
			moveValues[m] = "Best. Matches go=" + moveTarget + "."
		} else if m == string(MoveIdle) {
			moveValues[m] = "Allowed for a brief wait or to pair with JUMP for a vertical jump; do not idle without a reason."
		} else {
			moveValues[m] = "Continue forward when safe; backward movement is not allowed."
		}
	}
	actionOrder := preferFirst(cues.Need, []string{"NONE", "JUMP", "CROUCH"})
	actionValues := map[string]string{}
	for _, a := range actionOrder {
		if a == cues.Need {
			actionValues[a] = "Best. Matches need=" + cues.Need + "."
		} else {
			actionValues[a] = "Worse."
		}
	}
	moveCrit, _ := marshalOrderedObject(moveOrder, moveValues)
	actionCrit, _ := marshalOrderedObject(actionOrder, actionValues)

	questions := map[string]decision.Question{
		"move": {
			Type:         decision.TypeChoice,
			Instructions: "Pick best move.",
			Criteria:     moveCrit,
		},
		"action": {
			Type:         decision.TypeChoice,
			Instructions: "Pick best action.",
			Criteria:     actionCrit,
		},
	}
	rawState, _ := json.Marshal(cues.Text)
	req := decision.Request{
		Provider:  p.Provider.Name(),
		State:     rawState,
		Questions: questions,
	}
	inferStart := time.Now()
	resp, err := p.Provider.Decide(ctx, req)
	inferMS := float64(time.Since(inferStart).Microseconds()) / 1000
	if err != nil {
		return DecisionResult{}, err
	}
	if err := decision.ValidateResponse(req, resp); err != nil {
		return DecisionResult{}, err
	}

	proposedMove, moveProbs, err := pickChoice(resp.Answers, "move")
	if err != nil {
		return DecisionResult{}, err
	}
	executedMove := proposedMove
	proposedAction, actionProbs, err := pickChoice(resp.Answers, "action")
	if err != nil {
		return DecisionResult{}, err
	}
	executedAction := proposedAction
	intervened := false
	if !containsChoice(moveOptions, executedMove) {
		return DecisionResult{}, fmt.Errorf("provider returned unsupported move %q", executedMove)
	}
	if cues.LockMoveToGo && executedMove != moveTarget {
		executedMove = moveTarget
		intervened = true
	}
	if !containsChoice([]string{"NONE", "JUMP", "CROUCH"}, proposedAction) {
		return DecisionResult{}, fmt.Errorf("provider returned unsupported action %q", proposedAction)
	}
	if cues.Need != "" && cues.Need != "NONE" && proposedAction != cues.Need {
		executedAction = cues.Need
		intervened = true
	}
	proposed := proposedMove + "+" + proposedAction
	executed := executedMove + "+" + executedAction

	return DecisionResult{
		Move: executedMove, Action: executedAction, ProbMove: moveProbs, ProbAction: actionProbs,
		Proposed: proposed, Executed: executed, Intervened: intervened,
		InferenceMS: inferMS, DecisionMS: float64(time.Since(start).Microseconds()) / 1000,
		Provider: resp.Provider, Model: resp.Model,
	}, nil
}

func containsChoice(options []string, choice string) bool {
	for _, option := range options {
		if choice == option {
			return true
		}
	}
	return false
}

func preferFirst(first string, all []string) []string {
	out := make([]string, 0, len(all))
	seen := map[string]bool{}
	if first != "" {
		out = append(out, first)
		seen[first] = true
	}
	for _, v := range all {
		if seen[v] {
			continue
		}
		out = append(out, v)
	}
	return out
}

func marshalOrderedObject(order []string, values map[string]string) (json.RawMessage, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range order {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(values[k])
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func pickChoice(answers map[string]json.RawMessage, key string) (string, map[string]float64, error) {
	raw, ok := answers[key]
	if !ok {
		return "", nil, fmt.Errorf("provider response omitted %s answer", key)
	}
	var obj struct {
		Choice        string             `json:"choice"`
		Probabilities map[string]float64 `json:"probabilities"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", nil, fmt.Errorf("invalid %s answer: %w", key, err)
	}
	if obj.Choice == "" {
		return "", nil, fmt.Errorf("provider response omitted %s choice", key)
	}
	if len(obj.Probabilities) == 0 {
		return "", nil, fmt.Errorf("provider response omitted %s probabilities", key)
	}
	return obj.Choice, obj.Probabilities, nil
}
