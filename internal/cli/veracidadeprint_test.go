package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/veracidade"
)

// Relatorio inteiro sustentado vira UMA linha: o relatorio do run ja e
// longo, e repetir o que confere empurra para baixo o que nao confere.
func TestTudoConfereViraUmaLinha(t *testing.T) {
	var b bytes.Buffer
	escreveVeracidade(&b, []veracidade.Veredito{
		{Comando: "go test ./...", Estado: veracidade.Sustentado, Confianca: 0.97},
		{Comando: "go vet ./...", Estado: veracidade.Sustentado, Confianca: 0.93},
	})
	if n := strings.Count(strings.TrimSpace(b.String()), "\n"); n != 0 {
		t.Errorf("quero uma linha so, tenho:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "2 de 2") {
		t.Errorf("a linha precisa dizer quantos conferem: %q", b.String())
	}
}

// Comando citado e nunca executado precisa aparecer nomeado.
func TestDivergenciaApareceNomeada(t *testing.T) {
	var b bytes.Buffer
	escreveVeracidade(&b, []veracidade.Veredito{
		{Comando: "go test ./...", Estado: veracidade.SemExecucao},
		{Comando: "go vet ./...", Estado: veracidade.Sustentado, Confianca: 0.93},
	})
	out := b.String()
	if !strings.Contains(out, "DIVERGE") {
		t.Errorf("a divergencia precisa ser visivel na primeira linha: %q", out)
	}
	if !strings.Contains(out, "go test ./...") || !strings.Contains(out, "nenhuma execucao") {
		t.Errorf("o comando e o motivo precisam aparecer: %q", out)
	}
	if strings.Contains(out, "go vet") {
		t.Errorf("o que confere nao precisa de linha propria: %q", out)
	}
}

// Sem conferencia (relatorio ausente, Jev fora) nao se imprime nada: nao
// dizer nada e diferente de dizer que esta tudo bem.
func TestSemConferenciaNaoImprimeNada(t *testing.T) {
	var b bytes.Buffer
	escreveVeracidade(&b, nil)
	if b.Len() != 0 {
		t.Errorf("quero saida vazia, tenho %q", b.String())
	}
}

// So omissao nao e divergencia: ninguem afirmou nada falso.
func TestSoOmissaoNaoAcusa(t *testing.T) {
	var b bytes.Buffer
	escreveVeracidade(&b, []veracidade.Veredito{
		{Comando: "go vet ./...", Estado: veracidade.NaoMencionado},
	})
	if strings.Contains(b.String(), "DIVERGE") {
		t.Errorf("omissao virou acusacao: %q", b.String())
	}
}

// Incerto nao e acusacao. Medido em 2026-09-22: duas conferencias a 0,77 e
// 0,78 — logo abaixo do corte — saiam sob o cabecalho "DIVERGE", e o
// relatorio acusava o executor de algo que ninguem afirmou.
func TestSoInconclusivoNaoAcusa(t *testing.T) {
	var b bytes.Buffer
	escreveVeracidade(&b, []veracidade.Veredito{
		{Comando: "go test ./...", Estado: veracidade.Sustentado, Confianca: 0.97},
		{Comando: "go vet ./...", Estado: veracidade.Incerto, Confianca: 0.78},
	})
	out := b.String()
	if strings.Contains(out, "DIVERGE") {
		t.Errorf("inconclusivo virou acusacao: %q", out)
	}
	if !strings.Contains(out, "inconclusivo") {
		t.Errorf("a linha precisa dizer que ha inconclusivo: %q", out)
	}
	if !strings.Contains(out, "go vet") {
		t.Errorf("o comando inconclusivo precisa ser nomeado: %q", out)
	}
}
