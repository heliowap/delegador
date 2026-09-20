package jev

import (
	"encoding/json"
	"strings"
	"testing"
)

// Toda pergunta precisa carregar significado completa: o id nao e enviado ao
// modelo, entao "defeito_unico" sozinho nao diz nada a ele.
func TestAllQuestionsHaveSubstantiveInstructions(t *testing.T) {
	sets := map[string]map[string]Question{
		"delegability": DelegabilityQuestions(),
		"briefing":     BriefingQuestions(),
		"route":        RouteQuestions(),
		"watchdog":     WatchdogQuestions(),
		"evidence":     EvidenceQuestion(),
		"compaction":   CompactionQuestions(),
		"report":       ReportQuestion(),
		"finding":      FindingQuestions(),
	}
	for set, qs := range sets {
		if len(qs) == 0 {
			t.Errorf("conjunto %s esta vazio", set)
		}
		for id, q := range qs {
			raw, err := json.Marshal(q)
			if err != nil {
				t.Fatalf("%s/%s: %v", set, id, err)
			}
			var probe struct {
				Instructions string `json:"instructions"`
			}
			if err := json.Unmarshal(raw, &probe); err != nil {
				t.Fatalf("%s/%s: %v", set, id, err)
			}
			if len(probe.Instructions) < 40 {
				t.Errorf("%s/%s: instructions curtas demais (%d chars): %q",
					set, id, len(probe.Instructions), probe.Instructions)
			}
			if strings.Contains(probe.Instructions, "_") && !strings.Contains(probe.Instructions, " ") {
				t.Errorf("%s/%s: instructions parecem um id, nao uma pergunta", set, id)
			}
		}
	}
}

func TestDelegabilityHasExpectedIDs(t *testing.T) {
	qs := DelegabilityQuestions()
	for _, id := range []string{
		"tipo_de_tarefa", "defeito_unico", "desenho_em_aberto",
		"cruza_pacotes", "toca_sensivel", "criterio_de_pronto",
	} {
		if _, ok := qs[id]; !ok {
			t.Errorf("pergunta %q ausente", id)
		}
	}
	ch, ok := qs["tipo_de_tarefa"].(Choice)
	if !ok {
		t.Fatal("tipo_de_tarefa deveria ser Choice")
	}
	for _, opt := range []string{"correcao_com_teste", "revisao_somente_leitura", "investigacao", "nao_delegavel"} {
		if _, ok := ch.Criteria[opt]; !ok {
			t.Errorf("opcao %q ausente em tipo_de_tarefa", opt)
		}
	}
}

// dangerous nunca pode ser escolhido pela rota: e regra global do projeto.
func TestRouteNeverOffersDangerous(t *testing.T) {
	ch, ok := RouteQuestions()["permissao"].(Choice)
	if !ok {
		t.Fatal("permissao deveria ser Choice")
	}
	if _, found := ch.Criteria["dangerous"]; found {
		t.Error("dangerous nao pode ser opcao da rota")
	}
	for _, want := range []string{"auto", "accept-edits", "smart"} {
		if _, ok := ch.Criteria[want]; !ok {
			t.Errorf("modo %q ausente", want)
		}
	}
}

// Score precisa de niveis descritos por situacao concreta, do menor ao maior.
func TestComplexityScoreHasConcreteLevels(t *testing.T) {
	sc, ok := RouteQuestions()["complexidade"].(Score)
	if !ok {
		t.Fatal("complexidade deveria ser Score")
	}
	if len(sc.Criteria) < 2 || len(sc.Criteria) > 10 {
		t.Fatalf("len(Criteria) = %d, o Jev aceita de 2 a 10", len(sc.Criteria))
	}
	for i, level := range sc.Criteria {
		if len(level) < 30 {
			t.Errorf("nivel %d descrito de forma vaga: %q", i, level)
		}
	}
}

// Todo conjunto precisa caber no orcamento junto com um state util.
func TestEverySetLeavesRoomForState(t *testing.T) {
	sets := map[string]map[string]Question{
		"delegability": DelegabilityQuestions(),
		"briefing":     BriefingQuestions(),
		"route":        RouteQuestions(),
		"watchdog":     WatchdogQuestions(),
		"compaction":   CompactionQuestions(),
		"finding":      FindingQuestions(),
	}
	for name, qs := range sets {
		budget, err := StateBudget(qs, DefaultLimits())
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if budget < 20_000 {
			t.Errorf("%s: sobram so %d tokens para o state", name, budget)
		}
	}
}

func TestDimensaoDominanteTemAsTresComBenchmark(t *testing.T) {
	ch, ok := RouteQuestions()["dimensao_dominante"].(Choice)
	if !ok {
		t.Fatal("dimensao_dominante deveria ser Choice")
	}
	// As opcoes existem porque as tres tem coluna de benchmark. Dimensao sem
	// medida correspondente seria resposta bonita que o codigo nao usa.
	for _, o := range []string{"mecanica", "raciocinio", "agentica"} {
		if _, ok := ch.Criteria[o]; !ok {
			t.Errorf("opcao %q ausente", o)
		}
	}
	if len(ch.Criteria) != 3 {
		t.Errorf("quero exatamente 3 opcoes, tenho %d", len(ch.Criteria))
	}
}

func TestTarefaAutocontidaExiste(t *testing.T) {
	n, ok := AutonomyQuestion()["tarefa_autocontida"].(Noul)
	if !ok {
		t.Fatal("tarefa_autocontida deveria ser Noul")
	}
	if n.Criteria == nil || n.Criteria.True == "" || n.Criteria.False == "" {
		t.Error("a fronteira entre transcrever e decidir precisa estar descrita")
	}
}

// Saiu do Jev quando a permissao virou allowlist em codigo: o estado que ela
// detectava nao existe mais.
func TestBloqueioDePermissaoNaoEhMaisPerguntaJev(t *testing.T) {
	if _, existe := WatchdogQuestions()["bloqueio_de_permissao"]; existe {
		t.Error("bloqueio_de_permissao deveria ter saido do conjunto")
	}
	if len(WatchdogQuestions()) != 1 {
		t.Errorf("o watchdog do v2 tem uma pergunta so, tem %d", len(WatchdogQuestions()))
	}
}
