// Package route decide qual modelo do roster executa a tarefa. O Jev julga
// o briefing — dimensão dominante e complexidade — e o código faz a
// aritmética: a dimensão escolhe o índice de benchmark, a complexidade vira
// um percentil de corte dentro dos candidatos, e entre os que passam vence
// o menor custo por tarefa — não por token, porque verbosidade é custo
// (spec §6.2).
package route

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/roster"
)

// Dimensao é o eixo de benchmark que a tarefa mais exige. Os três valores
// existem porque são os três com coluna de benchmark — dimensão sem medida
// correspondente seria resposta bonita e inútil.
type Dimensao string

const (
	Mecanica   Dimensao = "mecanica"   // coding_index
	Raciocinio Dimensao = "raciocinio" // intelligence_index
	Agentica   Dimensao = "agentica"   // tau_bench
)

// Escolha é o modelo escolhido e o porquê, para o relatório explicar a
// rota sem refazer a conta.
type Escolha struct {
	Modelo    roster.Model
	Dimensao  Dimensao
	Percentil float64 // corte aplicado, já saturado em [0,1]
	NaoMedido bool    // sem benchmark: não passou pelo corte, entrou por custo
	Motivo    string  // vazio quando a escolha é o caso normal
}

// Asker é a parte do cliente Jev de que a rota precisa.
// *jev.Client implementa.
type Asker interface {
	Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error)
}

// Escolher aplica o corte de percentil sobre os candidatos no índice da
// dimensão e devolve o mais barato por tarefa entre os que sobram.
//
// O percentil é posição dentro do conjunto medido — os índices vivem em
// escalas diferentes e corte absoluto seria erro. Modelo sem benchmark sai
// do cálculo (senão distorce a distribuição) mas continua elegível, marcado
// NaoMedido: só vence quando nenhum medido está disponível, porque não tem
// custo por tarefa para comparar. Corte vazio nunca é erro — se ninguém
// passa, o melhor disponível assume e o Motivo registra.
func Escolher(ms []roster.Model, d Dimensao, percentil float64) (Escolha, error) {
	if len(ms) == 0 {
		return Escolha{}, fmt.Errorf("route: sem candidatos")
	}
	switch d {
	case Mecanica, Raciocinio, Agentica:
	default:
		return Escolha{}, fmt.Errorf("route: dimensão desconhecida %q", d)
	}
	percentil = min(1, max(0, percentil))

	var measured, unmeasured []roster.Model
	for _, m := range ms {
		if m.Benchmark == nil {
			unmeasured = append(unmeasured, m)
		} else {
			measured = append(measured, m)
		}
	}

	// Posição i/(n-1) na ordenação crescente: o melhor está no percentil 1,
	// o pior no 0 — o topo sempre passa qualquer corte válido.
	sort.SliceStable(measured, func(i, j int) bool {
		return indexOf(measured[i], d) < indexOf(measured[j], d)
	})
	var passing []roster.Model
	for i, m := range measured {
		pos := 1.0
		if len(measured) > 1 {
			pos = float64(i) / float64(len(measured)-1)
		}
		if pos >= percentil {
			passing = append(passing, m)
		}
	}

	if len(passing) > 0 {
		best := passing[0]
		for _, m := range passing[1:] {
			if m.Benchmark.CustoPorTarefaUSD < best.Benchmark.CustoPorTarefaUSD {
				best = m
			}
		}
		return Escolha{Modelo: best, Dimensao: d, Percentil: percentil}, nil
	}

	// Ninguém passou o corte: com o corte saturado em [0,1] isso só acontece
	// quando não há medidos — o topo da ordenação tem posição 1 e passa
	// sempre. Ou seja, todos os candidatos são sem benchmark: sem índice
	// não há distribuição nem corte, e entra o mais barato por MTok entre
	// os declarados, marcado para o relatório.
	best := unmeasured[0]
	for _, m := range unmeasured[1:] {
		if costMTok(m) < costMTok(best) {
			best = m
		}
	}
	return Escolha{Modelo: best, Dimensao: d, Percentil: percentil, NaoMedido: true,
		Motivo: fmt.Sprintf("%s não tem benchmark no índice da dimensão %s; "+
			"escolhido por viabilidade e menor custo declarado por MTok", best.ID, d)}, nil
}

