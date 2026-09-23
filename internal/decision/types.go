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
