package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// DefaultBaseURL e o endpoint do System One.
const DefaultBaseURL = "https://api.typesafe.ai/v1/systemone"

// ModelAlias e o alias estavel do Jev. Nao ha escolha de modelo aqui:
// o TypeSafe usa os mesmos pesos para todas as contas.
const ModelAlias = "jev-latest"

// Usage e o consumo de tokens de uma requisicao. Saida e gratuita.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Result e a resposta completa de uma requisicao.
type Result struct {
	Answers Answers
	Usage   Usage
}

// Options configura o cliente.
type Options struct {
	APIKey     string
	BaseURL    string
	HTTP       *http.Client
	MaxRetries int

	// backoffBase e zerado nos testes para nao dormir.
	backoffBase time.Duration
}

// Client fala com a API do System One.
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
		baseURL:     o.BaseURL,
		http:        o.HTTP,
		maxRetries:  o.MaxRetries,
		backoffBase: o.backoffBase,
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 60 * time.Second}
	}
	if c.maxRetries == 0 {
		// Medido em 2026-09-21: o Jev devolveu 503 intermitente em janelas de
		// dezenas de segundos, e 3 tentativas com base de 500ms cobriam 3,5s
		// — curto demais. Cinco tentativas com base de 1s cobrem ~31s e
		// atravessam a janela. Um plan faz uma chamada por item de
		// evidencia: um unico 503 derrubava o plan inteiro.
		c.maxRetries = 5
	}
	if c.backoffBase == 0 {
		c.backoffBase = time.Second
	}
	return c
}

type request struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

type response struct {
	Model   string  `json:"model"`
	Answers Answers `json:"answers"`
	Usage   Usage   `json:"usage"`
}

// Ask faz uma requisicao com todas as perguntas juntas. Perguntas de uma
// mesma requisicao sao avaliadas em paralelo e nao veem as respostas umas
// das outras — e por isso que fan-out especulativo funciona.
func (c *Client) Ask(ctx context.Context, state any, qs map[string]Question) (Result, error) {
	if len(qs) == 0 {
		return Result{}, fmt.Errorf("jev: nenhuma pergunta")
	}
	body, err := json.Marshal(request{State: state, Model: ModelAlias, Questions: qs})
	if err != nil {
		return Result{}, fmt.Errorf("jev: serializando requisicao: %w", err)
	}

	var lastErr error
	var retryAfter time.Duration
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			d := c.backoffBase * time.Duration(1<<(attempt-1))
			// Jitter de ate 25%: varias chamadas do mesmo plan falham juntas,
			// e sem jitter elas voltam juntas e derrubam de novo.
			if d > 0 {
				d += time.Duration(rand.Int63n(int64(d)/4 + 1))
			}
			if ra := retryAfter; ra > 0 && ra > d {
				d = ra // o servidor sabe melhor que o nosso backoff
			}
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			case <-time.After(d):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
		if err != nil {
			return Result{}, fmt.Errorf("jev: montando requisicao: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("jev: conexao: %w", err)
			continue
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusOK:
			if readErr != nil {
				return Result{}, fmt.Errorf("jev: lendo resposta: %w", readErr)
			}
			var out response
			if err := json.Unmarshal(raw, &out); err != nil {
				return Result{}, fmt.Errorf("jev: resposta nao e JSON valido: %w", err)
			}
			return Result{Answers: out.Answers, Usage: out.Usage}, nil

		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			retryAfter = 0
			if v := resp.Header.Get("Retry-After"); v != "" {
				if secs, err := strconv.Atoi(v); err == nil && secs > 0 && secs <= 120 {
					retryAfter = time.Duration(secs) * time.Second
				}
			}
			// transitorio: tenta de novo
			lastErr = fmt.Errorf("jev: status %d", resp.StatusCode)

		default:
			// 401, 422 e afins nao melhoram com retry. A chave nunca entra
			// na mensagem; so o status e o corpo da API.
			return Result{}, fmt.Errorf("jev: status %d: %s", resp.StatusCode, truncate(string(raw), 300))
		}
	}
	return Result{}, fmt.Errorf("jev: esgotadas %d tentativas: %w", c.maxRetries, lastErr)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
