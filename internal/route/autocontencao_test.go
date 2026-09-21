package route

import (
	"testing"

	"github.com/heliowap/delegador/internal/roster"
)

// Quando a tarefa NAO e autocontida, preco deixa de ser o unico criterio
// entre os baratos: o modelo precisa sustentar o enquadramento sozinho.
// tau_bench mede uso de ferramenta em ambiente multi-turno, que e o proxy
// mais proximo disso que existe no benchmark — e e dado, nao palpite.

func comTau(id string, cod, tau, custoTarefa, preco float64) roster.Model {
	p := preco
	return roster.Model{ID: id, Habilitado: true, CustoUSDPorMTok: &p,
		Sondado: roster.Probe{ToolCall: true},
		Benchmark: &roster.Benchmark{CodingIndex: cod, IntelligenceIndex: cod * 0.6,
			TauBench: tau, CustoPorTarefaUSD: custoTarefa}}
}

func semNota(id string, custoTarefa, preco float64) roster.Model {
	p := preco
	return roster.Model{ID: id, Habilitado: true, CustoUSDPorMTok: &p,
		Sondado: roster.Probe{ToolCall: true}, Benchmark: nil}
}

// baratoFragil: barato e bom em codigo, mas fraco em horizonte longo.
// baratoFirme: um pouco mais caro, mas sustenta o enquadramento.
func candidatosAmbiguidade() []roster.Model {
	// fragil e o mais barato e bom em codigo, mas o pior em horizonte longo.
	// firme custa o dobro por tarefa e sustenta o enquadramento.
	return []roster.Model{
		comTau("barato-fragil", 65.0, 0.40, 0.006, 0.15),
		comTau("barato-firme", 71.0, 0.76, 0.012, 0.20),
		comTau("forte", 81.0, 0.79, 0.700, 5.00),
	}
}

// Tarefa autocontida: nada muda, vence o menor custo por tarefa.
func TestAutocontidaMantemOMaisBarato(t *testing.T) {
	e, err := EscolherCom(candidatosAmbiguidade(), Mecanica, 0.0, Opcoes{Autocontida: 0.95})
	if err != nil {
		t.Fatalf("EscolherCom: %v", err)
	}
	if e.Modelo.ID != "barato-fragil" {
		t.Errorf("ID = %q, quero barato-fragil: tarefa fechada, preco decide", e.Modelo.ID)
	}
}

// Tarefa com decisao em aberto: o fragil sai, mesmo sendo o mais barato.
func TestNaoAutocontidaExigePisoDeTauBench(t *testing.T) {
	e, err := EscolherCom(candidatosAmbiguidade(), Mecanica, 0.0, Opcoes{Autocontida: 0.15})
	if err != nil {
		t.Fatalf("EscolherCom: %v", err)
	}
	if e.Modelo.ID == "barato-fragil" {
		t.Error("tarefa ambigua nao deveria cair no modelo que nao sustenta horizonte")
	}
	if e.Modelo.ID != "barato-firme" {
		t.Errorf("ID = %q, quero barato-firme: passa o piso e ainda e o mais barato dos que passam", e.Modelo.ID)
	}
	if e.Motivo == "" {
		t.Error("o relatorio precisa saber por que o mais barato foi preterido")
	}
}

// Sem nota de terceiro nao ha evidencia de que sustenta enquadramento.
// Em tarefa ambigua isso pesa: ausencia de medicao nao vira aposta.
func TestNaoAutocontidaPreterModeloSemNota(t *testing.T) {
	ms := append(candidatosAmbiguidade(), semNota("sem-nota", 0.001, 0.05))

	// Tarefa ambigua: o sem-nota sai da disputa mesmo sendo de longe o mais
	// barato. Ausencia de medicao nao vira aposta quando a tarefa exige
	// sustentar enquadramento.
	aberta, err := EscolherCom(ms, Mecanica, 0.0, Opcoes{Autocontida: 0.15})
	if err != nil {
		t.Fatalf("EscolherCom: %v", err)
	}
	if aberta.Modelo.ID == "sem-nota" {
		t.Error("tarefa ambigua nao deveria cair em modelo sem nenhuma medicao")
	}
}

// Roster so de modelos sem nota: nao se trava o trabalho por falta de
// benchmark, mas o relatorio precisa dizer que a escolha foi as cegas.
func TestSoSemNotaNaoTravaTarefaAmbigua(t *testing.T) {
	e, err := EscolherCom([]roster.Model{semNota("unico", 0.01, 0.1)},
		Mecanica, 0.0, Opcoes{Autocontida: 0.10})
	if err != nil {
		t.Fatalf("nao pode travar: %v", err)
	}
	if e.Modelo.ID != "unico" || !e.NaoMedido {
		t.Errorf("escolha = %+v; quero o unico disponivel, marcado nao medido", e)
	}
}

// Se ninguem passa o piso, nao se trava o trabalho: usa o melhor tau
// disponivel e explica no Motivo.
func TestPisoIntransponivelNaoTravaOTrabalho(t *testing.T) {
	fracos := []roster.Model{
		comTau("a", 60, 0.20, 0.004, 0.10),
		comTau("b", 62, 0.25, 0.005, 0.10),
	}
	e, err := EscolherCom(fracos, Mecanica, 0.20, Opcoes{Autocontida: 0.10})
	if err != nil {
		t.Fatalf("roster fraco nao pode travar: %v", err)
	}
	if e.Modelo.ID != "b" {
		t.Errorf("ID = %q, quero b (melhor tau entre os disponiveis)", e.Modelo.ID)
	}
	if e.Motivo == "" {
		t.Error("precisa dizer que ninguem passou o piso")
	}
}

// Escolher continua funcionando sem opcoes: o oraculo da Task 9 nao muda.
func TestEscolherSemOpcoesEhOComportamentoAntigo(t *testing.T) {
	a, _ := Escolher(candidatosAmbiguidade(), Mecanica, 0.0)
	b, _ := EscolherCom(candidatosAmbiguidade(), Mecanica, 0.0, Opcoes{Autocontida: 1.0})
	if a.Modelo.ID != b.Modelo.ID {
		t.Errorf("Escolher=%q EscolherCom(autocontida=1)=%q; deveriam coincidir", a.Modelo.ID, b.Modelo.ID)
	}
}
