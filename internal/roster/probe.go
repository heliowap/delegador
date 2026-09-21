// probe.go — a coleta externa do roster: sondagem de viabilidade contra o
// proxy e cache dos benchmarks de terceiro.
package roster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

// ProbeModel faz uma chamada mínima com as ferramentas reais e mede o que a
// sondagem registra: se o modelo emite tool call, se emite reasoning_content
// e qual o piso de tokens do prompt (com o schema inteiro, que é o que o
// laço manda de verdade).
func ProbeModel(ctx context.Context, c *llm.Client, id string) (Probe, error) {
	inicio := time.Now()
	resp, err := c.Complete(ctx, id, []llm.Message{{
		Role:    "user",
		Content: "Use a ferramenta read_file para ler o arquivo go.mod.",
	}}, (&tools.Registry{}).Schemas())
	if err != nil {
		return Probe{}, err
	}
	return Probe{
		ToolCall:         len(resp.Message.ToolCalls) > 0,
		ReasoningContent: resp.Message.ReasoningContent != "",
		TokensBase:       resp.Usage.PromptTokens,
		LatenciaS:        time.Since(inicio).Seconds(),
		Em:               time.Now(),
	}, nil
}

// benchmarksBaseURL é o endpoint padrão; OPENROUTER_BASE_URL sobrepõe e é
// lido na chamada, não no init — os testes apontam para httptest.
const benchmarksBaseURL = "https://openrouter.ai/api/v1"

// FetchBenchmarks baixa /benchmarks do OpenRouter e junta por permaslug as
// duas fontes que interessam: artificial-analysis (os três índices) e
// openrouter com benchmark_type tau_bench_verified_airline (accuracy,
// desvio e custo por tarefa). design-arena é ignorado — é elo de categoria
// de UI e não serve para rotear código.
//
// O cache em disco existe por causa da cota (30 req/min, 500/dia): dentro
// do TTL nem toca a rede; fora dele, se a rede falhar, o vencido ainda
// serve — cota estourada não invalida o que já se sabe. Só erra quando não
// há nada utilizável.
func FetchBenchmarks(ctx context.Context, apiKey, cachePath string, ttl time.Duration) (map[string]Benchmark, error) {
	dados, mod, cacheErr := readCache(cachePath)
	if cacheErr == nil && time.Since(mod) < ttl {
		return dados, nil
	}

	fresco, asOf, err := fetchBenchmarks(ctx, apiKey)
	if err != nil {
		if cacheErr == nil {
			return dados, nil
		}
		return nil, err
	}
	// Falha ao gravar o cache não é fatal: o dado já está na mão.
	_ = writeCache(cachePath, asOf, fresco)
	return fresco, nil
}

// benchCache é o formato do arquivo: {"as_of": ..., "dados": {permaslug: {...}}}.
type benchCache struct {
	AsOf  string               `json:"as_of"`
	Dados map[string]Benchmark `json:"dados"`
}

// readCache devolve os dados e a data da gravação — é o fetch, não o as_of
// do fornecedor, que mede a frescura.
func readCache(path string) (map[string]Benchmark, time.Time, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	var c benchCache
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, time.Time{}, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	return c.Dados, st.ModTime(), nil
}

func writeCache(path, asOf string, dados map[string]Benchmark) error {
	raw, err := json.Marshal(benchCache{AsOf: asOf, Dados: dados})
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// benchResponse é o envelope do OpenRouter: uma linha por fonte x modelo,
// com os campos de índice e de accuracy misturados — o join é por
// permaslug + source.
type benchResponse struct {
	Data []struct {
		Source            string  `json:"source"`
		Permaslug         string  `json:"model_permaslug"`
		BenchmarkType     string  `json:"benchmark_type"`
		CodingIndex       float64 `json:"coding_index"`
		IntelligenceIndex float64 `json:"intelligence_index"`
		AgenticIndex      float64 `json:"agentic_index"`
		Accuracy          float64 `json:"accuracy"`
		AccuracyStddev    float64 `json:"accuracy_stddev"`
		AvgCostPerTask    float64 `json:"avg_cost_per_task"`
	} `json:"data"`
	Meta struct {
		AsOf string `json:"as_of"`
	} `json:"meta"`
}

func fetchBenchmarks(ctx context.Context, apiKey string) (map[string]Benchmark, string, error) {
	base := os.Getenv("OPENROUTER_BASE_URL")
	if base == "" {
		base = benchmarksBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(base, "/")+"/benchmarks", nil)
	if err != nil {
		return nil, "", fmt.Errorf("roster: montando requisição: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("roster: conexão: %w", err)
	}
	raw, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// A chave nunca entra na mensagem; só o status.
		return nil, "", fmt.Errorf("roster: benchmarks: status %d", resp.StatusCode)
	}
	if readErr != nil {
		return nil, "", fmt.Errorf("roster: lendo resposta: %w", readErr)
	}
	var out benchResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", fmt.Errorf("roster: resposta não é JSON válido: %w", err)
	}

	dados := map[string]Benchmark{}
	for _, row := range out.Data {
		if row.Permaslug == "" {
			continue
		}
		b := dados[row.Permaslug]
		switch {
		case row.Source == "artificial-analysis":
			b.CodingIndex = row.CodingIndex
			b.IntelligenceIndex = row.IntelligenceIndex
			b.AgenticIndex = row.AgenticIndex
		case row.Source == "openrouter" && row.BenchmarkType == "tau_bench_verified_airline":
			b.TauBench = row.Accuracy
			b.TauBenchDesvio = row.AccuracyStddev
			b.CustoPorTarefaUSD = row.AvgCostPerTask
		default:
			continue
		}
		dados[row.Permaslug] = b
	}
	return dados, out.Meta.AsOf, nil
}
