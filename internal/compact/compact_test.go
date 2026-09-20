package compact

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

type porID struct{ manter map[string]bool }

func (b porID) Ask(_ context.Context, state any, _ map[string]jev.Question) (jev.Result, error) {
	id := state.(map[string]any)["interacao"].(map[string]any)["id"].(string)
	v := 0.1
	if b.manter[id] {
		v = 0.95
	}
	raw, _ := json.Marshal(map[string]any{
		"chamada_necessaria":            map[string]any{"type": "noul", "noul": v},
		"resultado_necessario_verbatim": map[string]any{"type": "noul", "noul": v}})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 10}}, nil
}

func amostra() []agent.Turn {
	mk := func(i int, id, cmd, out string) agent.Turn {
		return agent.Turn{Index: i,
			Message: llm.Message{ToolCalls: []tools.Call{{Name: "exec",
				Args: map[string]string{"command": cmd, "_id": id}}}},
			Results: []tools.Result{{Output: out}}}
	}
	return []agent.Turn{
		mk(0, "t0", "ls", "total 4"),
		mk(1, "t1", "go test ./x/", "FAIL: TestSoma"),
		mk(2, "t2", "edit soma.go", "1 hunk"),
	}
}

// Delecao, nunca reescrita: o que fica, fica palavra por palavra.
func TestTurnsDeletaSemReescrever(t *testing.T) {
	kept, _, err := Turns(context.Background(), porID{manter: map[string]bool{"t1": true, "t2": true}},
		"tarefa", amostra())
	if err != nil {
		t.Fatalf("Turns: %v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("quero 2 turnos mantidos, tenho %d", len(kept))
	}
	if kept[0].Results[0].Output != "FAIL: TestSoma" {
		t.Errorf("resultado foi reescrito: %q", kept[0].Results[0].Output)
	}
}

func TestTurnsTruncaDeclarandoOTruncamento(t *testing.T) {
	ruido := strings.Repeat("saida de build sem valor de conferencia\n", 200)
	turns := []agent.Turn{{Index: 0,
		Message: llm.Message{ToolCalls: []tools.Call{{Name: "exec",
			Args: map[string]string{"command": "go build", "_id": "t0"}}}},
		Results: []tools.Result{{Output: ruido}}}}

	// mantem a chamada, descarta o resultado verbatim
	kept, _, err := Turns(context.Background(), meioAMeio{}, "tarefa", turns)
	if err != nil {
		t.Fatalf("Turns: %v", err)
	}
	out := kept[0].Results[0].Output
	if len(out) >= len(ruido) {
		t.Error("deveria ter truncado")
	}
	if !strings.Contains(out, "truncado") {
		t.Errorf("truncamento silencioso esconde perda: %q", out)
	}
}

type meioAMeio struct{}

func (meioAMeio) Ask(context.Context, any, map[string]jev.Question) (jev.Result, error) {
	raw, _ := json.Marshal(map[string]any{
		"chamada_necessaria":            map[string]any{"type": "noul", "noul": 0.9},
		"resultado_necessario_verbatim": map[string]any{"type": "noul", "noul": 0.1}})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 5}}, nil
}
