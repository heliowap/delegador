package devin

import (
	"os"
	"testing"
)

func fixture(t *testing.T) []Model {
	t.Helper()
	f, err := os.Open("../../testdata/models-list.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	models, err := ParseModelList(f)
	if err != nil {
		t.Fatalf("ParseModelList: %v", err)
	}
	return models
}

func TestParseModelListReadsFreeAndPaid(t *testing.T) {
	models := fixture(t)

	byUID := map[string]Model{}
	for _, m := range models {
		byUID[m.UID] = m
	}

	swe, ok := byUID["swe-2-max"]
	if !ok {
		t.Fatal("swe-2-max ausente")
	}
	if !swe.Free {
		t.Error("swe-2-max deveria ser Free")
	}
	if swe.ContextTokens != 262_000 {
		t.Errorf("ContextTokens = %d, quero 262000", swe.ContextTokens)
	}
	if swe.Family != "swe-2" {
		t.Errorf("Family = %q, quero swe-2", swe.Family)
	}

	opus, ok := byUID["claude-opus-5-max"]
	if !ok {
		t.Fatal("claude-opus-5-max ausente")
	}
	if opus.Free {
		t.Error("claude-opus-5-max nao e gratuito")
	}
	if opus.InputUSDPerM != 5 {
		t.Errorf("InputUSDPerM = %v, quero 5", opus.InputUSDPerM)
	}
	if opus.ContextTokens != 1_000_000 {
		t.Errorf("ContextTokens = %d, quero 1000000", opus.ContextTokens)
	}
}

// Linha em formato desconhecido nao pode derrubar o parser: o devin ganha
// familias novas com frequencia e o plugin precisa sobreviver a isso.
func TestParseModelListIgnoresUnknownFormat(t *testing.T) {
	for _, m := range fixture(t) {
		if m.UID == "qf9-warp" {
			t.Error("linha em formato desconhecido nao deveria virar Model")
		}
	}
}

func TestSelectModelPrefersFree(t *testing.T) {
	got, err := SelectModel(fixture(t), EffortMax)
	if err != nil {
		t.Fatalf("SelectModel: %v", err)
	}
	if got.UID != "swe-2-max" {
		t.Errorf("UID = %q, quero swe-2-max (gratuito vence)", got.UID)
	}
}

func TestSelectModelFallsBackToCheapestPaid(t *testing.T) {
	var paid []Model
	for _, m := range fixture(t) {
		if !m.Free {
			paid = append(paid, m)
		}
	}
	got, err := SelectModel(paid, EffortMedium)
	if err != nil {
		t.Fatalf("SelectModel: %v", err)
	}
	if got.UID != "gpt-5-6-luna-medium" {
		t.Errorf("UID = %q, quero gpt-5-6-luna-medium (mais barato)", got.UID)
	}
}

func TestSelectModelErrorsOnEmpty(t *testing.T) {
	if _, err := SelectModel(nil, EffortMax); err == nil {
		t.Fatal("quero erro com lista vazia")
	}
}
