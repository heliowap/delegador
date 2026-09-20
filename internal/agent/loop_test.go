// internal/agent/loop_test.go
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/testsupport"
	"github.com/heliowap/delegador/internal/tools"
)

func setup(t *testing.T, s testsupport.Scenario) (*llm.Client, *tools.Registry, Config) {
	t.Helper()
	url, _ := testsupport.StartFakeAPI(t, s)
	wt := t.TempDir()
	return llm.New(llm.Options{BaseURL: url, APIKey: "k"}),
		&tools.Registry{},
		Config{Model: "x", MaxTurns: 10, Policy: tools.Policy{
			Worktree: wt, WritePrefixes: []string{""}, AllowCommands: []string{"go test"}}}
}

func TestRunExecutesToolCallAndFeedsResultBack(t *testing.T) {
	c, reg, cfg := setup(t, testsupport.Scenario{
		{ToolCalls: []testsupport.ToolCall{{ID: "c1", Name: "write_file",
			Arguments: `{"path":"a.txt","content":"oi"}`}}, FinishReason: "tool_calls"},
		{ToolCalls: []testsupport.ToolCall{{ID: "c2", Name: "read_file",
			Arguments: `{"path":"a.txt"}`}}, FinishReason: "tool_calls"},
		{Content: "arquivo diz oi", FinishReason: "stop"},
	})

	out, err := Run(context.Background(), c, reg, cfg, "escreva e leia", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Stop != "final" {
		t.Errorf("Stop = %q, quero final", out.Stop)
	}
	if len(out.Turns) != 3 {
		t.Fatalf("quero 3 turnos, tenho %d", len(out.Turns))
	}
	if !strings.Contains(out.Turns[1].Results[0].Output, "oi") {
		t.Errorf("o resultado da leitura nao voltou: %+v", out.Turns[1].Results)
	}
	if out.Final != "arquivo diz oi" {
		t.Errorf("Final = %q", out.Final)
	}
}

// Recusa nao mata o laco: e a correcao central do v2.
func TestRunContinuesAfterDeniedCall(t *testing.T) {
	c, reg, cfg := setup(t, testsupport.Scenario{
		{ToolCalls: []testsupport.ToolCall{{ID: "c1", Name: "exec",
			Arguments: `{"command":"git push"}`}}, FinishReason: "tool_calls"},
		{Content: "entendi, nao posso fazer push", FinishReason: "stop"},
	})
	cfg.Policy.AllowCommands = append(cfg.Policy.AllowCommands, "git push")

	out, err := Run(context.Background(), c, reg, cfg, "suba o codigo", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Stop != "final" {
		t.Fatalf("recusa nao deveria encerrar o laco: Stop = %q", out.Stop)
	}
	r := out.Turns[0].Results[0]
	if !r.IsError || r.Output == "" {
		t.Errorf("a recusa deveria voltar como resultado com motivo: %+v", r)
	}
}

func TestRunStopsAtTurnCap(t *testing.T) {
	var s testsupport.Scenario
	for i := 0; i < 20; i++ {
		s = append(s, testsupport.Reply{
			ToolCalls:    []testsupport.ToolCall{{ID: "c", Name: "list_dir", Arguments: `{"path":"."}`}},
			FinishReason: "tool_calls"})
	}
	c, reg, cfg := setup(t, s)
	cfg.MaxTurns = 4

	out, _ := Run(context.Background(), c, reg, cfg, "explore", nil)
	if out.Stop != "teto_de_turnos" {
		t.Errorf("Stop = %q, quero teto_de_turnos", out.Stop)
	}
	if len(out.Turns) != 4 {
		t.Errorf("quero exatamente 4 turnos, tenho %d", len(out.Turns))
	}
}

func TestRunHonorsVeto(t *testing.T) {
	var s testsupport.Scenario
	for i := 0; i < 10; i++ {
		s = append(s, testsupport.Reply{
			ToolCalls:    []testsupport.ToolCall{{ID: "c", Name: "list_dir", Arguments: `{"path":"."}`}},
			FinishReason: "tool_calls"})
	}
	c, reg, cfg := setup(t, s)

	pre := func(turns []Turn) *Veto {
		if len(turns) >= 2 {
			return &Veto{Signal: "sem_progresso", Excerpt: "ls repetido", Probability: 0.9}
		}
		return nil
	}
	out, _ := Run(context.Background(), c, reg, cfg, "explore", pre)
	if out.Stop != "veto" {
		t.Errorf("Stop = %q, quero veto", out.Stop)
	}
	if len(out.Turns) != 2 {
		t.Errorf("o veto deveria cortar no turno 2, tenho %d", len(out.Turns))
	}
}

func TestRunAccumulatesUsageAcrossTurns(t *testing.T) {
	c, reg, cfg := setup(t, testsupport.Scenario{
		{ToolCalls: []testsupport.ToolCall{{ID: "c1", Name: "list_dir", Arguments: `{"path":"."}`}},
			FinishReason: "tool_calls"},
		{Content: "pronto", FinishReason: "stop"},
	})
	out, _ := Run(context.Background(), c, reg, cfg, "x", nil)
	if out.Usage.PromptTokens == 0 {
		t.Error("o uso total precisa somar os turnos")
	}
}
