// Package llm implementa o cliente OpenAI-compatível que o laço do executor
// usa para falar com o proxy: /chat/completions, tool calls e usage.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/heliowap/delegador/internal/tools"
)

// Message é uma mensagem do histórico da conversa. ToolCalls carrega as
// chamadas pedidas pelo assistente; ToolCallID é preenchido nas mensagens
// "tool" com o id da chamada que o resultado responde.
type Message struct {
	Role             string
	Content          string
	ToolCalls        []tools.Call
	ToolCallID       string
	ReasoningContent string
}

// Response é a resposta completa de uma requisição.
type Response struct {
	Message      Message
	FinishReason string
	Usage        Usage
}

// Usage é o consumo de tokens reportado pela API. O ledger conta o que a
// API reporta, não uma estimativa local.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Options configura o cliente.
type Options struct {
	BaseURL    string
	APIKey     string
	HTTP       *http.Client
	MaxRetries int

	// backoffBase e zerado nos testes para nao dormir.
	backoffBase time.Duration
}

// Client fala com uma API OpenAI-compatível de chat completions.
type Client struct {
	apiKey      string
	baseURL     string
	http        *http.Client
	maxRetries  int
	backoffBase time.Duration
}

// New cria o cliente. A chave nunca e gravada nem impressa.
func New(o Options) *Client {
	c := &Client{
		apiKey:      o.APIKey,
		baseURL:     strings.TrimRight(o.BaseURL, "/"),
		http:        o.HTTP,
		maxRetries:  o.MaxRetries,
		backoffBase: o.backoffBase,
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 60 * time.Second}
	}
	if c.maxRetries == 0 {
		c.maxRetries = 3
	}
	if c.backoffBase == 0 {
		c.backoffBase = 500 * time.Millisecond
	}
	return c
}

// Complete pede uma completion. O modelo vai no corpo; tools só entra no
// pedido quando toolSchemas é não vazio, porque alguns endpoints rejeitam
// o campo vazio.
func (c *Client) Complete(ctx context.Context, model string, msgs []Message, toolSchemas []map[string]any) (Response, error) {
	reqBody := map[string]any{
		"model":    model,
		"messages": wireMessages(msgs),
	}
	if len(toolSchemas) > 0 {
		reqBody["tools"] = toolSchemas
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return Response{}, fmt.Errorf("llm: serializando requisicao: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			d := c.backoffBase * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return Response{}, ctx.Err()
			case <-time.After(d):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return Response{}, fmt.Errorf("llm: montando requisicao: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("llm: conexao: %w", err)
			continue
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusOK:
			if readErr != nil {
				return Response{}, fmt.Errorf("llm: lendo resposta: %w", readErr)
			}
			return parseResponse(raw)

		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			// transitorio: tenta de novo
			lastErr = fmt.Errorf("llm: status %d", resp.StatusCode)

		case falhaDeUpstream(raw):
			// 400 que na verdade e falha de UPSTREAM. Um proxy que agrega
			// varias contas devolve o erro do backend com o proprio codigo
			// dele, e a mesma requisicao que falha agora passa no minuto
			// seguinte, quando a rotacao cai noutra conta.
			//
			// Medido em 2026-09-21 no cpa-ocgo-glm-5.3-flash: seis 400
			// seguidos com `"message":"Upstream request failed"`, e doze
			// OK na mesma forma de requisicao minutos depois. Sem esta
			// clausula, um desses matava o laco no turno ZERO — e o run
			// inteiro com ele.
			lastErr = fmt.Errorf("llm: status %d (falha de upstream): %s",
				resp.StatusCode, truncate(string(raw), 200))

		default:
			// 401, 422 e afins nao melhoram com retry. A chave nunca entra
			// na mensagem; so o status e o corpo da API.
			return Response{}, fmt.Errorf("llm: status %d: %s", resp.StatusCode, truncate(string(raw), 300))
		}
	}
	return Response{}, fmt.Errorf("llm: esgotadas %d tentativas: %w", c.maxRetries, lastErr)
}

// wireMessages converte o histórico para o formato do pedido: role e content
// sempre; tool_calls quando o assistente pediu ferramentas; tool_call_id nas
// mensagens "tool". ReasoningContent não volta ao servidor — os endpoints
// de raciocínio pedem que ele não seja reenviado.
func wireMessages(msgs []Message) []map[string]any {
	out := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		w := map[string]any{
			"role":    m.Role,
			"content": m.Content,
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]any, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				calls[j] = map[string]any{
					// O id da chamada viaja em Args["_id"] — tools.Call
					// não tem campo próprio para ele.
					"id":   tc.Args["_id"],
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": wireArguments(tc.Args),
					},
				}
			}
			w["tool_calls"] = calls
		}
		if m.ToolCallID != "" {
			w["tool_call_id"] = m.ToolCallID
		}
		out[i] = w
	}
	return out
}

// wireArguments serializa os argumentos da chamada de volta para JSON. Os
// marcadores internos ("_id", "_parse_error") não saem na linha.
func wireArguments(args map[string]string) string {
	out := make(map[string]string, len(args))
	for k, v := range args {
		if strings.HasPrefix(k, "_") {
			continue
		}
		out[k] = v
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

// response é o formato que o proxy devolve: choices[0].message com content,
// tool_calls e reasoning_content opcionais, finish_reason e usage.
type response struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

func parseResponse(raw []byte) (Response, error) {
	var out response
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, fmt.Errorf("llm: resposta nao e JSON valido: %w", err)
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("llm: resposta sem choices")
	}
	ch := out.Choices[0]
	r := Response{
		Message: Message{
			Role:             "assistant",
			Content:          ch.Message.Content,
			ReasoningContent: ch.Message.ReasoningContent,
		},
		FinishReason: ch.FinishReason,
		Usage:        out.Usage,
	}
	for _, tc := range ch.Message.ToolCalls {
		r.Message.ToolCalls = append(r.Message.ToolCalls, parseCall(tc.ID, tc.Function.Name, tc.Function.Arguments))
	}
	return r, nil
}

// parseCall converte uma tool call do formato da API para tools.Call. O id
// vai para Args["_id"] para o laço casar o resultado depois. Argumentos que
// não parseiam não são erro de Go: a chamada chega com "_parse_error"
// preenchido e o laço devolve isso ao modelo como resultado de ferramenta.
func parseCall(id, name, rawArgs string) tools.Call {
	call := tools.Call{Name: name, Args: map[string]string{"_id": id}}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &parsed); err != nil {
		call.Args["_parse_error"] = fmt.Sprintf("argumentos invalidos (%v): %s", err, truncate(rawArgs, 200))
		return call
	}
	for k, v := range parsed {
		if s, ok := v.(string); ok {
			call.Args[k] = s
		} else {
			// número, booleano ou aninhado: normaliza para JSON em string,
			// que é o formato que Args carrega.
			b, _ := json.Marshal(v)
			call.Args[k] = string(b)
		}
	}
	return call
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// falhaDeUpstream reconhece o corpo em que o proxy repassa um erro do
// backend com codigo proprio. O reconhecimento e pelo TEXTO de propósito:
// o status nao distingue "sua requisicao esta errada" de "a conta la atras
// recusou", e tratar todo 400 como transitorio esconderia requisicao
// malformada atras de quatro tentativas iguais.
func falhaDeUpstream(corpo []byte) bool {
	return bytes.Contains(corpo, []byte("Upstream request failed"))
}
