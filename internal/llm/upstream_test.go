package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// Medido em 2026-09-21 no cpa-ocgo-glm-5.3-flash: seis 400 seguidos com
// "Upstream request failed" e doze OK na mesma forma de requisicao minutos
// depois. E a rotacao de contas do proxy caindo numa conta ruim. Sem retry,
// um desses matava o laco no turno ZERO e levava o run inteiro.
func TestRetentaQuatrocentosDeUpstream(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) <= 2 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"param":"reasoning","type":"invalid_request_error",` +
				`"message":"Upstream request failed: [invalid_request_error] unknown field"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},` +
			`"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, MaxRetries: 4})
	resp, err := c.Complete(context.Background(), "m", []Message{{Role: "user", Content: "oi"}}, nil)
	if err != nil {
		t.Fatalf("quero recuperacao apos os 400 de upstream: %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Errorf("conteudo = %q", resp.Message.Content)
	}
	if n < 3 {
		t.Errorf("quero 3 tentativas, houve %d", n)
	}
}

// 400 que e culpa NOSSA nao melhora com retry, e insistir esconderia a
// requisicao malformada atras de quatro tentativas iguais.
func TestQuatrocentosProprioNaoRetenta(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"messages: field required"}}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, MaxRetries: 4})
	if _, err := c.Complete(context.Background(), "m", []Message{{Role: "user"}}, nil); err == nil {
		t.Fatal("quero erro")
	}
	if n != 1 {
		t.Errorf("quero UMA tentativa, houve %d", n)
	}
}
