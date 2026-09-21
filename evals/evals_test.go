// Package evals roda as fixtures rotuladas de fixtures.json contra o Jev
// de verdade (spec §11): sao a medida que recalibra thresholds e texto de
// pergunta. Sem TYPESAFE_API_KEY nao ha nada a medir — o teste pula, e o
// CI passa sem a chave.
package evals

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/heliowap/delegador/internal/jev"
)

// Cada fixture nomeia um conjunto de perguntas e o rotulo esperado: a rota
// traz dimensao e faixa de complexidade, a autonomia e o watchdog trazem o
// lado esperado do noul.
type fixtureSet struct {
	Rota []struct {
		Nome     string `json:"nome"`
		Texto    string `json:"texto"`
		Esperado struct {
			Dimensao        string   `json:"dimensao_dominante"`
			ComplexidadeMin *float64 `json:"complexidade_minima"`
			ComplexidadeMax *float64 `json:"complexidade_maxima"`
		} `json:"esperado"`
	} `json:"rota"`
	Autonomia []struct {
		Nome     string `json:"nome"`
		Texto    string `json:"texto"`
		Esperado struct {
			TarefaAutocontida bool `json:"tarefa_autocontida"`
		} `json:"esperado"`
	} `json:"autonomia"`
	Watchdog []struct {
		Nome     string `json:"nome"`
		Atual    string `json:"atual"`
		Previos  string `json:"previos"`
		Esperado struct {
			SemProgresso bool `json:"sem_progresso"`
		} `json:"esperado"`
	} `json:"watchdog"`
}

func TestFixtures(t *testing.T) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY ausente: evals so roda contra o Jev real (spec §11)")
	}
	raw, err := os.ReadFile("fixtures.json")
	if err != nil {
		t.Fatalf("lendo fixtures.json: %v", err)
	}
	var fx fixtureSet
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("fixtures.json invalido: %v", err)
	}

	client := jev.New(jev.Options{APIKey: key, BaseURL: os.Getenv("TYPESAFE_BASE_URL")})
	ask := func(t *testing.T, state any, qs map[string]jev.Question) jev.Answers {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		res, err := client.Ask(ctx, state, qs)
		if err != nil {
			t.Fatalf("Ask: %v", err)
		}
		return res.Answers
	}

	t.Run("rota", func(t *testing.T) {
		qs := jev.RouteQuestions()
		askRoute := map[string]jev.Question{
			"dimensao_dominante": qs["dimensao_dominante"],
			"complexidade":       qs["complexidade"],
		}
		for _, f := range fx.Rota {
			t.Run(f.Nome, func(t *testing.T) {
				state := map[string]any{"tarefa": map[string]any{"texto": f.Texto}}
				a := ask(t, state, askRoute)

				ch, ok := a.ChoiceOf("dimensao_dominante")
				if !ok {
					t.Fatal("resposta sem dimensao_dominante")
				}
				if ch.Choice != f.Esperado.Dimensao {
					t.Errorf("dimensao_dominante = %q, rotulo diz %q", ch.Choice, f.Esperado.Dimensao)
				}
				sa, ok := a.ScoreOf("complexidade")
				if !ok {
					t.Fatal("resposta sem complexidade")
				}
				if min := f.Esperado.ComplexidadeMin; min != nil && sa.Score < *min {
					t.Errorf("complexidade = %.2f, abaixo da faixa rotulada >= %.2f", sa.Score, *min)
				}
				if max := f.Esperado.ComplexidadeMax; max != nil && sa.Score > *max {
					t.Errorf("complexidade = %.2f, acima da faixa rotulada <= %.2f", sa.Score, *max)
				}
			})
		}
	})

	t.Run("autonomia", func(t *testing.T) {
		for _, f := range fx.Autonomia {
			t.Run(f.Nome, func(t *testing.T) {
				// O fixture serve de briefing: a pergunta le briefing.texto.
				state := map[string]any{
					"tarefa":   map[string]any{"texto": f.Texto},
					"briefing": map[string]any{"texto": f.Texto},
				}
				a := ask(t, state, jev.AutonomyQuestion())
				p, ok := a.NoulOf("tarefa_autocontida")
				if !ok {
					t.Fatal("resposta sem tarefa_autocontida")
				}
				if got := p >= 0.5; got != f.Esperado.TarefaAutocontida {
					t.Errorf("tarefa_autocontida = %.2f, rotulo diz %v", p, f.Esperado.TarefaAutocontida)
				}
			})
		}
	})

	t.Run("watchdog", func(t *testing.T) {
		for _, f := range fx.Watchdog {
			t.Run(f.Nome, func(t *testing.T) {
				// Mesma janela que a pre-condicao monta: turno_atual contra
				// os previos, texto bruto da rodada.
				state := map[string]any{"janela": map[string]any{
					"turno_atual":    map[string]any{"index": 1, "texto": f.Atual},
					"turnos_previos": []any{map[string]any{"index": 0, "texto": f.Previos}},
				}}
				a := ask(t, state, jev.WatchdogQuestions())
				p, ok := a.NoulOf("sem_progresso")
				if !ok {
					t.Fatal("resposta sem sem_progresso")
				}
				if got := p >= 0.5; got != f.Esperado.SemProgresso {
					t.Errorf("sem_progresso = %.2f, rotulo diz %v", p, f.Esperado.SemProgresso)
				}
			})
		}
	})
}
