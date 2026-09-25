package decision

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
)

const (
	ProviderLaya     = "laya"
	ProviderBochaJev = "bocha-jev"
	ProviderMock     = "mock"
)

type QuestionType string

const (
	TypeChoice QuestionType = "choice"
	TypeScore  QuestionType = "score"
	TypeNoul   QuestionType = "noul"
)

type Question struct {
	Type         QuestionType      `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     json.RawMessage   `json:"criteria,omitempty"`
	CriteriaMap  map[string]string `json:"-"`
	CriteriaList []string          `json:"-"`
}

type Request struct {
	Provider  string              `json:"provider"`
	Model     string              `json:"model,omitempty"`
	State     json.RawMessage     `json:"state"`
	Questions map[string]Question `json:"questions"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Timing struct {
	ProviderMS float64 `json:"provider_ms"`
	RequestMS  float64 `json:"request_ms"`
}

type Response struct {
	Provider string                     `json:"provider"`
	Model    string                     `json:"model"`
	Answers  map[string]json.RawMessage `json:"answers"`
	Usage    Usage                      `json:"usage"`
	Timing   Timing                     `json:"timing"`
}

type InvalidResponseError struct {
	Message string
}

func (e *InvalidResponseError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "provider returned an invalid decision response"
}

type Provider interface {
	Name() string
	Available() bool
	Decide(ctx context.Context, req Request) (Response, error)
}

type ModelInfo struct {
	ID           string `json:"id"`
	Ready        bool   `json:"ready"`
	Runtime      string `json:"runtime"`
	ComputeUnits string `json:"compute_units,omitempty"`
	Status       string `json:"status,omitempty"`
}

type ProviderInfo struct {
	ID        string      `json:"id"`
	Available bool        `json:"available"`
	Model     string      `json:"model,omitempty"`
	Models    []ModelInfo `json:"models,omitempty"`
}

type Capabilities struct {
	Providers []ProviderInfo `json:"providers"`
}

func NormalizeQuestions(questions map[string]Question) (map[string]Question, error) {
	if len(questions) == 0 {
		return nil, fmt.Errorf("questions required")
	}
	out := make(map[string]Question, len(questions))
	for key, q := range questions {
		if q.Instructions == "" {
			return nil, fmt.Errorf("question %q missing instructions", key)
		}
		switch q.Type {
		case TypeChoice:
			var m map[string]string
			if err := json.Unmarshal(q.Criteria, &m); err != nil || len(m) == 0 {
				return nil, fmt.Errorf("question %q choice criteria must be a non-empty object", key)
			}
			q.CriteriaMap = m
		case TypeScore:
			var list []string
			if err := json.Unmarshal(q.Criteria, &list); err != nil || len(list) < 2 {
				return nil, fmt.Errorf("question %q score criteria must be an array with >= 2 labels", key)
			}
			q.CriteriaList = list
		case TypeNoul:
			// no criteria
		default:
			return nil, fmt.Errorf("question %q has unsupported type %q", key, q.Type)
		}
		out[key] = q
	}
	return out, nil
}

func StateAsAny(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("state required")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("invalid state json: %w", err)
	}
	return v, nil
}

func FiniteProb(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
}

const probabilitySumTolerance = 0.02

// ValidateResponse validates the typed-decision contract at the provider
// boundary. Callers must not derive an action from an incomplete or malformed
// answer because doing so hides provider failures and can bypass safety rules.
func ValidateResponse(req Request, resp Response) error {
	questions, err := NormalizeQuestions(req.Questions)
	if err != nil {
		return &InvalidResponseError{Message: "cannot validate provider response: " + err.Error()}
	}
	if resp.Answers == nil {
		return &InvalidResponseError{Message: "provider response omitted answers"}
	}
	if len(resp.Answers) != len(questions) {
		return &InvalidResponseError{Message: fmt.Sprintf("provider returned %d answers for %d questions", len(resp.Answers), len(questions))}
	}

	for key, q := range questions {
		raw, ok := resp.Answers[key]
		if !ok {
			return &InvalidResponseError{Message: fmt.Sprintf("provider response omitted answer %q", key)}
		}
		if err := validateAnswer(key, q, raw); err != nil {
			return &InvalidResponseError{Message: err.Error()}
		}
	}
	for key := range resp.Answers {
		if _, ok := questions[key]; !ok {
			return &InvalidResponseError{Message: fmt.Sprintf("provider response returned unknown answer %q", key)}
		}
	}
	return nil
}

