package mockprovider

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

// Provider returns deterministic heuristic answers for local demos without an API key.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string    { return decision.ProviderMock }
func (p *Provider) Available() bool { return true }

func (p *Provider) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	_ = ctx
	start := time.Now()
	questions, err := decision.NormalizeQuestions(req.Questions)
	if err != nil {
		return decision.Response{}, err
	}
	answers := make(map[string]json.RawMessage, len(questions))
	for key, q := range questions {
		var raw json.RawMessage
		switch q.Type {
		case decision.TypeChoice:
			probs := heuristicChoice(q.CriteriaMap)
			raw, _ = json.Marshal(map[string]any{
				"type":          "choice",
				"probabilities": probs,
				"confidence":    maxProb(probs),
			})
		case decision.TypeScore:
			n := len(q.CriteriaList)
			probs := make([]float64, n)
			for i := range probs {
				probs[i] = 1 / float64(n)
			}
			score := float64(n-1) / 2
			raw, _ = json.Marshal(map[string]any{
				"type":          "score",
				"score":         score,
				"probabilities": probs,
				"confidence":    probs[int(score)],
			})
		case decision.TypeNoul:
			v := 0.85
			state := strings.ToLower(string(req.State))
			instr := strings.ToLower(q.Instructions)
			if strings.Contains(state, `"no"`) || strings.Contains(state, "no.") {
				v = 0.15
			}
			if strings.Contains(state, "yes") {
				v = 0.9
			}
			if strings.Contains(instr, "safe") && strings.Contains(state, "safe route: no") {
				v = 0.1
			}
			raw, _ = json.Marshal(map[string]any{
				"type":       "noul",
				"noul":       v,
				"confidence": v,
			})
		}
		answers[key] = raw
	}
	return decision.Response{
		Provider: decision.ProviderMock,
		Model:    "mock-heuristic-v1",
		Answers:  answers,
		Usage:    decision.Usage{},
		Timing:   decision.Timing{ProviderMS: float64(time.Since(start).Microseconds()) / 1000},
	}, nil
}

func heuristicChoice(criteria map[string]string) map[string]float64 {
	weights := make(map[string]float64, len(criteria))
	total := 0.0
	for k, desc := range criteria {
		lower := strings.ToLower(desc)
		w := 0.05
		switch {
		case strings.Contains(lower, "eat"):
			w = 1.0
		case strings.Contains(lower, "best"):
			w = 0.8
		case strings.Contains(lower, "unsafe") || strings.Contains(lower, "blocked") || strings.Contains(lower, "collision"):
			w = 0.02
		case strings.Contains(lower, "slower"):
			w = 0.25
		case strings.Contains(lower, "safe"):
			w = 0.5
		}
		weights[k] = w
		total += w
	}
	out := make(map[string]float64, len(weights))
	if total == 0 {
		n := float64(len(criteria))
		for k := range criteria {
			out[k] = 1 / n
		}
		return out
	}
	for k, w := range weights {
		out[k] = w / total
	}
	return out
}

func maxProb(m map[string]float64) float64 {
	max := 0.0
	for _, v := range m {
		if v > max {
			max = v
		}
	}
	return max
}