// Classificar pergunta ao Jev a dimensão dominante e a complexidade do
// briefing, numa requisição só, e devolve a dimensão e o percentil de
// corte pronto para Escolher.
// Classificacao e o que a rota apura sobre a tarefa, em uma requisição só.
// Dimensao e Percentil decidem QUAL modelo; Volume decide QUANTO ele pode
// gastar chegando lá. São eixos distintos de propósito.
type Classificacao struct {
	Dimensao  Dimensao
	Percentil float64
	Volume    float64
}

func Classificar(ctx context.Context, a Asker, briefing string) (Classificacao, jev.Usage, error) {
	// complexidade existe desde o v1; dimensao_dominante chega com as
	// perguntas novas de rota — sem ela no conjunto não há o que perguntar.
	qd, okD := jev.RouteQuestions()["dimensao_dominante"]
	qc, okC := jev.RouteQuestions()["complexidade"]
	qv, okV := jev.RouteQuestions()["volume"]
	if !okD || !okC || !okV {
		return Classificacao{}, jev.Usage{}, fmt.Errorf("route: perguntas de rota ausentes em jev.RouteQuestions "+
			"(dimensao_dominante=%v, complexidade=%v, volume=%v)", okD, okC, okV)
	}
	sc, ok := qc.(jev.Score)
	if !ok || len(sc.Criteria) < 2 {
		return Classificacao{}, jev.Usage{}, fmt.Errorf("route: complexidade não é um Score de níveis")
	}

	state := map[string]any{"tarefa": map[string]any{"texto": briefing}}
	res, err := a.Ask(ctx, state, map[string]jev.Question{
		"dimensao_dominante": qd,
		"complexidade":       qc,
		"volume":             qv,
	})
	if err != nil {
		return Classificacao{}, jev.Usage{}, fmt.Errorf("route: %w", err)
	}

	ch, ok := res.Answers.ChoiceOf("dimensao_dominante")
	if !ok {
		return Classificacao{}, res.Usage, fmt.Errorf("route: resposta sem dimensao_dominante")
	}
	d := Dimensao(ch.Choice)
	switch d {
	case Mecanica, Raciocinio, Agentica:
	default:
		return Classificacao{}, res.Usage, fmt.Errorf("route: dimensão desconhecida %q", ch.Choice)
	}

	sa, ok := res.Answers.ScoreOf("complexidade")
	if !ok {
		return Classificacao{}, res.Usage, fmt.Errorf("route: resposta sem complexidade")
	}
	// O Score é a posição esperada entre os níveis, de 0 a n-1 — dividir
	// pelo último nível o normaliza para o corte em [0,1].
	percentil := min(1, max(0, sa.Score/float64(len(sc.Criteria)-1)))

	// Volume ausente não invalida a rota: cai no nível 1, que é a âncora do
	// orçamento base. Perder o ajuste de tamanho é pior que parar o trabalho.
	volume := 1.0
	if sv, ok := res.Answers.ScoreOf("volume"); ok {
		volume = sv.Score
	}
	return Classificacao{Dimensao: d, Percentil: percentil, Volume: volume}, res.Usage, nil
}

// indexOf devolve a coluna de benchmark da dimensão. Chamado só com
// Benchmark não nulo e dimensão válida.
func indexOf(m roster.Model, d Dimensao) float64 {
	switch d {
	case Mecanica:
		return m.Benchmark.CodingIndex
	case Raciocinio:
		return m.Benchmark.IntelligenceIndex
	case Agentica:
		return m.Benchmark.TauBench
	}
	return 0
}

// costMTok devolve o custo declarado por token; não declarado é o mais
// caro possível — null não é zero.
func costMTok(m roster.Model) float64 {
	if m.CustoUSDPorMTok == nil {
		return math.Inf(1)
	}
	return *m.CustoUSDPorMTok
}
