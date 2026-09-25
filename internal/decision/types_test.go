package decision_test

import (
	"encoding/json"
	"testing"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

func TestNormalizeQuestionsChoiceScoreNoul(t *testing.T) {
	choiceCrit, _ := json.Marshal(map[string]string{"a": "A", "b": "B"})
	scoreCrit, _ := json.Marshal([]string{"low", "mid", "high"})
	in := map[string]decision.Question{
		"c": {Type: decision.TypeChoice, Instructions: "pick", Criteria: choiceCrit},
		"s": {Type: decision.TypeScore, Instructions: "score", Criteria: scoreCrit},
		"n": {Type: decision.TypeNoul, Instructions: "yes?"},
	}
	out, err := decision.NormalizeQuestions(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out["c"].CriteriaMap) != 2 || len(out["s"].CriteriaList) != 3 {
		t.Fatalf("%+v", out)
	}
}

func TestNormalizeQuestionsRejectsBadChoice(t *testing.T) {
	_, err := decision.NormalizeQuestions(map[string]decision.Question{
		"c": {Type: decision.TypeChoice, Instructions: "pick", Criteria: json.RawMessage(`[]`)},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFiniteProb(t *testing.T) {
	if !decision.FiniteProb(0.5) || decision.FiniteProb(-0.1) || decision.FiniteProb(1.1) {
		t.Fatal("FiniteProb bounds")
	}
}

func TestStateAsAny(t *testing.T) {
	v, err := decision.StateAsAny(json.RawMessage(`"hi"`))
	if err != nil || v.(string) != "hi" {
		t.Fatalf("%v %v", v, err)
	}
	_, err = decision.StateAsAny(nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateResponseTypedAnswers(t *testing.T) {
	choiceCrit, _ := json.Marshal(map[string]string{"a": "A", "b": "B"})
	scoreCrit, _ := json.Marshal([]string{"low", "mid", "high"})
	req := decision.Request{
		Provider: "test",
		State:    json.RawMessage(`"state"`),
		Questions: map[string]decision.Question{
			"choice": {Type: decision.TypeChoice, Instructions: "pick", Criteria: choiceCrit},
			"score":  {Type: decision.TypeScore, Instructions: "score", Criteria: scoreCrit},
			"noul":   {Type: decision.TypeNoul, Instructions: "yes?"},
		},
	}
	resp := decision.Response{Answers: map[string]json.RawMessage{
		"choice": json.RawMessage(`{"type":"choice","choice":"a","probabilities":{"a":0.7,"b":0.3}}`),
		"score":  json.RawMessage(`{"type":"score","score":1,"probabilities":[0.2,0.6,0.2]}`),
		"noul":   json.RawMessage(`{"type":"noul","noul":0.8}`),
	}}
	if err := decision.ValidateResponse(req, resp); err != nil {
		t.Fatal(err)
	}
}

func TestValidateResponseRejectsIncompleteOrInvalidAnswers(t *testing.T) {
	criteria := json.RawMessage(`{"a":"A","b":"B"}`)
	req := decision.Request{
		Provider:  "test",
		State:     json.RawMessage(`"state"`),
		Questions: map[string]decision.Question{"move": {Type: decision.TypeChoice, Instructions: "pick", Criteria: criteria}},
	}
	cases := []json.RawMessage{
		json.RawMessage(`{"type":"choice","probabilities":{"a":0.7,"b":0.3}}`),
		json.RawMessage(`{"type":"choice","choice":"a","probabilities":{"a":0.9,"b":0.9}}`),
		json.RawMessage(`{"type":"choice","choice":"c","probabilities":{"a":0.5,"b":0.5}}`),
	}
	for i, answer := range cases {
		err := decision.ValidateResponse(req, decision.Response{Answers: map[string]json.RawMessage{"move": answer}})
		if err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}
