// internal/roster/roster_test.go
package roster

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/testsupport"
)

func TestLoadReadsRealRoster(t *testing.T) {
	ms, err := Load(filepath.Join("..", "..", "config", "roster.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ms) != 6 {
		t.Fatalf("quero 6 modelos, tenho %d", len(ms))
	}
	byID := map[string]Model{}
	for _, m := range ms {
		byID[m.ID] = m
	}
	glm, ok := byID["cpa-fw-glm-5.3-flash"]
	if !ok {
		t.Fatal("glm ausente")
	}
	if glm.Benchmark == nil || glm.Benchmark.TauBench != 0.758 {
		t.Errorf("benchmark do glm: %+v", glm.Benchmark)
	}
	if glm.Sondado.TokensBase != 162 {
		t.Errorf("TokensBase = %d, quero 162", glm.Sondado.TokensBase)
	}
	swe := byID["devin/swe-2"]
	if swe.Benchmark != nil {
		t.Error("swe-2 nao tem benchmark; nil e o valor certo, nao zero")
	}
	if swe.Papel != "barato" {
		t.Errorf("Papel do swe-2 = %q, quero barato", swe.Papel)
	}
}

// Custo nulo nao vira zero: modelo sem custo declarado nao compete por preco.
// REESCRITO em 2026-09-21. A versao anterior lia config/roster.yaml e
// afirmava "nenhum tem custo preenchido, logo 0 elegiveis" — o que e um fato
// sobre o CONTEUDO do arquivo, nao sobre o comportamento da funcao. Quando os
// custos foram preenchidos, o teste quebrou sem que nada de logica mudasse.
//
// A intencao — custo nulo nao vira zero, e modelo sem custo declarado nao
// compete — esta preservada inteira, agora sobre fixture sintetica.
func TestElegiveisExcluiSemCusto(t *testing.T) {
	agora := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	custo := 0.15
	sondado := Probe{ToolCall: true, Em: agora}

	semCusto := Model{ID: "sem-custo", Habilitado: true, Sondado: sondado}
	comCusto := Model{ID: "com-custo", Habilitado: true, Sondado: sondado, CustoUSDPorMTok: &custo}

	ok, motivos := Elegiveis([]Model{semCusto, comCusto}, 30*24*time.Hour, agora)
	if len(ok) != 1 || ok[0].ID != "com-custo" {
		t.Errorf("elegiveis = %v; so o que declara custo deveria passar", ok)
	}
	if len(motivos) != 1 {
		t.Fatalf("quero 1 motivo de exclusao, tenho %d: %v", len(motivos), motivos)
	}
	if !strings.Contains(motivos[0], "sem-custo") {
		t.Errorf("o motivo precisa nomear quem foi excluido: %q", motivos[0])
	}
}

func TestElegiveisExcluiSondagemVelhaOuSemToolCall(t *testing.T) {
	custo := 0.15
	base := Model{ID: "m", Habilitado: true, CustoUSDPorMTok: &custo,
		Sondado: Probe{ToolCall: true, Em: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)}}
	agora := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	velho, motivos := Elegiveis([]Model{base}, 30*24*time.Hour, agora)
	if len(velho) != 0 || len(motivos) != 1 {
		t.Errorf("sondagem velha deveria excluir: %v / %v", velho, motivos)
	}

	semTool := base
	semTool.Sondado.ToolCall = false
	semTool.Sondado.Em = agora
	if ok, _ := Elegiveis([]Model{semTool}, 30*24*time.Hour, agora); len(ok) != 0 {
		t.Error("modelo sem tool call nao executa tarefa nenhuma")
	}

	bom := base
	bom.Sondado.Em = agora
	if ok, _ := Elegiveis([]Model{bom}, 30*24*time.Hour, agora); len(ok) != 1 {
		t.Error("modelo sondado, habilitado e com custo deveria passar")
	}
}

func TestProbeModelMedeCapacidades(t *testing.T) {
	url, _ := testsupport.StartFakeAPI(t, testsupport.Scenario{{
		ToolCalls:        []testsupport.ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path":"go.mod"}`}},
		ReasoningContent: "pensando",
		FinishReason:     "tool_calls",
	}})
	p, err := ProbeModel(context.Background(), llm.New(llm.Options{BaseURL: url, APIKey: "k"}), "x")
	if err != nil {
		t.Fatalf("ProbeModel: %v", err)
	}
	if !p.ToolCall || !p.ReasoningContent || p.TokensBase == 0 || p.Em.IsZero() {
		t.Errorf("sondagem incompleta: %+v", p)
	}
}

// O cache existe por causa da cota: 30 req/min e 500/dia. Segunda chamada
// dentro do TTL nao pode tocar a rede.
func TestFetchBenchmarksUsaCacheDentroDoTTL(t *testing.T) {
	var chamadas int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chamadas++
		io.WriteString(w, `{"data":[{"source":"artificial-analysis",`+
			`"model_permaslug":"z-ai/glm-5.3-flash-20260826","coding_index":71.5,`+
			`"intelligence_index":41.8,"agentic_index":50.9}],`+
			`"meta":{"as_of":"2026-09-20T00:03:27Z"}}`)
	}))
	defer srv.Close()
	t.Setenv("OPENROUTER_BASE_URL", srv.URL)

	cache := filepath.Join(t.TempDir(), "bench.json")
	for i := 0; i < 3; i++ {
		b, err := FetchBenchmarks(context.Background(), "k", cache, time.Hour)
		if err != nil {
			t.Fatalf("FetchBenchmarks: %v", err)
		}
		if b["z-ai/glm-5.3-flash-20260826"].CodingIndex != 71.5 {
			t.Fatalf("indice nao veio: %+v", b)
		}
	}
	if chamadas != 1 {
		t.Errorf("tocou a rede %d vezes; o cache deveria segurar em 1", chamadas)
	}
}

// Cota estourada nao invalida o que ja se sabe: cache vencido e melhor que
// nada, e o chamador precisa saber que esta velho.
func TestFetchBenchmarksDegradaParaCacheVencido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	t.Setenv("OPENROUTER_BASE_URL", srv.URL)

	cache := filepath.Join(t.TempDir(), "bench.json")
	if err := os.WriteFile(cache, []byte(`{"as_of":"2020-01-01T00:00:00Z","dados":`+
		`{"z-ai/glm-5.3-flash-20260826":{"coding_index":71.5}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := FetchBenchmarks(context.Background(), "k", cache, time.Nanosecond)
	if err != nil {
		t.Fatalf("cache vencido com rede fora nao deveria ser erro fatal: %v", err)
	}
	if b["z-ai/glm-5.3-flash-20260826"].CodingIndex != 71.5 {
		t.Error("deveria ter degradado para o cache vencido")
	}
}