func validateAnswer(key string, q Question, raw json.RawMessage) error {
	var envelope struct {
		Type          string          `json:"type"`
		Choice        string          `json:"choice"`
		Score         *float64        `json:"score"`
		Noul          *float64        `json:"noul"`
		Confidence    *float64        `json:"confidence"`
		Probabilities json.RawMessage `json:"probabilities"`
		Action        *struct {
			ActProbability *float64 `json:"act_probability"`
		} `json:"action"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("answer %q is not valid JSON: %w", key, err)
	}
	if envelope.Type != "" && envelope.Type != string(q.Type) {
		return fmt.Errorf("answer %q has type %q, want %q", key, envelope.Type, q.Type)
	}
	if envelope.Confidence != nil && !FiniteProb(*envelope.Confidence) {
		return fmt.Errorf("answer %q has invalid confidence", key)
	}
	if envelope.Action != nil && envelope.Action.ActProbability != nil && !FiniteProb(*envelope.Action.ActProbability) {
		return fmt.Errorf("answer %q has invalid action probability", key)
	}

	switch q.Type {
	case TypeChoice:
		if envelope.Choice == "" {
			return fmt.Errorf("answer %q omitted choice", key)
		}
		if _, ok := q.CriteriaMap[envelope.Choice]; !ok {
			return fmt.Errorf("answer %q selected unknown choice %q", key, envelope.Choice)
		}
		probs, err := decodeProbabilityMap(envelope.Probabilities)
		if err != nil {
			return fmt.Errorf("answer %q has invalid choice probabilities: %w", key, err)
		}
		if err := validateProbabilityMap(probs, q.CriteriaMap); err != nil {
			return fmt.Errorf("answer %q has invalid choice probabilities: %w", key, err)
		}
	case TypeScore:
		if envelope.Score == nil || !finite(*envelope.Score) {
			return fmt.Errorf("answer %q omitted a finite score", key)
		}
		if *envelope.Score < 0 || *envelope.Score > float64(len(q.CriteriaList)-1) {
			return fmt.Errorf("answer %q score %v is outside [0,%d]", key, *envelope.Score, len(q.CriteriaList)-1)
		}
		if err := validateScoreProbabilities(envelope.Probabilities, len(q.CriteriaList)); err != nil {
			return fmt.Errorf("answer %q has invalid score probabilities: %w", key, err)
		}
	case TypeNoul:
		if envelope.Noul == nil || !FiniteProb(*envelope.Noul) {
			return fmt.Errorf("answer %q omitted a finite noul probability", key)
		}
	}
	return nil
}

func decodeProbabilityMap(raw json.RawMessage) (map[string]float64, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("probabilities omitted")
	}
	var probs map[string]float64
	if err := json.Unmarshal(raw, &probs); err != nil || probs == nil {
		if err == nil {
			err = fmt.Errorf("expected an object")
		}
		return nil, err
	}
	return probs, nil
}

func validateProbabilityMap(probs map[string]float64, criteria map[string]string) error {
	if len(probs) != len(criteria) {
		return fmt.Errorf("got %d options, want %d", len(probs), len(criteria))
	}
	sum := 0.0
	for key := range criteria {
		value, ok := probs[key]
		if !ok {
			return fmt.Errorf("missing option %q", key)
		}
		if !FiniteProb(value) {
			return fmt.Errorf("option %q is outside [0,1]", key)
		}
		sum += value
	}
	if math.Abs(sum-1) > probabilitySumTolerance {
		return fmt.Errorf("probability sum is %v, want 1", sum)
	}
	return nil
}

func validateScoreProbabilities(raw json.RawMessage, count int) error {
	if len(raw) == 0 {
		return fmt.Errorf("probabilities omitted")
	}
	var list []float64
	if err := json.Unmarshal(raw, &list); err == nil && list != nil {
		return validateProbabilityList(list, count)
	}
	var object map[string]float64
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return fmt.Errorf("expected an array or object")
	}
	if len(object) != count {
		return fmt.Errorf("got %d options, want %d", len(object), count)
	}
	list = make([]float64, count)
	for i := range list {
		value, ok := object[fmt.Sprintf("%d", i)]
		if !ok {
			return fmt.Errorf("missing option %d", i)
		}
		list[i] = value
	}
	return validateProbabilityList(list, count)
}

func validateProbabilityList(values []float64, count int) error {
	if len(values) != count {
		return fmt.Errorf("got %d options, want %d", len(values), count)
	}
	sum := 0.0
	for _, value := range values {
		if !FiniteProb(value) {
			return fmt.Errorf("probability is outside [0,1]")
		}
		sum += value
	}
	if math.Abs(sum-1) > probabilitySumTolerance {
		return fmt.Errorf("probability sum is %v, want 1", sum)
	}
	return nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
