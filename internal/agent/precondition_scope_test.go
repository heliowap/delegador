package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

// O sinal fora_do_escopo do spec §6.3 ficou deferido durante a execucao do
// plano, por um defeito da minha interface: a pre-condicao nao recebia a
// politica. tools.Allow ja NEGA a escrita; este sinal detecta o modelo
// INSISTINDO, que e sintoma de briefing errado ou modelo perdido — e sem ele
// cada tentativa vira recusa silenciosa ate o teto de turnos.

func escopo(t *testing.T) tools.Policy {
	t.Helper()
	wt := t.TempDir()
	for _, d := range []string{"pkg/svc", "infra"} {
		if err := os.MkdirAll(filepath.Join(wt, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return tools.Policy{Worktree: wt, WritePrefixes: []string{"pkg/svc/"}}
}

func turnoEscrevendo(i int, path string) Turn {
	return Turn{Index: i,
		Message: llm.Message{ToolCalls: []tools.Call{{Name: "write_file",
			Args: map[string]string{"path": path, "content": "x"}}}},
		Results: []tools.Result{{Output: "negado", IsError: true}}}
}

func preComEscopo(t *testing.T, p tools.Policy) Precondition {
	t.Helper()
	cfg := DefaultPreConfig()
	cfg.RepeatThreshold = 99 // isola o sinal de escopo do de repeticao
	cfg.IdleTurns = 99
	cfg.Policy = p
	return NewPrecondition(cfg, nil, func() float64 { return 0 })
}

// Errar uma vez e normal: o modelo nao conhece o escopo ate tentar.
func TestUmaTentativaForaDoEscopoNaoVeta(t *testing.T) {
	pre := preComEscopo(t, escopo(t))
	if v := pre([]Turn{turnoEscrevendo(0, "infra/deploy.yaml")}); v != nil {
		t.Errorf("uma tentativa nao deveria vetar: %+v", v)
	}
}

// Insistir e o sinal.
func TestInsistirForaDoEscopoVeta(t *testing.T) {
	pre := preComEscopo(t, escopo(t))
	pre([]Turn{turnoEscrevendo(0, "infra/deploy.yaml")})
	v := pre([]Turn{turnoEscrevendo(0, "infra/deploy.yaml"), turnoEscrevendo(1, "infra/secrets.tf")})
	if v == nil {
		t.Fatal("duas tentativas fora do escopo deveriam vetar")
	}
	if v.Signal != "fora_do_escopo" {
		t.Errorf("Signal = %q", v.Signal)
	}
	if v.Excerpt == "" {
		t.Error("o veto precisa nomear o caminho que o causou")
	}
	if v.Probability != 1 {
		t.Errorf("Probability = %v; sinal deterministico vale 1", v.Probability)
	}
}

func TestEscritaDentroDoEscopoNuncaVeta(t *testing.T) {
	pre := preComEscopo(t, escopo(t))
	turns := []Turn{}
	for i := 0; i < 6; i++ {
		turns = append(turns, Turn{Index: i,
			Message: llm.Message{ToolCalls: []tools.Call{{Name: "write_file",
				Args: map[string]string{"path": "pkg/svc/rota.go", "content": "x"}}}},
			Results: []tools.Result{{Output: "ok"}}})
		if v := pre(turns); v != nil {
			t.Fatalf("escrita no escopo vetou no turno %d: %+v", i, v)
		}
	}
}

// Sem escopo declarado nao ha escopo a violar — o sinal fica desligado, e nao
// ligado para tudo. Ausencia de politica nao inventa politica.
func TestSemEscopoDeclaradoOSinalFicaDesligado(t *testing.T) {
	p := escopo(t)
	p.WritePrefixes = nil
	pre := preComEscopo(t, p)
	for i := 0; i < 5; i++ {
		if v := pre([]Turn{turnoEscrevendo(0, "qualquer.go"), turnoEscrevendo(1, "outro.go")}); v != nil {
			t.Fatalf("sem prefixo declarado nao deveria vetar: %+v", v)
		}
	}
}

// Leitura fora do escopo e legitima: so escrita conta.
func TestLeituraForaDoEscopoNaoVeta(t *testing.T) {
	pre := preComEscopo(t, escopo(t))
	ler := func(i int) Turn {
		return Turn{Index: i,
			Message: llm.Message{ToolCalls: []tools.Call{{Name: "read_file",
				Args: map[string]string{"path": "infra/deploy.yaml"}}}},
			Results: []tools.Result{{Output: "conteudo"}}}
	}
	pre([]Turn{ler(0)})
	if v := pre([]Turn{ler(0), ler(1), ler(2)}); v != nil {
		t.Errorf("leitura nao deveria disparar o sinal de escopo: %+v", v)
	}
}
