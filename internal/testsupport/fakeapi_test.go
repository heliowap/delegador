package testsupport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func post(t *testing.T, url string, body map[string]any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(url+"/chat/completions", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFakeAPIServesRepliesInOrder(t *testing.T) {
	url, _ := StartFakeAPI(t, Scenario{
		{ToolCalls: []ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path":"go.mod"}`}}, FinishReason: "tool_calls"},
		{Content: "pronto", FinishReason: "stop"},
	})

	first := post(t, url, map[string]any{"model": "x", "messages": []any{}})
	ch := first["choices"].([]any)[0].(map[string]any)
	if ch["finish_reason"] != "tool_calls" {
		t.Errorf("primeira resposta: finish_reason = %v", ch["finish_reason"])
	}
	tc := ch["message"].(map[string]any)["tool_calls"].([]any)
	if len(tc) != 1 {
		t.Fatalf("quero 1 tool call, tenho %d", len(tc))
	}

	second := post(t, url, map[string]any{"model": "x", "messages": []any{}})
	ch2 := second["choices"].([]any)[0].(map[string]any)
	if ch2["message"].(map[string]any)["content"] != "pronto" {
		t.Errorf("segunda resposta: %v", ch2["message"])
	}
}

func TestFakeAPIRecordsRequests(t *testing.T) {
	url, reqs := StartFakeAPI(t, Scenario{{Content: "ok", FinishReason: "stop"}})

	post(t, url, map[string]any{
		"model":    "modelo-x",
		"messages": []any{map[string]any{"role": "user", "content": "oi"}},
		"tools":    []any{map[string]any{"type": "function"}},
	})

	if len(*reqs) != 1 {
		t.Fatalf("quero 1 requisicao registrada, tenho %d", len(*reqs))
	}
	r := (*reqs)[0]
	if r.Model != "modelo-x" {
		t.Errorf("Model = %q", r.Model)
	}
	if len(r.Messages) != 1 || len(r.Tools) != 1 {
		t.Errorf("mensagens/ferramentas nao registradas: %+v", r)
	}
}

// Cenario esgotado e erro de teste, nao resposta silenciosa: laco que pede
// mais turnos do que o roteiro previu esta em loop, e o teste tem que acusar.
func TestFakeAPIExhaustedScenarioReturns500(t *testing.T) {
	url, _ := StartFakeAPI(t, Scenario{{Content: "ok", FinishReason: "stop"}})
	post(t, url, map[string]any{"model": "x", "messages": []any{}})

	raw, _ := json.Marshal(map[string]any{"model": "x", "messages": []any{}})
	resp, err := http.Post(url+"/chat/completions", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, quero 500 com cenario esgotado", resp.StatusCode)
	}
}

func TestFakeAPIReportsUsage(t *testing.T) {
	url, _ := StartFakeAPI(t, Scenario{{Content: "ok", FinishReason: "stop"}})
	out := post(t, url, map[string]any{"model": "x", "messages": []any{}})
	u, ok := out["usage"].(map[string]any)
	if !ok || u["prompt_tokens"] == nil || u["completion_tokens"] == nil {
		t.Errorf("usage ausente ou incompleto: %v", out["usage"])
	}
}
