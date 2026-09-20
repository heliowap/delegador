// internal/agent/precondition_test.go
package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

type stubAsker struct{ noProgress float64 }

func (s stubAsker) Ask(context.Context, any, map[string]jev.Question) (jev.Result, error) {
	raw, _ := json.Marshal(map[string]any{
		"sem_progresso": map[string]any{"type": "noul", "noul": s.noProgress}})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 50}}, nil
}

func turnsRepeating(n int) []Turn {
	var out []Turn
	for i := 0; i < n; i++ {
		out = append(out, Turn{Index: i,
			Message: llm.Message{ToolCalls: []tools.Call{{Name: "exec",
				Args: map[string]string{"command": "go test ./x/"}}}},
			Results: []tools.Result{{Output: "FAIL: no module", IsError: true}}})
	}
	return out
}

// Deterministico: mesma chamada, mesmo resultado, N vezes. Sem modelo.
func TestVetoesRepeatedIdenticalCall(t *testing.T) {
	cfg := DefaultPreConfig()
	pre := NewPrecondition(cfg, nil, func() float64 { return 0 })

	if v := pre(turnsRepeating(cfg.RepeatThreshold - 1)); v != nil {
		t.Error("abaixo do limiar nao deveria vetar")
	}
	v := pre(turnsRepeating(cfg.RepeatThreshold))
	if v == nil {
		t.Fatal("repeticao identica deveria vetar")
	}
	if v.Excerpt == "" {
		t.Error("o veto precisa carregar o trecho que o causou")
	}
	if v.Probability != 1 {
		t.Errorf("Probability = %v; sinal deterministico vale 1", v.Probability)
	}
}

// Falhar, mudar de abordagem e falhar de novo nao e travar.
func TestDoesNotVetoDifferentCalls(t *testing.T) {
	cfg := DefaultPreConfig()
	pre := NewPrecondition(cfg, nil, func() float64 { return 0 })

	var turns []Turn
	for i, cmd := range []string{"go test ./a/", "go test ./b/", "go test ./c/"} {
		turns = append(turns, Turn{Index: i,
			Message: llm.Message{ToolCalls: []tools.Call{{Name: "exec",
				Args: map[string]string{"command": cmd}}}},
			Results: []tools.Result{{Output: "FAIL", IsError: true}}})
	}
	if pre(turns) != nil {
		t.Error("comandos diferentes nao caracterizam repeticao")
	}
}

func TestVetoesOnCostCap(t *testing.T) {
	cfg := DefaultPreConfig()
	cfg.CostCapUSD = 0.50
	pre := NewPrecondition(cfg, nil, func() float64 { return 0.51 })

	v := pre([]Turn{{Index: 0}})
	if v == nil || v.Signal != "teto_de_custo" {
		t.Fatalf("teto de custo deveria vetar: %+v", v)
	}
}

// sem_progresso exige duas janelas consecutivas: um turno ruim e normal.
func TestSemanticVetoNeedsTwoConsecutiveWindows(t *testing.T) {
	cfg := DefaultPreConfig()
	cfg.RepeatThreshold = 99 // isola o sinal semantico
	pre := NewPrecondition(cfg, stubAsker{noProgress: 0.95}, func() float64 { return 0 })

	if v := pre(turnsRepeating(1)); v != nil {
		t.Error("uma janela so nao deveria vetar")
	}
	if v := pre(turnsRepeating(2)); v == nil || v.Signal != "sem_progresso" {
		t.Errorf("duas janelas consecutivas deveriam vetar: %+v", v)
	}
}

func TestSemanticVetoIgnoresLowProbability(t *testing.T) {
	cfg := DefaultPreConfig()
	cfg.RepeatThreshold = 99
	pre := NewPrecondition(cfg, stubAsker{noProgress: 0.2}, func() float64 { return 0 })

	pre(turnsRepeating(1))
	if v := pre(turnsRepeating(2)); v != nil {
		t.Errorf("probabilidade abaixo do limiar nao veta: %+v", v)
	}
}

// Sem Asker, so os sinais deterministicos operam — e o laco segue vivo.
func TestNilAskerDisablesOnlySemanticSignal(t *testing.T) {
	cfg := DefaultPreConfig()
	cfg.RepeatThreshold = 99
	pre := NewPrecondition(cfg, nil, func() float64 { return 0 })
	if v := pre(turnsRepeating(5)); v != nil {
		t.Errorf("sem Asker e sem repeticao, nao ha veto: %+v", v)
	}
}
