package snake

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

type DecisionResult struct {
	Probabilities  map[string]float64 `json:"probabilities"`
	Proposed       string             `json:"proposed"`
	Executed       string             `json:"executed"`
	SafeDirections []string           `json:"safe_directions"`
	Intervened     bool               `json:"intervened"`
	DeadEndRisk    float64            `json:"dead_end_risk"`
	FoodReachable  float64            `json:"food_reachable"`
	InferenceMS    float64            `json:"inference_ms"`
	DecisionMS     float64            `json:"decision_ms"`
	InputTokens    int                `json:"input_tokens"`
	OutputTokens   int                `json:"output_tokens"`
	SafeCount      int                `json:"safe_count"`
	PlannerBest    string             `json:"planner_best"`
	Provider       string             `json:"provider"`
	Model          string             `json:"model"`
}

type Policy struct {
	Provider decision.Provider
	Guarded  bool
	Prompt   string // compact | detailed
}

func (p *Policy) Decide(ctx context.Context, game *Game) (DecisionResult, error) {
	started := time.Now()
	moves := game.Moves()
	safe := make([]MoveInfo, 0, len(moves))
	for _, m := range moves {
		if m.Safe {
			safe = append(safe, m)
		}
	}
	if p.Guarded && len(safe) == 0 {
		return DecisionResult{}, fmt.Errorf("cycle safety invariant violated: no safe action")
	}
	preferred := "NONE"
	if len(safe) > 0 {
		best := safe[0]
		for _, m := range safe[1:] {
			if m.Advance > best.Advance {
				best = m
			}
		}
		preferred = best.Direction
	}
	reachable, space := game.FoodReachability()
	state, questions := p.buildPrompt(moves, safe, preferred, reachable, space, len(game.Body))

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return DecisionResult{}, err
	}
	req := decision.Request{
		Provider:  p.Provider.Name(),
		State:     stateJSON,
		Questions: questions,
	}
	inferStart := time.Now()
	resp, err := p.Provider.Decide(ctx, req)
	inferenceMS := float64(time.Since(inferStart).Microseconds()) / 1000
	if err != nil {
		return DecisionResult{}, err
	}

	probs, risk, food, err := parseAnswers(resp.Answers)
	if err != nil {
		return DecisionResult{}, err
	}
	proposed := maxKey(probs, Directions)
	allowed := make([]string, 0, len(safe))
	for _, m := range safe {
		allowed = append(allowed, m.Direction)
	}
	executed := proposed
	if p.Guarded {
		found := false
		for _, d := range allowed {
			if d == proposed {
				found = true
				break
			}
		}
		if !found && len(allowed) > 0 {
			executed = maxKey(probs, allowed)
		}
	}
	return DecisionResult{
		Probabilities:  probs,
		Proposed:       proposed,
		Executed:       executed,
		SafeDirections: allowed,
		Intervened:     proposed != executed,
		DeadEndRisk:    1 - risk,
		FoodReachable:  food,
		InferenceMS:    inferenceMS,
		DecisionMS:     float64(time.Since(started).Microseconds()) / 1000,
		InputTokens:    resp.Usage.InputTokens,
		OutputTokens:   resp.Usage.OutputTokens,
		SafeCount:      len(safe),
		PlannerBest:    preferred,
		Provider:       resp.Provider,
		Model:          resp.Model,
	}, nil
}

