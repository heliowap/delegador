package jev

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAskSendsCorrectPayload(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer k-123" {
			t.Errorf("Authorization = %q", auth)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, `{"model":"jev-1.13.0","answers":{"urgente":{"type":"noul","noul":0.91}},"usage":{"input_tokens":120,"output_tokens":0}}`)
	}))
	defer srv.Close()

	c := New(Options{APIKey: "k-123", BaseURL: srv.URL})
	res, err := c.Ask(context.Background(), "pagamentos falhando ha 3 dias", map[string]Question{
		"urgente": Noul{Instructions: "O texto transmite urgencia?"},
	})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}

	if got["model"] != "jev-latest" {
		t.Errorf("model = %v, quero jev-latest", got["model"])
	}
	qs := got["questions"].(map[string]any)["urgente"].(map[string]any)
	if qs["type"] != "noul" {
		t.Errorf("type = %v, quero noul", qs["type"])
	}

	p, ok := res.Answers.NoulOf("urgente")
	if !ok {
		t.Fatal("resposta urgente ausente")
	}
	if p != 0.91 {
		t.Errorf("noul = %v, quero 0.91", p)
	}
	if res.Usage.InputTokens != 120 {
		t.Errorf("InputTokens = %d, quero 120", res.Usage.InputTokens)
	}
}

func TestAskParsesChoiceAndScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"model":"jev-1.13.0","answers":{
			"rota":{"type":"choice","choice":"smart","confidence":0.8,"probabilities":{"smart":0.8,"auto":0.2}},
			"complexidade":{"type":"score","score":1.43,"confidence":0.6,"probabilities":[0.0,0.57,0.43],"legend":{"0":"trivial","1":"media","2":"alta"}}
		},"usage":{"input_tokens":10,"output_tokens":0}}`)
	}))
	defer srv.Close()

	c := New(Options{APIKey: "k", BaseURL: srv.URL})
	res, err := c.Ask(context.Background(), "x", map[string]Question{
		"rota":         Choice{Instructions: "Qual modo?", Criteria: map[string]string{"smart": "roda teste", "auto": "so leitura"}},
		"complexidade": Score{Instructions: "Quao complexa?", Criteria: []string{"trivial", "media", "alta"}},
	})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}

	ch, ok := res.Answers.ChoiceOf("rota")
	if !ok || ch.Choice != "smart" || ch.Confidence != 0.8 {
		t.Errorf("ChoiceOf = %+v, ok=%v", ch, ok)
	}
	sc, ok := res.Answers.ScoreOf("complexidade")
	if !ok || sc.Score != 1.43 {
		t.Errorf("ScoreOf = %+v, ok=%v", sc, ok)
	}
}

func TestAskRetriesOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{"model":"jev","answers":{"a":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1}}`)
	}))
	defer srv.Close()

	c := New(Options{APIKey: "k", BaseURL: srv.URL, MaxRetries: 5, backoffBase: time.Nanosecond})
	if _, err := c.Ask(context.Background(), "x", map[string]Question{"a": Noul{Instructions: "?"}}); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("chamadas = %d, quero 3", n)
	}
}

// 401 nao e transitorio: falhar rapido, e sem vazar a chave na mensagem.
func TestAskDoesNotRetryOn401AndHidesKey(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(Options{APIKey: "sk-supersecreta", BaseURL: srv.URL, MaxRetries: 5, backoffBase: time.Nanosecond})
	_, err := c.Ask(context.Background(), "x", map[string]Question{"a": Noul{Instructions: "?"}})
	if err == nil {
		t.Fatal("quero erro em 401")
	}
	if calls.Load() != 1 {
		t.Errorf("chamadas = %d, quero 1 (sem retry)", calls.Load())
	}
	if strings.Contains(err.Error(), "sk-supersecreta") {
		t.Error("a mensagem de erro vazou a chave")
	}
}
