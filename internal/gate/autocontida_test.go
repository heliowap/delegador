package gate

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/heliowap/delegador/internal/jev"
)

// askerCom responde todos os nouls com o valor saudavel, menos os
// sobrescritos, e devolve a choice de tipo pedida.
type askerCom struct {
	saudavel float64
	tipo     string
	excecoes map[string]float64
}

func (a askerCom) Ask(_ context.Context, _ any, qs map[string]jev.Question) (jev.Result, error) {
	corpo := map[string]any{}
	for id, q := range qs {
		if _, ok := q.(jev.Choice); ok {
			corpo[id] = map[string]any{"type": "choice", "choice": a.tipo, "confidence": 0.9}
			continue
		}
		p := a.saudavel
		if v, ok := a.excecoes[id]; ok {
			p = v
		}
		corpo[id] = map[string]any{"type": "noul", "noul": p}
	}
	raw, _ := json.Marshal(corpo)
	var ans jev.Answers
	_ = json.Unmarshal(raw, &ans)
	return jev.Result{Answers: ans, Usage: jev.Usage{InputTokens: 10}}, nil
}

func julga(t *testing.T, autocontida float64) Verdict {
	t.Helper()
	a := askerCom{saudavel: 0.95, tipo: "correcao_com_teste",
		excecoes: map[string]float64{
			"tarefa_autocontida": autocontida,
			"desenho_em_aberto":  0.05,
			"toca_sensivel":      0.05,
		}}
	v, _, err := CheckCom(context.Background(), a, "tarefa", "briefing", RepoFacts{}, DefaultThresholds())
	if err != nil {
		t.Fatalf("CheckCom: %v", err)
	}
	return v
}

func temFaltando(v Verdict, id string) bool {
	for _, m := range v.Missing {
		if m == id {
			return true
		}
	}
	return false
}

// Quando a decisao E a entrega, delegar nao produz uma resposta pior:
// produz a resposta de outra pergunta. Abaixo do piso o veredito e nao
// delegavel, e nao "delegavel com modelo melhor".
func TestAbaixoDoPisoNaoEDelegavel(t *testing.T) {
	v := julga(t, 0.10)
	if v.Delegable {
		t.Error("tarefa com as decisoes em aberto nao e delegavel a modelo nenhum")
	}
	if !temFaltando(v, "tarefa_autocontida") {
		t.Errorf("a reprova precisa nomear o que corrigir: %v", v.Missing)
	}
	if v.Autocontida != 0.10 {
		t.Errorf("o numero continua indo para o relatorio: %v", v.Autocontida)
	}
}

// Entre o piso e o limiar da rota, delega — e a rota eleva o piso de tau.
// Este e o caso que ja existia e nao pode ter mudado.
func TestEntreOPisoEOLimiarContinuaDelegavel(t *testing.T) {
	v := julga(t, 0.40)
	if !v.Delegable {
		t.Errorf("ambiguidade moderada e trabalho da rota, nao do gate: %v", v.Missing)
	}
}

// Tarefa fechada segue delegavel.
func TestTarefaFechadaContinuaDelegavel(t *testing.T) {
	if v := julga(t, 0.92); !v.Delegable {
		t.Errorf("tarefa fechada reprovada: %v", v.Missing)
	}
}

// Piso desligado preserva o comportamento antigo.
func TestPisoDesligadoNaoReprova(t *testing.T) {
	a := askerCom{saudavel: 0.95, tipo: "correcao_com_teste",
		excecoes: map[string]float64{
			"tarefa_autocontida": 0.01,
			"desenho_em_aberto":  0.05,
			"toca_sensivel":      0.05,
		}}
	th := DefaultThresholds()
	th.SelfContainedFloor = 0
	v, _, err := CheckCom(context.Background(), a, "tarefa", "briefing", RepoFacts{}, th)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Delegable {
		t.Errorf("com o piso desligado nada muda: %v", v.Missing)
	}
}
