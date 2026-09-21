package gate

import (
	"context"
	"fmt"
	"regexp"
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

	// SelfContainedFloor e o piso de tarefa_autocontida abaixo do qual a
	// tarefa deixa de ser delegavel — nao "delegavel com modelo melhor",
	// nao delegavel.
	//
	// A rota ja tratava ambiguidade elevando o piso de tau_bench, o que
	// supoe que existe algum modelo capaz de sustentar o enquadramento
	// sozinho. Ha um ponto em que essa suposicao deixa de valer: quando a
	// decisao E a entrega, delegar nao produz uma resposta pior, produz a
	// resposta de outra pergunta. A Cognition mediu esse caso num Fusion
	// com julgamento delegado — custo caiu 28%, score caiu de 54 para 27 —
	// e o remedio deles e o mesmo daqui: nao delegar.
	//
	// NAO CALIBRADO, e a distincao importa. As sete fixtures de
	// autocontencao rotulam "fechada" contra "aberta", nao "delegavel"
	// contra "indelegavel": 0.25 fica abaixo do grupo rotulado aberto
	// (0.050, 0.050, 0.080, 0.350) de forma a separar os tres mais
	// extremos, mas nenhum rotulo diz que ESSES tres nao deviam ter sido
	// delegados. Calibrar de verdade exige rodar tarefas ambiguas e medir
	// onde a entrega deixa de responder a pergunta feita.
	SelfContainedFloor float64
}

// DefaultThresholds traz os valores de partida, a recalibrar com evals/.
func DefaultThresholds() Thresholds {
	return Thresholds{
		DesignOpen: 0.60, Sensitive: 0.50, DoneCriteria: 0.60,
		SingleDefect: 0.50, CrossPackage: 0.70, BriefingItem: 0.60,
		SelfContainedFloor: 0.25,
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
	return CheckCom(ctx, a, task, briefing, facts, DefaultThresholds())
}

// CheckCom e o Check com os limiares explicitos. Existe para que uma
// fixture possa exercer um limiar sem depender do valor de fabrica — mesma
// convencao de route.EscolherCom.
func CheckCom(ctx context.Context, a Asker, task, briefing string, facts RepoFacts,
	th Thresholds) (Verdict, jev.Usage, error) {
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
		// Abaixo do piso a tarefa nao e delegavel a modelo nenhum: o que
		// falta nao e capacidade, e a decisao que ninguem tomou. O remedio
		// e fechar o briefing, e por isso a reprova vem com o proprio id
		// na lista — e o mesmo texto que precisa mudar.
		if th.SelfContainedFloor > 0 && p < th.SelfContainedFloor {
			v.Delegable = false
			v.Missing = append(v.Missing, "tarefa_autocontida")
		}
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
	pontos := make([]float64, len(items))

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
		pontos[i] = p
	}

	// Rede de seguranca: o gate de briefing REPROVA quando falta a fonte do
	// contrato ou a localizacao do defeito, e a selecao acabou de poder jogar
	// fora o unico item que as supre. Duas etapas minhas discordando garante
	// reprovacao, e isso nao e julgamento — e acoplamento, e se resolve em
	// codigo. Quando nenhum item de um tipo exigido sobreviveu, o melhor
	// pontuado daquele tipo volta.
	for _, tipo := range tiposExigidos {
		if temTipo(kept, tipo) {
			continue
		}
		melhor, achou := -1, false
		for i := range items {
			if items[i].Kind != tipo {
				continue
			}
			if !achou || pontos[i] > pontos[melhor] {
				melhor, achou = i, true
			}
		}
		if achou {
			items[melhor].Kept = true
			kept = append(kept, items[melhor])
		}
	}

	// Segunda rede, sobre PROPRIEDADE e nao sobre tipo: aponta_arquivo_linha
	// exige um caminho:linha apontando o defeito, e a secao que o alimenta e
	// montada so dos trechos. Medido em 2026-09-21 na issue expr-lang/expr#950,
	// onde havia dois trechos — o diff do teste, sem linha, e a triagem, com
	// ela. A selecao descartou os dois, a rede de tipo resgatou o primeiro, e
	// o gate reprovou. Ser `trecho` nao implica localizar.
	if !temLocalizacao(kept) {
		melhor, achou := -1, false
		for i := range items {
			if items[i].Kind != "trecho" || !localiza(items[i]) {
				continue
			}
			if !achou || pontos[i] > pontos[melhor] {
				melhor, achou = i, true
			}
		}
		if achou {
			if !items[melhor].Kept {
				items[melhor].Kept = true
				kept = append(kept, items[melhor])
			}
		}
	}
	return kept, total, nil
}

// refLocalizacao casa uma referencia caminho/arquivo.ext:linha. Conta so o
// que tem extensao e numero: `pkg/svc` e `Fetch()` sao nome solto, que o
// criterio do gate rejeita explicitamente.
var refLocalizacao = regexp.MustCompile(`[\w./\\-]+\.[A-Za-z0-9]+:\d+`)

// localiza diz se este item carrega um caminho:linha, na ref ou no texto.
func localiza(e Evidence) bool {
	return refLocalizacao.MatchString(e.Ref) || refLocalizacao.MatchString(e.Text)
}

// temLocalizacao diz se algum TRECHO mantido carrega caminho:linha. Escopo
// no trecho de proposito: um `erro` costuma citar a linha do teste que
// falhou, que e o sintoma, nao o lugar do defeito.
func temLocalizacao(items []Evidence) bool {
	for _, e := range items {
		if e.Kind == "trecho" && localiza(e) {
			return true
		}
	}
	return false
}

// tiposExigidos sao os tipos de evidencia que alimentam secoes cujo gate
// bloqueia: `fonte` supre cita_fonte_do_contrato, `trecho` supre
// aponta_arquivo_linha.
var tiposExigidos = []string{"fonte", "trecho"}

func temTipo(items []Evidence, tipo string) bool {
	for _, e := range items {
		if e.Kind == tipo {
			return true
		}
	}
	return false
}
