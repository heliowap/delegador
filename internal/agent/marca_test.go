package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

// askerEspiao registra as perguntas de cada request e responde a todas.
type askerEspiao struct {
	requests  [][]string
	respostas map[string]float64
}

func (e *askerEspiao) Ask(_ context.Context, _ any, qs map[string]jev.Question) (jev.Result, error) {
	var ids []string
	corpo := map[string]any{}
	for id := range qs {
		ids = append(ids, id)
		p, ok := e.respostas[id]
		if !ok {
			p = 0.9
		}
		corpo[id] = map[string]any{"type": "noul", "noul": p}
	}
	e.requests = append(e.requests, ids)
	raw, _ := json.Marshal(corpo)
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 100}}, nil
}

func contem(ids []string, alvo string) bool {
	for _, id := range ids {
		if id == alvo {
			return true
		}
	}
	return false
}

func turnoCom(idx int, chamadas ...string) Turn {
	t := Turn{Index: idx}
	for _, c := range chamadas {
		t.Message.ToolCalls = append(t.Message.ToolCalls,
			tools.Call{Name: "exec", Args: map[string]string{"command": c}})
		t.Results = append(t.Results, tools.Result{Output: "saida de " + c})
	}
	return t
}

// Medido em 2026-09-21 num job de 22 turnos: 21 requests de watchdog mais 29
// de compactacao, todos sobre o mesmo material, todos seriais. As duas
// decisoes olham o mesmo turno: vao no mesmo request.
func TestWatchdogECompactacaoVaoNoMesmoRequest(t *testing.T) {
	esp := &askerEspiao{respostas: map[string]float64{"sem_progresso": 0.1}}
	var marcas []Marca
	cfg := DefaultPreConfig()
	cfg.Marcar = func(m Marca) { marcas = append(marcas, m) }

	pre := NewPrecondition(cfg, esp, func() float64 { return 0 })
	turns := []Turn{turnoCom(0, "go build ./..."), turnoCom(1, "go test ./x/", "go vet ./...")}
	if v := pre(turns); v != nil {
		t.Fatalf("veto inesperado: %+v", v)
	}

	if n := len(esp.requests); n != 1 {
		t.Fatalf("quero UM request por turno, tenho %d", n)
	}
	ids := esp.requests[0]
	for _, alvo := range []string{"sem_progresso", "chamada_0_necessaria", "resultado_0_verbatim",
		"chamada_1_necessaria", "resultado_1_verbatim"} {
		if !contem(ids, alvo) {
			t.Errorf("pergunta %q ficou de fora do request: %v", alvo, ids)
		}
	}
	if len(marcas) != 2 {
		t.Fatalf("quero uma marca por chamada do turno atual, tenho %d: %+v", len(marcas), marcas)
	}
	if marcas[0].Turno != 1 || marcas[0].Chamada != 0 || marcas[1].Chamada != 1 {
		t.Errorf("marcas nao casam com a posicao das chamadas: %+v", marcas)
	}
	if marcas[0].Necessaria != 0.9 {
		t.Errorf("o agent devolve a probabilidade crua, sem aplicar corte: %+v", marcas[0])
	}
}

// Sem Marcar, o request continua sendo so o do watchdog: quem nao coleta
// marcas nao paga perguntas de compactacao.
func TestSemMarcarNaoPerguntaCompactacao(t *testing.T) {
	esp := &askerEspiao{respostas: map[string]float64{"sem_progresso": 0.1}}
	pre := NewPrecondition(DefaultPreConfig(), esp, func() float64 { return 0 })
	pre([]Turn{turnoCom(0, "ls"), turnoCom(1, "go test ./...")})
	if n := len(esp.requests); n != 1 {
		t.Fatalf("quero um request, tenho %d", n)
	}
	if len(esp.requests[0]) != 1 || esp.requests[0][0] != "sem_progresso" {
		t.Errorf("quero so o watchdog, tenho %v", esp.requests[0])
	}
}

// Turno vetado pelo watchdog ainda entrega as marcas: o trace do veto e
// justamente o que o operador vai ler para julgar se o veto foi justo.
func TestTurnoVetadoAindaEntregaMarcas(t *testing.T) {
	esp := &askerEspiao{respostas: map[string]float64{"sem_progresso": 0.95}}
	var marcas []Marca
	cfg := DefaultPreConfig()
	cfg.ConsecutiveWindows = 1
	cfg.Marcar = func(m Marca) { marcas = append(marcas, m) }
	pre := NewPrecondition(cfg, esp, func() float64 { return 0 })

	v := pre([]Turn{turnoCom(0, "ls"), turnoCom(1, "ls")})
	if v == nil || v.Signal != "sem_progresso" {
		t.Fatalf("quero veto de sem_progresso, tenho %+v", v)
	}
	if len(marcas) != 1 {
		t.Errorf("as marcas do turno vetado se perderam: %+v", marcas)
	}
}

var _ = llm.Message{}
