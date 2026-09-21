package gate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/jev"
)

// fakeAsker devolve respostas fixas por id de pergunta.
type fakeAsker struct {
	nouls   map[string]float64
	choices map[string]string
	scores  map[string]float64
}

func (f fakeAsker) Ask(_ context.Context, _ any, qs map[string]jev.Question) (jev.Result, error) {
	raw := map[string]any{}
	for id, q := range qs {
		switch q.(type) {
		case jev.Noul:
			raw[id] = map[string]any{"type": "noul", "noul": f.nouls[id]}
		case jev.Choice:
			raw[id] = map[string]any{"type": "choice", "choice": f.choices[id], "confidence": 0.9,
				"probabilities": map[string]any{f.choices[id]: 0.9}}
		case jev.Score:
			raw[id] = map[string]any{"type": "score", "score": f.scores[id], "confidence": 0.8}
		}
	}
	b, _ := json.Marshal(raw)
	var answers jev.Answers
	if err := json.Unmarshal(b, &answers); err != nil {
		return jev.Result{}, err
	}
	return jev.Result{Answers: answers, Usage: jev.Usage{InputTokens: 100}}, nil
}

func healthyAsker() fakeAsker {
	return fakeAsker{
		nouls: map[string]float64{
			"defeito_unico": 0.95, "desenho_em_aberto": 0.05, "cruza_pacotes": 0.1,
			"toca_sensivel": 0.02, "criterio_de_pronto": 0.93,
			"aponta_arquivo_linha": 0.97, "cita_fonte_do_contrato": 0.9,
			"pede_teste_antes_da_correcao": 0.96, "comandos_copiaveis": 0.98,
			"limites_explicitos": 0.94, "pede_relatorio": 0.95,
			"tarefa_autocontida": 0.9,
		},
		choices: map[string]string{"tipo_de_tarefa": "correcao_com_teste"},
		scores:  map[string]float64{"complexidade": 1.2},
	}
}

func TestCheckApprovesHealthyTask(t *testing.T) {
	v, _, err := Check(context.Background(), healthyAsker(), "tarefa", "briefing", RepoFacts{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !v.Delegable {
		t.Fatalf("quero delegavel, Missing=%v", v.Missing)
	}
	if v.Kind != "correcao_com_teste" {
		t.Errorf("Kind = %q", v.Kind)
	}
}

func TestCheckRejectsOpenDesign(t *testing.T) {
	a := healthyAsker()
	a.nouls["desenho_em_aberto"] = 0.88

	v, _, err := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if v.Delegable {
		t.Fatal("tarefa com desenho em aberto nao e delegavel")
	}
	if !contains(v.Missing, "desenho_em_aberto") {
		t.Errorf("Missing = %v, quero incluir desenho_em_aberto", v.Missing)
	}
}

func TestCheckRejectsSensitiveTask(t *testing.T) {
	a := healthyAsker()
	a.nouls["toca_sensivel"] = 0.8

	v, _, _ := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
	if v.Delegable {
		t.Fatal("tarefa que toca credencial ou deploy nao e delegavel")
	}
}

func TestCheckNamesMissingBriefingItem(t *testing.T) {
	a := healthyAsker()
	a.nouls["pede_teste_antes_da_correcao"] = 0.2

	v, _, _ := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
	if v.Delegable {
		t.Fatal("briefing sem pedido de vermelho reprova")
	}
	if !contains(v.Missing, "pede_teste_antes_da_correcao") {
		t.Errorf("Missing = %v", v.Missing)
	}
}

// cruza_pacotes e aviso, nao reprovacao: a decisao fica com o autor.
func TestCheckWarnsButApprovesCrossPackage(t *testing.T) {
	a := healthyAsker()
	a.nouls["cruza_pacotes"] = 0.9

	v, _, _ := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
	if !v.Delegable {
		t.Fatal("cruza_pacotes nao deve reprovar")
	}
	if !contains(v.Warnings, "cruza_pacotes") {
		t.Errorf("Warnings = %v", v.Warnings)
	}
}

// A autocontencao nao e criterio de gate: ela sobe ao Verdict para a rota
// decidir se a tarefa cabe num modelo barato (spec §8).
func TestCheckSurfacesAutocontida(t *testing.T) {
	a := healthyAsker()
	a.nouls["tarefa_autocontida"] = 0.35

	v, _, err := Check(context.Background(), a, "t", "b", RepoFacts{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if v.Autocontida != 0.35 {
		t.Errorf("Autocontida = %v, quero 0.35 (o noul de tarefa_autocontida)", v.Autocontida)
	}
	if !v.Delegable {
		t.Error("autocontencao baixa nao reprova: nao e item do gate")
	}
}

// dropAsker embrulha o fake removendo ids da resposta — simula a API
// devolvendo a requisicao sem uma pergunta respondida.
type dropAsker struct {
	inner fakeAsker
	drop  map[string]bool
}

func (d dropAsker) Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error) {
	res, err := d.inner.Ask(ctx, state, qs)
	for id := range d.drop {
		delete(res.Answers, id)
	}
	return res, err
}

// Resposta que pesa na decisao nao pode faltar: ausente nao e "passou",
// e erro — aprovar em cima de resposta cortada e o que o gate evita.
func TestCheckFailsClosedOnMissingAnswer(t *testing.T) {
	for _, id := range []string{"desenho_em_aberto", "tipo_de_tarefa", "pede_relatorio", "tarefa_autocontida"} {
		a := dropAsker{inner: healthyAsker(), drop: map[string]bool{id: true}}
		v, _, err := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
		if err == nil {
			t.Fatalf("resposta ausente de %s tinha que ser erro, nao veredito %+v", id, v)
		}
		if !strings.Contains(err.Error(), id) {
			t.Errorf("o erro tinha que nomear a resposta ausente %s: %v", id, err)
		}
	}
}

// Os avisos sao opcionais de verdade: faltar cruza_pacotes ou defeito_unico
// nao reprova nem derruba o gate — a decisao segue com o autor.
func TestCheckTreatsWarningsAsOptional(t *testing.T) {
	a := dropAsker{inner: healthyAsker(), drop: map[string]bool{"cruza_pacotes": true, "defeito_unico": true}}
	v, _, err := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
	if err != nil {
		t.Fatalf("aviso ausente nao pode derrubar o gate: %v", err)
	}
	if !v.Delegable {
		t.Error("sem os avisos a tarefa saudavel continua delegavel")
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
