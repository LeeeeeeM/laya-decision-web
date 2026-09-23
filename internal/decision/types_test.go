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
