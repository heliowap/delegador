package gate

import (
	"context"
	"fmt"
	"sort"

	"github.com/heliowap/delegador/internal/jev"
)

// Asker e o que os gates precisam de um cliente Jev. Interface estreita para
// que os testes nao precisem de HTTP.
type Asker interface {
	Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error)
}

// Thresholds sao os limiares de decisao. Ficam em configuracao, nunca
// espalhados como constante magica.
type Thresholds struct {
	DesignOpen   float64 // acima disto, reprova
	Sensitive    float64 // acima disto, reprova
	DoneCriteria float64 // abaixo disto, reprova
	SingleDefect float64 // abaixo disto, avisa
	CrossPackage float64 // acima disto, avisa
	BriefingItem float64 // abaixo disto, reprova o item
}

// DefaultThresholds traz os valores de partida, a recalibrar com evals/.
func DefaultThresholds() Thresholds {
	return Thresholds{
		DesignOpen: 0.60, Sensitive: 0.50, DoneCriteria: 0.60,
		SingleDefect: 0.50, CrossPackage: 0.70, BriefingItem: 0.60,
	}
}

// RepoFacts sao os fatos que o delegador levanta sozinho, sem modelo.
type RepoFacts struct {
	BranchBase string   `json:"branch_base"`
	CitedFiles []string `json:"arquivos_citados"`
	Packages   []string `json:"pacotes_atingidos"`
}

// Verdict e a decisao do gate. Autocontida e a probabilidade de o briefing
// determinar a execucao a ponto de ela ser transcricao — sai da pergunta
// tarefa_autocontida e informa a rota, sem reprovar nem aprovar sozinha.
type Verdict struct {
	Delegable   bool     `json:"delegavel"`
	Kind        string   `json:"tipo"`
	Missing     []string `json:"faltando"`
	Warnings    []string `json:"avisos"`
	Autocontida float64  `json:"autocontida"`
}

// Check roda delegabilidade, gate de briefing e autocontencao numa unica
// requisicao. Perguntas independentes sobre o mesmo state vao juntas: elas
// sao avaliadas em paralelo e nao veem as respostas umas das outras. A rota
// nao esta aqui: dimensao e complexidade sao perguntas do route.Classificar,
// que o plan chama a parte sobre o briefing aprovado.
func Check(ctx context.Context, a Asker, task, briefing string, facts RepoFacts) (Verdict, jev.Usage, error) {
	th := DefaultThresholds()

	state := map[string]any{
		"tarefa":   map[string]any{"texto": task},
		"briefing": map[string]any{"texto": briefing},
		"repo":     facts,
	}

	qs := map[string]jev.Question{}
	for id, q := range jev.DelegabilityQuestions() {
		qs[id] = q
	}
	for id, q := range jev.BriefingQuestions() {
		qs[id] = q
	}
	for id, q := range jev.AutonomyQuestion() {
		qs[id] = q
	}

	res, err := a.Ask(ctx, state, qs)
	if err != nil {
		return Verdict{}, jev.Usage{}, fmt.Errorf("gate: %w", err)
	}

	// Fecha a porta: toda resposta que pesa na decisao tem que estar
	// presente e no tipo certo. Ausente ou malformada nao e "passou" — e
	// resposta cortada, bug ou API manca, e aprovar em cima dela e o que o
	// gate existe para impedir. So os avisos (cruza_pacotes, defeito_unico)
	// ficam opcionais.
	if _, ok := res.Answers.ChoiceOf("tipo_de_tarefa"); !ok {
		return Verdict{}, res.Usage, fmt.Errorf("gate: resposta ausente: tipo_de_tarefa")
	}
	requiredNouls := []string{"desenho_em_aberto", "toca_sensivel",
		"criterio_de_pronto", "tarefa_autocontida"}
	for id := range jev.BriefingQuestions() {
		requiredNouls = append(requiredNouls, id)
	}
	sort.Strings(requiredNouls)
	for _, id := range requiredNouls {
		if _, ok := res.Answers.NoulOf(id); !ok {
			return Verdict{}, res.Usage, fmt.Errorf("gate: resposta ausente: %s", id)
		}
	}

	v := Verdict{Delegable: true}

	if ch, ok := res.Answers.ChoiceOf("tipo_de_tarefa"); ok {
		v.Kind = ch.Choice
		if ch.Choice == "nao_delegavel" {
			v.Delegable = false
			v.Missing = append(v.Missing, "tipo_de_tarefa=nao_delegavel")
		}
	}

	reject := func(id string, p float64, above bool, limit float64) {
		if (above && p > limit) || (!above && p < limit) {
			v.Delegable = false
			v.Missing = append(v.Missing, id)
		}
	}
	if p, ok := res.Answers.NoulOf("desenho_em_aberto"); ok {
		reject("desenho_em_aberto", p, true, th.DesignOpen)
	}
	if p, ok := res.Answers.NoulOf("toca_sensivel"); ok {
		reject("toca_sensivel", p, true, th.Sensitive)
	}
	if p, ok := res.Answers.NoulOf("criterio_de_pronto"); ok {
		reject("criterio_de_pronto", p, false, th.DoneCriteria)
	}

	// Avisos: sinalizam, mas a decisao fica com o autor.
	if p, ok := res.Answers.NoulOf("cruza_pacotes"); ok && p > th.CrossPackage {
		v.Warnings = append(v.Warnings, "cruza_pacotes")
	}
	if p, ok := res.Answers.NoulOf("defeito_unico"); ok && p < th.SingleDefect {
		v.Warnings = append(v.Warnings, "defeito_unico")
	}

	for id := range jev.BriefingQuestions() {
		if p, ok := res.Answers.NoulOf(id); ok && p < th.BriefingItem {
			v.Delegable = false
			v.Missing = append(v.Missing, id)
		}
	}

	if p, ok := res.Answers.NoulOf("tarefa_autocontida"); ok {
		v.Autocontida = p
	}

	sort.Strings(v.Missing)
	sort.Strings(v.Warnings)
	return v, res.Usage, nil
}

// SelectEvidence mantem apenas a evidencia necessaria a esta tarefa. Nunca
// reescreve: cada item entra inteiro ou nao entra.
func SelectEvidence(ctx context.Context, a Asker, task string, items []Evidence) ([]Evidence, jev.Usage, error) {
	var total jev.Usage
	var kept []Evidence

	for i := range items {
		state := map[string]any{
			"tarefa": map[string]any{"texto": task},
			"item":   map[string]any{"tipo": items[i].Kind, "ref": items[i].Ref, "texto": items[i].Text},
		}
		res, err := a.Ask(ctx, state, jev.EvidenceQuestion())
		if err != nil {
			return nil, total, fmt.Errorf("selecao de evidencia: %w", err)
		}
		total.InputTokens += res.Usage.InputTokens

		p, ok := res.Answers.NoulOf("evidencia_necessaria")
		if ok && p >= 0.5 {
			items[i].Kept = true
			kept = append(kept, items[i])
		}
	}
	return kept, total, nil
}
