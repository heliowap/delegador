package cascade

import (
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/verify"
)

func verde() verify.Report {
	return verify.Report{Steps: []verify.Step{{Name: "teste", ExitCode: 0}}, MutationProved: true}
}
func vermelho() verify.Report {
	return verify.Report{Steps: []verify.Step{{Name: "teste", ExitCode: 1}}}
}

func TestNaoEscalaComVerificacaoVerde(t *testing.T) {
	d := Avaliar(agent.Outcome{Stop: "final"}, verde(), 0, DefaultConfig())
	if d.Escala {
		t.Error("verde nao escala")
	}
}

func TestEscalaComVerificacaoVermelha(t *testing.T) {
	cfg := DefaultConfig()
	d := Avaliar(agent.Outcome{Stop: "final"}, vermelho(), 0, cfg)
	if !d.Escala {
		t.Fatal("falha de verificacao deveria escalar")
	}
	if d.NovoPercentil <= 0 {
		t.Error("a escalada precisa elevar o corte")
	}
}

// A regra mais importante: recusa de permissao e teto de custo nao sao falha
// do modelo. Escalar aqui e pagar caro por erro de ambiente.
func TestNaoEscalaPorVetoDeCusto(t *testing.T) {
	d := Avaliar(agent.Outcome{Stop: "veto", Veto: &agent.Veto{Signal: "teto_de_custo"}},
		vermelho(), 0, DefaultConfig())
	if d.Escala {
		t.Error("teto de custo nao e falha do modelo")
	}
	if d.Motivo == "" {
		t.Error("o relatorio precisa saber por que nao escalou")
	}
}

func TestNaoEscalaQuandoNadaFoiExecutado(t *testing.T) {
	d := Avaliar(agent.Outcome{Stop: "erro"}, verify.Report{}, 0, DefaultConfig())
	if d.Escala {
		t.Error("erro de execucao nao e falha de qualidade")
	}
}

func TestRespeitaTetoDeEscaladas(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxEscaladas != 1 {
		t.Fatalf("o padrao deve ser uma escalada, e %d", cfg.MaxEscaladas)
	}
	d := Avaliar(agent.Outcome{Stop: "final"}, vermelho(), cfg.MaxEscaladas, cfg)
	if d.Escala {
		t.Error("teto atingido nao escala; o caso vai para o humano")
	}
}

// Teste que passa com a correcao desfeita nao prova nada — e motivo de
// escalada tanto quanto suite vermelha.
func TestEscalaQuandoMutacaoNaoProvaNada(t *testing.T) {
	rep := verify.Report{Steps: []verify.Step{{Name: "teste", ExitCode: 0}}, MutationProved: false}
	if d := Avaliar(agent.Outcome{Stop: "final"}, rep, 0, DefaultConfig()); !d.Escala {
		t.Error("mutacao que nao prova nada deveria escalar")
	}
}

// Sonda pulada (sem TestCmd ou erro de infra) tambem escala, mas o Motivo
// precisa nomear a causa real: nao e mutacao improdutiva, e entrega sem
// prova porque a sonda nao rodou.
func TestEscalaQuandoSondaNaoRodou(t *testing.T) {
	rep := verify.Report{Steps: []verify.Step{
		{Name: "teste", ExitCode: 0},
		{Name: "mutacao", Skipped: true},
	}, MutationProved: false}
	d := Avaliar(agent.Outcome{Stop: "final"}, rep, 0, DefaultConfig())
	if !d.Escala {
		t.Fatal("entrega sem prova deveria escalar")
	}
	if !strings.Contains(d.Motivo, "nao rodou") {
		t.Errorf("Motivo %q deveria nomear a sonda que nao rodou", d.Motivo)
	}
}
