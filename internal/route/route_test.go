// internal/route/route_test.go
package route

import (
	"testing"

	"github.com/heliowap/delegador/internal/roster"
)

func m(id string, cod, intel, tau, custoTarefa, precoMTok float64) roster.Model {
	p := precoMTok
	return roster.Model{ID: id, Habilitado: true, CustoUSDPorMTok: &p,
		Sondado: roster.Probe{ToolCall: true},
		Benchmark: &roster.Benchmark{CodingIndex: cod, IntelligenceIndex: intel,
			TauBench: tau, CustoPorTarefaUSD: custoTarefa}}
}

func candidatos() []roster.Model {
	return []roster.Model{
		m("glm", 71.5, 41.8, 0.758, 0.0061, 0.15),
		m("deepseek", 69.1, 34.3, 0.730, 0.0075, 0.11),
		m("fable", 81.6, 53.4, 0.783, 0.7261, 5.00),
		m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50),
	}
}

// Entre os que passam o corte, vence o menor custo POR TAREFA — nao o menor
// preco por token. Verbosidade e custo.
func TestEscolheMaisBaratoPorTarefaAcimaDoCorte(t *testing.T) {
	e, err := Escolher(candidatos(), Mecanica, 0.25)
	if err != nil {
		t.Fatalf("Escolher: %v", err)
	}
	if e.Modelo.ID != "glm" {
		t.Errorf("ID = %q, quero glm", e.Modelo.ID)
	}
}

func TestCorteAltoExcluiOsBaratos(t *testing.T) {
	e, err := Escolher(candidatos(), Mecanica, 0.90)
	if err != nil {
		t.Fatalf("Escolher: %v", err)
	}
	if e.Modelo.ID != "fable" {
		t.Errorf("ID = %q, quero fable (maior coding_index)", e.Modelo.ID)
	}
}

// Dimensao agentica usa tau_bench, nao coding_index.
func TestDimensaoAgenticaUsaTauBench(t *testing.T) {
	e, _ := Escolher(candidatos(), Agentica, 0.95)
	if e.Modelo.ID != "opus" {
		t.Errorf("ID = %q, quero opus (maior tau_bench)", e.Modelo.ID)
	}
}

// Percentil e dentro do roster: os indices nao sao comparaveis entre si.
// coding_index vai a 81.6; agentic_index a 57.9. Corte absoluto seria erro.
func TestPercentilEhRelativoAoRoster(t *testing.T) {
	poucos := []roster.Model{m("a", 10, 10, 0.10, 0.01, 0.1), m("b", 12, 12, 0.12, 0.02, 0.1)}
	e, err := Escolher(poucos, Mecanica, 0.90)
	if err != nil {
		t.Fatalf("roster fraco ainda deve escolher alguem: %v", err)
	}
	if e.Modelo.ID != "b" {
		t.Errorf("ID = %q, quero b", e.Modelo.ID)
	}
}

// Sem nota entra por viabilidade e custo, marcado. Ausencia de nota nao e
// nota baixa, e tratar como zero excluiria o modelo para sempre.
func TestSemBenchmarkEntraMarcado(t *testing.T) {
	c := 0.0
	semNota := roster.Model{ID: "swe-2", Habilitado: true, CustoUSDPorMTok: &c,
		Sondado: roster.Probe{ToolCall: true}, Benchmark: nil}

	e, err := Escolher([]roster.Model{semNota}, Mecanica, 0.50)
	if err != nil {
		t.Fatalf("Escolher: %v", err)
	}
	if e.Modelo.ID != "swe-2" || !e.NaoMedido {
		t.Errorf("escolha = %+v; quero swe-2 marcado como nao medido", e)
	}
	if e.Motivo == "" {
		t.Error("o relatorio precisa saber por que um nao medido foi escolhido")
	}
}

func TestSemCandidatoDaErro(t *testing.T) {
	if _, err := Escolher(nil, Mecanica, 0.5); err == nil {
		t.Error("quero erro com roster vazio")
	}
}
