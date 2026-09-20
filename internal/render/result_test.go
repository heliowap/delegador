package render

import (
	"bytes"
	"testing"

	"github.com/heliowap/delegador/internal/route"
	"github.com/heliowap/delegador/internal/verify"
)

// A flag de divergencia e o motivo de o plugin existir: relatorio afirmando
// verde contra exit code que discorda.
func TestFlagaRelatorioVerdeComSuiteVermelha(t *testing.T) {
	var b bytes.Buffer
	Result(&b, Input{
		Verify: verify.Report{Steps: []verify.Step{
			{Name: "teste", Command: "go test ./...", ExitCode: 1, Stdout: "FAIL: TestSoma"}}},
		AfirmaVerde: 0.95})

	if !bytes.Contains(b.Bytes(), []byte("DIVERGENCIA")) {
		t.Errorf("falta a flag:\n%s", b.String())
	}
	if !bytes.Contains(b.Bytes(), []byte("FAIL: TestSoma")) {
		t.Error("a saida real do teste precisa aparecer")
	}
}

func TestFlagaMutacaoQueNaoProvaNada(t *testing.T) {
	var b bytes.Buffer
	Result(&b, Input{Verify: verify.Report{
		Steps: []verify.Step{{Name: "teste", ExitCode: 0}}, MutationProved: false}})
	if !bytes.Contains(b.Bytes(), []byte("mutacao")) {
		t.Errorf("mutacao que nao provou nada precisa aparecer:\n%s", b.String())
	}
}

func TestSemDivergenciaNaoFlaga(t *testing.T) {
	var b bytes.Buffer
	Result(&b, Input{Verify: verify.Report{
		Steps:          []verify.Step{{Name: "teste", ExitCode: 0}, {Name: "suite", ExitCode: 0}},
		MutationProved: true}, AfirmaVerde: 0.95})
	if bytes.Contains(b.Bytes(), []byte("DIVERGENCIA")) {
		t.Errorf("sem divergencia nao deveria flagar:\n%s", b.String())
	}
}

// Modelo nao medido escolhido precisa aparecer no relatorio: quem le tem de
// saber que a escolha nao teve nota por tras.
func TestDizQuandoModeloNaoTemNota(t *testing.T) {
	var b bytes.Buffer
	Result(&b, Input{Verify: verify.Report{Steps: []verify.Step{{Name: "teste", ExitCode: 0}}},
		Escolha: routeEscolhaNaoMedida()})
	if !bytes.Contains(b.Bytes(), []byte("nao medido")) {
		t.Errorf("escolha sem nota precisa ser declarada:\n%s", b.String())
	}
}

func routeEscolhaNaoMedida() route.Escolha {
	return route.Escolha{NaoMedido: true}
}
