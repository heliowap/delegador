package cascade

import (
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/verify"
)

func vetoDe(sinal string) agent.Outcome {
	return agent.Outcome{Stop: "veto", Veto: &agent.Veto{Signal: sinal}}
}

// Revisao do invariante, 2026-09-21. O texto antigo tratava todo veto como
// falha de ambiente. Para teto de custo e recusa de permissao isso e
// verdade; para sem_escrita nao e — o watchdog MEDIU o executor empacado,
// que e a situacao que a cascata existe para resolver.
//
// Medido em oito execucoes sobre bugs reais do expr-lang/expr: a cascata
// nao disparou nenhuma vez, e dois dos tres fracassos foram sem_escrita.
func TestEstagnacaoComVerificacaoVermelhaEscala(t *testing.T) {
	for _, sinal := range []string{"sem_escrita", "comando_repetido", "sem_progresso"} {
		t.Run(sinal, func(t *testing.T) {
			d := Avaliar(vetoDe(sinal), vermelho(), 0, DefaultConfig())
			if !d.Escala {
				t.Fatalf("%s com verificacao vermelha deveria escalar", sinal)
			}
			if !strings.Contains(d.Motivo, sinal) {
				t.Errorf("o motivo precisa nomear a parada: %q", d.Motivo)
			}
			if !strings.Contains(d.Motivo, "vermelha") {
				t.Errorf("o motivo precisa nomear a prova da falha: %q", d.Motivo)
			}
		})
	}
}

// O que se preservou do invariante: estagnacao nao decide sozinha. Ela
// deixa de curto-circuitar e cai na MESMA verificacao. Um run que empacou
// depois de deixar tudo verde e provado nao escala — nao ha falha.
func TestEstagnacaoComVerificacaoVerdeNaoEscala(t *testing.T) {
	d := Avaliar(vetoDe("sem_escrita"), verde(), 0, DefaultConfig())
	if d.Escala {
		t.Errorf("verde e provado nao escala, mesmo tendo empacado no fim: %q", d.Motivo)
	}
}

// Orcamento continua sem escalar: o teto e NOSSO, e escalar depois de
// estoura-lo e pagar mais caro por um limite que nos mesmos pusemos.
func TestOrcamentoEAmbienteContinuamSemEscalar(t *testing.T) {
	for _, sinal := range []string{"teto_de_custo", "fora_do_escopo"} {
		d := Avaliar(vetoDe(sinal), vermelho(), 0, DefaultConfig())
		if d.Escala {
			t.Errorf("%s nao pode escalar: %q", sinal, d.Motivo)
		}
		if !strings.Contains(d.Motivo, sinal) {
			t.Errorf("o motivo precisa nomear o veto: %q", d.Motivo)
		}
	}
	// teto_de_turnos nao e veto: o laco termina com Stop proprio.
	d := Avaliar(agent.Outcome{Stop: "teto_de_turnos"}, vermelho(), 0, DefaultConfig())
	if d.Escala {
		t.Errorf("teto de turnos e orcamento nosso: %q", d.Motivo)
	}
}

// A lista e fechada: sinal novo nao vira motivo de escalada por omissao.
func TestSinalDesconhecidoNaoEscala(t *testing.T) {
	d := Avaliar(vetoDe("sinal_que_ainda_nao_existe"), vermelho(), 0, DefaultConfig())
	if d.Escala {
		t.Error("sinal fora da lista nao pode escalar sem alguem ter decidido isso")
	}
}

// Estagnacao tambem respeita o teto de escaladas.
func TestEstagnacaoRespeitaOTetoDeEscaladas(t *testing.T) {
	cfg := DefaultConfig()
	d := Avaliar(vetoDe("sem_escrita"), vermelho(), cfg.MaxEscaladas, cfg)
	if d.Escala {
		t.Error("teto de escaladas vale para estagnacao tambem")
	}
}

// Entrega sem prova de mutacao, depois de empacar, tambem escala — e o
// motivo diz as duas coisas.
func TestEstagnacaoSemProvaDeMutacaoEscala(t *testing.T) {
	// Verde de verdade — a sonda de mutacao rodou e falhou como se espera —
	// mas MutationProved false: o teste nao prova que pega o defeito.
	semProva := verify.Report{
		Steps: []verify.Step{
			{Name: "teste", ExitCode: 0},
			{Name: "mutacao", ExitCode: 1, ExpectFail: true},
		},
		MutationProved: false,
	}
	if !semProva.Green() {
		t.Fatal("a fixture precisa estar verde para exercer o ramo da mutacao")
	}
	d := Avaliar(vetoDe("comando_repetido"), semProva, 0, DefaultConfig())
	if !d.Escala {
		t.Fatalf("entrega sem prova escala: %q", d.Motivo)
	}
	if !strings.Contains(d.Motivo, "comando_repetido") || !strings.Contains(d.Motivo, "mutacao") {
		t.Errorf("motivo incompleto: %q", d.Motivo)
	}
}
