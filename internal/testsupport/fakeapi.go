// Package testsupport reúne os dublês de teste do delegador.
package testsupport

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Scenario é o roteiro do servidor falso: respostas consumidas em ordem, uma
// por requisição. Pedir mais do que o roteiro prevê é erro de teste — o laço
// está em loop — e o servidor responde 500 em vez de inventar resposta.
type Scenario []Reply

// Reply é uma resposta roteirizada de /chat/completions.
type Reply struct {
	Content          string
	ToolCalls        []ToolCall
	ReasoningContent string
	FinishReason     string
}

// ToolCall é uma chamada de ferramenta roteirizada. O ID volta inalterado na
// resposta: é por ele que o laço casa o resultado da ferramenta com a chamada.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// Request é uma requisição que o servidor falso recebeu, para asserção.
type Request struct {
	Model    string
	Messages []map[string]any
	Tools    []map[string]any
}

// StartFakeAPI sobe um servidor httptest que responde o Scenario em ordem e
// devolve a base URL e um ponteiro para as requisições recebidas. O slice só
// deve ser lido depois que as requisições sob asserção já completaram.
func StartFakeAPI(t *testing.T, s Scenario) (baseURL string, requests *[]Request) {
	t.Helper()

	// httptest serve concorrente: o índice do roteiro e o registro de
	// requisições dividem o mesmo mutex.
	var (
		mu   sync.Mutex
		next int
		reqs []Request
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "rota desconhecida: "+r.URL.Path, http.StatusNotFound)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "lendo corpo: "+err.Error(), http.StatusBadRequest)
			return
		}
		var in struct {
			Model    string           `json:"model"`
			Messages []map[string]any `json:"messages"`
			Tools    []map[string]any `json:"tools"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			http.Error(w, "corpo não é JSON válido: "+err.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()
		reqs = append(reqs, Request{Model: in.Model, Messages: in.Messages, Tools: in.Tools})
		if next >= len(s) {
			mu.Unlock()
			http.Error(w, "cenário esgotado", http.StatusInternalServerError)
			return
		}
		reply := s[next]
		idx := next
		next++
		mu.Unlock()

		writeReply(w, in.Model, reply, idx, len(raw))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &reqs
}

// writeReply monta a resposta no formato que o proxy devolve na sondagem:
// choices[0].message com content, tool_calls e reasoning_content opcionais,
// choices[0].finish_reason e usage com os dois contadores de tokens.
func writeReply(w http.ResponseWriter, model string, reply Reply, idx, promptBytes int) {
	msg := map[string]any{
		"role":    "assistant",
		"content": reply.Content,
	}
	if len(reply.ToolCalls) > 0 {
		calls := make([]any, len(reply.ToolCalls))
		for i, tc := range reply.ToolCalls {
			calls[i] = map[string]any{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": tc.Arguments,
				},
			}
		}
		msg["tool_calls"] = calls
	}
	if reply.ReasoningContent != "" {
		msg["reasoning_content"] = reply.ReasoningContent
	}

	// Estimativa bytes/4 com piso 1: o ledger só precisa de números não nulos
	// e determinísticos, não de um tokenizador.
	msgJSON, _ := json.Marshal(msg)
	prompt := max(promptBytes/4, 1)
	completion := max(len(msgJSON)/4, 1)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      fmt.Sprintf("chatcmpl-fake-%d", idx),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       msg,
			"finish_reason": reply.FinishReason,
		}},
		"usage": map[string]any{
			"prompt_tokens":     prompt,
			"completion_tokens": completion,
			"total_tokens":      prompt + completion,
		},
	})
}
