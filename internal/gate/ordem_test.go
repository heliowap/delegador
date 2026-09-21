package gate

import (
	"strings"
	"testing"
)

// Medido em 2026-09-21: o template mandava "escreva primeiro o teste" mesmo
// quando a tarefa dizia que os testes ja existiam. O modelo obedeceu ao
// template, escreveu um teste novo e nunca chegou a implementacao.
func TestOrdemMudaQuandoOsTestesJaExistem(t *testing.T) {
	ev := []Evidence{{Kind: "trecho", Ref: "pkg/mat/soma.go:5", Text: "return a - b"}}

	normal := BuildBriefing("x", ev, BriefingLimits{TestCmd: "go test ./..."})
	if !strings.Contains(normal, "Escreva primeiro o teste") {
		t.Error("sem TestesJaEscritos, a ordem TDD original deve aparecer")
	}

	pronto := BuildBriefing("x", ev, BriefingLimits{
		TestCmd: "go test ./...", TestesJaEscritos: true,
		ArquivosDeTeste: []string{"pkg/mat/soma_test.go"}})
	if strings.Contains(pronto, "Escreva primeiro o teste") {
		t.Error("com os testes prontos, o briefing nao pode mandar escreve-los")
	}
	if !strings.Contains(pronto, "pkg/mat/soma_test.go") {
		t.Error("precisa nomear os arquivos de teste que sao o criterio")
	}
	if !strings.Contains(pronto, "nao os altere") {
		t.Error("precisa proibir alterar o oraculo")
	}
}

// A disciplina do vermelho vale nos dois casos: e ela que separa teste que
// prova de teste que acompanha.
func TestConfirmarOVermelhoValeNosDoisCasos(t *testing.T) {
	ev := []Evidence{{Kind: "trecho", Ref: "a.go:1", Text: "x"}}
	for _, pronto := range []bool{false, true} {
		b := BuildBriefing("x", ev, BriefingLimits{TestCmd: "go test ./...", TestesJaEscritos: pronto})
		if !strings.Contains(b, "confirme o vermelho") {
			t.Errorf("TestesJaEscritos=%v: o briefing perdeu a confirmacao do vermelho", pronto)
		}
	}
}

// O template falava com duas vozes: mandava nao escrever testes e depois
// pedia "o teste que voce escreveu".
func TestRelatorioNaoContradizAOrdem(t *testing.T) {
	ev := []Evidence{{Kind: "trecho", Ref: "a.go:1", Text: "x"}}
	pronto := BuildBriefing("x", ev, BriefingLimits{TestCmd: "go test ./...", TestesJaEscritos: true})
	if strings.Contains(pronto, "o teste que voce escreveu") {
		t.Error("com os testes prontos, o relatorio nao pode pedir o teste que o modelo escreveu")
	}
	normal := BuildBriefing("x", ev, BriefingLimits{TestCmd: "go test ./..."})
	if !strings.Contains(normal, "o teste que voce escreveu") {
		t.Error("no fluxo normal o relatorio deve pedir o teste escrito")
	}
}
