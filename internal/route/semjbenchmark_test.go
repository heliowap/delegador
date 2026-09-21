package route

import (
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/roster"
)

func semBenchmark(id string, limiar float64, c roster.Conta) roster.Model {
	return roster.Model{ID: id, Habilitado: true, Sondado: roster.Probe{ToolCall: true},
		AdmiteSemBenchmark: limiar, Conta: c}
}

// O corte de percentil mede JULGAMENTO. Briefing que fecha as decisoes nao
// pede julgamento, pede execucao fiel — e um modelo proprietario sem
// benchmark publico ficava fora da rota por falta de DADO, nao de
// capacidade.
func TestSemBenchmarkEntraQuandoOBriefingFechaAsDecisoes(t *testing.T) {
	ms := []roster.Model{
		m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50, 27.50),
		semBenchmark("devin/swe-2", 0.85, promo),
	}
	ms[0].Conta = orq

	// Tarefa fechada: o sem-benchmark entra e vence pela conta.
	fechada, err := EscolherCom(ms, Raciocinio, 0.9, Opcoes{Autocontida: 0.92})
	if err != nil {
		t.Fatal(err)
	}
	if fechada.Modelo.ID != "devin/swe-2" {
		t.Errorf("ID = %q, quero devin/swe-2 em tarefa fechada", fechada.Modelo.ID)
	}
	if !strings.Contains(fechada.Motivo, "sem benchmark") {
		t.Errorf("o relatorio precisa dizer por que ele entrou: %q", fechada.Motivo)
	}

	// Tarefa com decisao em aberto: a porta fica fechada.
	aberta, err := EscolherCom(ms, Raciocinio, 0.9, Opcoes{Autocontida: 0.30})
	if err != nil {
		t.Fatal(err)
	}
	if aberta.Modelo.ID == "devin/swe-2" {
		t.Error("tarefa aberta exige julgamento, e julgamento e o que nao foi medido")
	}
}

// Autocontencao nao informada mantem a porta fechada: a admissao depende de
// uma medida do gate, e sem ela nao ha o que julgar.
func TestSemAutocontencaoAPortaFicaFechada(t *testing.T) {
	ms := []roster.Model{
		m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50, 27.50),
		semBenchmark("devin/swe-2", 0.85, promo),
	}
	e, err := Escolher(ms, Raciocinio, 0.9) // sem Opcoes
	if err != nil {
		t.Fatal(err)
	}
	if e.Modelo.ID == "devin/swe-2" {
		t.Error("sem a medida de autocontencao ninguem entra por essa porta")
	}
}

// Sem o campo declarado, nada muda: a porta e uma decisao de quem monta o
// roster, nao um comportamento novo que se liga sozinho.
func TestSemOCampoDeclaradoNadaMuda(t *testing.T) {
	ms := []roster.Model{
		m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50, 27.50),
		semBenchmark("mudo", 0, promo),
	}
	e, err := EscolherCom(ms, Raciocinio, 0.9, Opcoes{Autocontida: 0.99})
	if err != nil {
		t.Fatal(err)
	}
	if e.Modelo.ID != "opus" {
		t.Errorf("ID = %q: quem nao declarou nao entra", e.Modelo.ID)
	}
}

// A porta tem duas chaves, e a segunda veio de medir a primeira sozinha:
// admitir so por autocontencao entregava TUDO ao sem-benchmark, porque a
// preferencia de conta domina o desempate e ele nao tem numero de qualidade
// com que ser comparado.
func TestPortaTambemOlhaAComplexidade(t *testing.T) {
	swe := semBenchmark("devin/swe-2", 0.75, promo)
	swe.AdmiteAtePercentil = 0.50
	opus := m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50, 27.50)
	opus.Conta = orq
	ms := []roster.Model{opus, swe}

	facil, err := EscolherCom(ms, Raciocinio, 0.30, Opcoes{Autocontida: 0.80})
	if err != nil {
		t.Fatal(err)
	}
	if facil.Modelo.ID != "devin/swe-2" {
		t.Errorf("fechada e simples: quero swe-2, tenho %q", facil.Modelo.ID)
	}

	dificil, err := EscolherCom(ms, Raciocinio, 0.90, Opcoes{Autocontida: 0.80})
	if err != nil {
		t.Fatal(err)
	}
	if dificil.Modelo.ID == "devin/swe-2" {
		t.Error("fechada mas dificil: decisao fechada nao torna o trabalho facil")
	}
}
