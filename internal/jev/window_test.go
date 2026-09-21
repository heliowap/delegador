package jev

import (
	"strings"
	"testing"
)

func TestEstimateTokensIsRoughlyCharsOverFour(t *testing.T) {
	got := EstimateTokens(strings.Repeat("a", 400))
	if got < 80 || got > 120 {
		t.Errorf("EstimateTokens(400 chars) = %d, quero entre 80 e 120", got)
	}
}

func TestStateBudgetSubtractsLongestQuestion(t *testing.T) {
	qs := map[string]Question{
		"curta": Noul{Instructions: "ok?"},
		"longa": Noul{Instructions: strings.Repeat("x", 4000)}, // ~1000 tokens
	}
	got, err := StateBudget(qs, DefaultLimits())
	if err != nil {
		t.Fatalf("StateBudget: %v", err)
	}
	// 32000 menos a maior pergunta (~1000), com folga para o envelope JSON.
	if got > 31_100 || got < 29_000 {
		t.Errorf("StateBudget = %d, quero perto de 31000", got)
	}
}

func TestStateBudgetErrorsWhenQuestionsAloneExceed(t *testing.T) {
	qs := map[string]Question{"gigante": Noul{Instructions: strings.Repeat("x", 200_000)}}
	if _, err := StateBudget(qs, DefaultLimits()); err == nil {
		t.Fatal("quero erro quando a pergunta sozinha estoura o teto")
	}
}

func TestSplitByBudgetGroupsItems(t *testing.T) {
	items := []string{"aaaa", "bbbb", "cccc", "dddd"} // 1 token cada
	chunks, err := SplitByBudget(items, func(s string) int { return EstimateTokens(s) }, 2)
	if err != nil {
		t.Fatalf("SplitByBudget: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, quero 2", len(chunks))
	}
	if len(chunks[0].Items) != 2 || len(chunks[1].Items) != 2 {
		t.Errorf("distribuicao errada: %+v", chunks)
	}
}

func TestSplitByBudgetErrorsOnOversizedItem(t *testing.T) {
	items := []string{strings.Repeat("x", 4000)}
	if _, err := SplitByBudget(items, func(s string) int { return EstimateTokens(s) }, 10); err == nil {
		t.Fatal("quero erro quando um item sozinho nao cabe")
	}
}
