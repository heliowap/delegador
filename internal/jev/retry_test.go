package jev

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// Medido em 2026-09-21: o Jev devolveu 503 intermitente em janelas de dezenas
// de segundos e derrubou cinco de seis execucoes de uma matriz de eval. Com
// 3 tentativas de base 500ms a cobertura era de 3,5s.
func TestAtravessaJanelaDe503(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1) <= 4 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"model":"jev","answers":{"a":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1}}`))
	}))
	defer srv.Close()

	c := New(Options{APIKey: "k", BaseURL: srv.URL, backoffBase: time.Millisecond})
	if _, err := c.Ask(context.Background(), "x", map[string]Question{"a": Noul{Instructions: "?"}}); err != nil {
		t.Fatalf("quatro 503 seguidos deveriam ser atravessados: %v", err)
	}
	if got := n.Load(); got != 5 {
		t.Errorf("chamadas = %d, quero 5", got)
	}
}

func TestPadraoCobreJanelaLonga(t *testing.T) {
	c := New(Options{APIKey: "k"})
	if c.maxRetries < 5 {
		t.Errorf("maxRetries = %d; janela curta demais para 503 intermitente", c.maxRetries)
	}
	var total time.Duration
	for i := 1; i <= c.maxRetries; i++ {
		total += c.backoffBase * time.Duration(1<<(i-1))
	}
	if total < 25*time.Second {
		t.Errorf("cobertura de %v; medido que a janela de instabilidade passa de 25s", total)
	}
}

// O servidor sabe melhor que o nosso backoff.
func TestHonraRetryAfter(t *testing.T) {
	var n atomic.Int32
	var quando []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		quando = append(quando, time.Now())
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", strconv.Itoa(1))
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"model":"jev","answers":{"a":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1}}`))
	}))
	defer srv.Close()

	c := New(Options{APIKey: "k", BaseURL: srv.URL, backoffBase: time.Millisecond})
	if _, err := c.Ask(context.Background(), "x", map[string]Question{"a": Noul{Instructions: "?"}}); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(quando) < 2 {
		t.Fatal("quero duas chamadas")
	}
	if esperou := quando[1].Sub(quando[0]); esperou < 900*time.Millisecond {
		t.Errorf("esperou %v; o Retry-After de 1s deveria vencer o backoff de 1ms", esperou)
	}
}

// Retry-After absurdo nao pode travar o processo.
func TestIgnoraRetryAfterAbsurdo(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "99999")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"model":"jev","answers":{"a":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1}}`))
	}))
	defer srv.Close()

	c := New(Options{APIKey: "k", BaseURL: srv.URL, backoffBase: time.Millisecond})
	feito := make(chan error, 1)
	go func() {
		_, err := c.Ask(context.Background(), "x", map[string]Question{"a": Noul{Instructions: "?"}})
		feito <- err
	}()
	select {
	case err := <-feito:
		if err != nil {
			t.Fatalf("Ask: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Retry-After absurdo travou o processo")
	}
}
