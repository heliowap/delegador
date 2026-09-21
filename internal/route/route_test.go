// internal/route/route_test.go
package route

import (
	"testing"

	"github.com/heliowap/delegador/internal/roster"
)

// precoIn e precoOut sao os precos do BENCHMARK, nao os de quem roda: e
// dividindo o custo por tarefa por eles que TokensPorTarefa recupera o
// tamanho da tarefa em tokens. A proporcao entre entrada e saida importa e
// varia por fornecedor (1:3 no deepseek, 1:5 nos demais do roster real),
// entao ela entra explicita em vez de sair de uma suposicao do helper.
func m(id string, cod, intel, tau, custoTarefa, precoIn, precoOut float64) roster.Model {
	p := precoIn
	return roster.Model{ID: id, Habilitado: true, CustoUSDPorMTok: &p,
		Sondado: roster.Probe{ToolCall: true},
		Benchmark: &roster.Benchmark{CodingIndex: cod, IntelligenceIndex: intel,
			TauBench: tau, CustoPorTarefaUSD: custoTarefa,
			PrecoEntradaUSDPorMTok: precoIn, PrecoSaidaUSDPorMTok: precoOut}}
}

func candidatos() []roster.Model {
	return []roster.Model{
		m("glm", 71.5, 41.8, 0.758, 0.0061, 0.15, 0.50),
		m("deepseek", 69.1, 34.3, 0.730, 0.0075, 0.11, 0.33),
		m("fable", 81.6, 53.4, 0.783, 0.7261, 5.00, 25.00),
		m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50, 27.50),
	}
}

// Entre os que passam o corte, vence quem termina com MENOS TRABALHO —
// tokens por tarefa, nao dolares. Preco nao e criterio de rota: quem monta
// o roster ja decidiu o orcamento escolhendo quais modelos entram.
func TestEscolheMenosTrabalhoAcimaDoCorte(t *testing.T) {
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
	poucos := []roster.Model{m("a", 10, 10, 0.10, 0.01, 0.1, 0.3), m("b", 12, 12, 0.12, 0.02, 0.1, 0.3)}
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

// O caso que o preco errava. Numeros do roster de 2026-09-21: por dolar, o
// deepseek (US$ 0,0075/tarefa) parece 66x melhor que o opus-5 (US$ 0,4930).
// Por trabalho, o deepseek precisa de ~34k tokens para terminar e o opus de
// ~30k — o barato por token e o mais verboso dos dois. Com os dois acima do
// corte, o criterio antigo escolhia deepseek e o novo escolhe opus.
func TestDesempateNaoEPrecoEOTrabalho(t *testing.T) {
	dois := []roster.Model{
		m("deepseek", 69.1, 34.3, 0.730, 0.0075, 0.11, 0.33),
		m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50, 27.50),
	}
	e, err := Escolher(dois, Mecanica, 0)
	if err != nil {
		t.Fatalf("Escolher: %v", err)
	}
	if e.Modelo.ID != "opus" {
		t.Errorf("ID = %q, quero opus: termina com menos tokens apesar de custar mais", e.Modelo.ID)
	}
	if barato, caro := dois[0], dois[1]; barato.Benchmark.CustoPorTarefaUSD >= caro.Benchmark.CustoPorTarefaUSD {
		t.Fatal("a fixture perdeu o sentido: o escolhido precisa ser o mais CARO por tarefa")
	}
}

// Benchmark sem preco nao da para converter em trabalho. Vai para o fim da
// fila em vez de ganhar por um zero.
func TestSemPrecoDoBenchmarkNaoGanhaPorZero(t *testing.T) {
	semPreco := m("mudo", 78.0, 50.8, 0.792, 0.1, 0, 0)
	semPreco.Benchmark.PrecoEntradaUSDPorMTok = 0
	semPreco.Benchmark.PrecoSaidaUSDPorMTok = 0
	e, err := Escolher([]roster.Model{semPreco, m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50, 27.50)}, Mecanica, 0)
	if err != nil {
		t.Fatalf("Escolher: %v", err)
	}
	if e.Modelo.ID != "opus" {
		t.Errorf("ID = %q, quero opus: o sem preco nao pode vencer por falta de dado", e.Modelo.ID)
	}
}