func (p *Policy) buildPrompt(
	moves []MoveInfo,
	safe []MoveInfo,
	preferred string,
	reachable bool,
	space int,
	length int,
) (string, map[string]decision.Question) {
	prompt := p.Prompt
	if prompt == "" {
		prompt = "compact"
	}
	yesNo := map[bool]string{true: "yes", false: "no"}
	var state string
	questions := map[string]decision.Question{
		"move": {
			Type: decision.TypeChoice,
		},
		"risk": {
			Type: decision.TypeNoul,
		},
		"food": {
			Type: decision.TypeNoul,
		},
	}

	if prompt == "detailed" {
		state = fmt.Sprintf(
			"Snake game. %d safe directions available. Food reachable through empty cells: %s. Open cells: %d. Snake length: %d. %s",
			len(safe),
			yesNo[reachable],
			space,
			length,
			map[bool]string{true: "There is a safe route forward.", false: "The snake is trapped."}[len(safe) > 0],
		)
		criteria := make(map[string]string, len(moves))
		for _, m := range moves {
			switch {
			case !m.Legal:
				criteria[m.Direction] = fmt.Sprintf("Collision: %s. Unsafe.", m.Reason)
			case !m.Safe:
				criteria[m.Direction] = "Unsafe route. Risk of trapping the snake."
			case m.Eats:
				criteria[m.Direction] = "Safe. Eat the food immediately. Best move."
			case m.Direction == preferred:
				criteria[m.Direction] = "Safe. Best progress toward food."
			default:
				criteria[m.Direction] = "Safe but less progress toward food."
			}
		}
		raw, _ := marshalOrderedObject(Directions, criteria)
		questions["move"] = decision.Question{
			Type:         decision.TypeChoice,
			Instructions: "Select the safest move with best progress toward food. Avoid collisions.",
			Criteria:     raw,
			CriteriaMap:  criteria,
		}
		questions["risk"] = decision.Question{
			Type:         decision.TypeNoul,
			Instructions: "Is there a safe route forward for the snake?",
		}
		questions["food"] = decision.Question{
			Type:         decision.TypeNoul,
			Instructions: "Is food reachable through the currently empty cells?",
		}
		return state, questions
	}

	state = fmt.Sprintf(
		"Safe route: %s. Food reachable through empty cells: %s.",
		yesNo[len(safe) > 0],
		yesNo[reachable],
	)
	criteria := make(map[string]string, len(moves))
	for _, m := range moves {
		switch {
		case !m.Legal:
			criteria[m.Direction] = "Blocked. Collision."
		case !m.Safe:
			criteria[m.Direction] = "Unsafe. Traps the snake."
		case m.Eats:
			criteria[m.Direction] = "Safe. Eat food now. Best."
		case m.Direction == preferred:
			criteria[m.Direction] = "Safe. Best route to food."
		default:
			criteria[m.Direction] = "Safe. Slower route."
		}
	}
	raw, _ := marshalOrderedObject(Directions, criteria)
	questions["move"] = decision.Question{
		Type:         decision.TypeChoice,
		Instructions: "Choose the best safe move toward food.",
		Criteria:     raw,
		CriteriaMap:  criteria,
	}
	questions["risk"] = decision.Question{
		Type:         decision.TypeNoul,
		Instructions: "Is a safe route available?",
	}
	questions["food"] = decision.Question{
		Type:         decision.TypeNoul,
		Instructions: "Is food reachable through empty cells?",
	}
	return state, questions
}

func parseAnswers(answers map[string]json.RawMessage) (map[string]float64, float64, float64, error) {
	moveRaw, ok := answers["move"]
	if !ok {
		return nil, 0, 0, fmt.Errorf("decision backend omitted move answer")
	}
	var move struct {
		Probabilities map[string]float64 `json:"probabilities"`
	}
	if err := json.Unmarshal(moveRaw, &move); err != nil {
		return nil, 0, 0, fmt.Errorf("invalid move answer: %w", err)
	}
	if move.Probabilities == nil {
		return nil, 0, 0, fmt.Errorf("decision backend omitted direction probabilities")
	}
	for _, d := range Directions {
		v, ok := move.Probabilities[d]
		if !ok || !decision.FiniteProb(v) {
			return nil, 0, 0, fmt.Errorf("invalid probability for %s", d)
		}
	}
	risk, err := parseNoul(answers, "risk")
	if err != nil {
		return nil, 0, 0, err
	}
	food, err := parseNoul(answers, "food")
	if err != nil {
		return nil, 0, 0, err
	}
	return move.Probabilities, risk, food, nil
}

func parseNoul(answers map[string]json.RawMessage, key string) (float64, error) {
	raw, ok := answers[key]
	if !ok {
		return 0, fmt.Errorf("decision backend omitted %s answer", key)
	}
	var noul struct {
		Noul float64 `json:"noul"`
	}
	if err := json.Unmarshal(raw, &noul); err != nil {
		return 0, fmt.Errorf("invalid %s answer: %w", key, err)
	}
	if !decision.FiniteProb(noul.Noul) {
		return 0, fmt.Errorf("invalid %s probability", key)
	}
	return noul.Noul, nil
}

func maxKey(probs map[string]float64, keys []string) string {
	best := keys[0]
	bestV := math.Inf(-1)
	for _, k := range keys {
		if v, ok := probs[k]; ok && v > bestV {
			bestV = v
			best = k
		}
	}
	return best
}

// marshalOrderedObject encodes an object with keys in the given order (Python 3.7+ dict parity).
func marshalOrderedObject(order []string, values map[string]string) ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	first := true
	for _, key := range order {
		val, ok := values[key]
		if !ok {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		kb, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		b.Write(vb)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}
