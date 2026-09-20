// internal/llm/client_test.go
package llm

import (
	"context"
	"testing"

	"github.com/heliowap/delegador/internal/testsupport"
)

func TestCompleteParsesToolCalls(t *testing.T) {
	url, reqs := testsupport.StartFakeAPI(t, testsupport.Scenario{{
		ToolCalls:    []testsupport.ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path":"go.mod"}`}},
		FinishReason: "tool_calls",
	}})

	c := New(Options{BaseURL: url, APIKey: "k"})
	r, err := c.Complete(context.Background(), "modelo-x",
		[]Message{{Role: "user", Content: "leia o go.mod"}},
		[]map[string]any{{"type": "function"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if r.FinishReason != "tool_calls" || len(r.Message.ToolCalls) != 1 {
		t.Fatalf("resposta = %+v", r)
	}
	tc := r.Message.ToolCalls[0]
	if tc.Name != "read_file" || tc.Args["path"] != "go.mod" {
		t.Errorf("tool call mal parseada: %+v", tc)
	}
	if (*reqs)[0].Model != "modelo-x" {
		t.Errorf("modelo nao propagado: %q", (*reqs)[0].Model)
	}
}

// Argumento invalido nao pode derrubar o laco: vira erro de ferramenta e o
// modelo tenta de novo. Medido: modelos erram o JSON com alguma frequencia.
func TestCompleteSurvivesMalformedToolArguments(t *testing.T) {
	url, _ := testsupport.StartFakeAPI(t, testsupport.Scenario{{
		ToolCalls:    []testsupport.ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path": `}},
		FinishReason: "tool_calls",
	}})

	c := New(Options{BaseURL: url, APIKey: "k"})
	r, err := c.Complete(context.Background(), "x", []Message{{Role: "user", Content: "y"}}, nil)
	if err != nil {
		t.Fatalf("JSON invalido nos argumentos nao deveria virar erro de Go: %v", err)
	}
	if len(r.Message.ToolCalls) != 1 {
		t.Fatalf("a chamada deveria chegar, mesmo malformada: %+v", r)
	}
	if r.Message.ToolCalls[0].Args["_parse_error"] == "" {
		t.Error("o erro de parse precisa ser sinalizado para virar resultado de ferramenta")
	}
}

func TestCompleteAccumulatesUsage(t *testing.T) {
	url, _ := testsupport.StartFakeAPI(t, testsupport.Scenario{{Content: "ok", FinishReason: "stop"}})
	c := New(Options{BaseURL: url, APIKey: "k"})
	r, _ := c.Complete(context.Background(), "x", []Message{{Role: "user", Content: "y"}}, nil)
	if r.Usage.PromptTokens == 0 {
		t.Error("usage precisa vir preenchido: o ledger conta o que a API reporta")
	}
}

func TestCompleteSendsToolResultsWithID(t *testing.T) {
	url, reqs := testsupport.StartFakeAPI(t, testsupport.Scenario{{Content: "ok", FinishReason: "stop"}})
	c := New(Options{BaseURL: url, APIKey: "k"})
	_, _ = c.Complete(context.Background(), "x", []Message{
		{Role: "user", Content: "leia"},
		{Role: "tool", ToolCallID: "c1", Content: "module exemplo"},
	}, nil)

	msgs := (*reqs)[0].Messages
	last := msgs[len(msgs)-1]
	if last["role"] != "tool" || last["tool_call_id"] != "c1" {
		t.Errorf("resultado de ferramenta sem id nao casa com a chamada: %v", last)
	}
}
