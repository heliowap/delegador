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
// Opcoes sao os ajustes de rota que nao vem da dimensao nem do percentil.
type Opcoes struct {
	// Autocontida e o noul tarefa_autocontida do gate: alto quando o briefing
	// fecha as decisoes, baixo quando sobra escolha para quem executa.
	//
	// Abaixo de LimiarAutocontida, preco deixa de ser o unico criterio entre
	// os baratos. A tarefa vai exigir sustentar o enquadramento sozinho, e
	// tau_bench — uso de ferramenta em ambiente multi-turno — e o proxy mais
	// proximo disso que o benchmark oferece. Vira piso adicional, e modelo
	// sem nota de terceiro e preterido: em tarefa ambigua, ausencia de
	// medicao nao vira aposta.
	//
	// Zero significa "nao informado" e preserva o comportamento antigo.
	Autocontida float64

	// ConfiancaDimensao e a confianca da Choice dimensao_dominante. Abaixo
	// de LimiarConfiancaDimensao o corte passa a exigir os tres indices.
	// Zero significa "nao informado" e preserva o comportamento antigo.
	ConfiancaDimensao float64
}

// LimiarAutocontida e o corte abaixo do qual a rota deixa de decidir so por
// preco.
//
// CALIBRADO em 2026-09-21 contra as 7 fixtures rotuladas de evals/, mediana
// de 3 execucoes cada:
//
//	rotulo true : 0.900  0.920  0.940
//	rotulo false: 0.050  0.050  0.080  0.350
//
// Vao de 0.550 entre as classes; o ponto medio e 0.625. O valor anterior,
// 0.50, tambem separava, mas ficava descentrado — margem de 0.15 de um lado
// e 0.40 do outro.
//
// O centro e escolha deliberada, e se houvesse que descentrar seria para
// CIMA: os dois erros custam coisas diferentes. Tratar tarefa ambigua como
// fechada manda o mais barato para um trabalho que ele vai abandonar no meio,
// e custa o run inteiro. Tratar fechada como ambigua compra um modelo um
// pouco melhor, e custa centavos.
const LimiarAutocontida = 0.625

// PisoTauMinimo e o percentil MINIMO de tau_bench exigido numa tarefa pouco
// autocontida, independente de quao baixo seja o corte de complexidade.
// Ambiguidade e eixo proprio: uma tarefa simples e ambigua ainda exige um
// modelo que sustente o enquadramento, e usar o percentil da complexidade
// deixaria essa combinacao sem piso nenhum.
//
// NAO CALIBRADO POR FIXTURE, e nao da para calibrar assim. Ao contrario do
// LimiarAutocontida, este nao e um corte sobre resposta do Jev: e um
// percentil dentro do roster, e nao existe rotulo de verdade dizendo "qual
// percentil de tau basta". Calibra-lo exigiria rodar tarefas ambiguas com
// modelos de tau diferente e medir onde a taxa de sucesso cai — experimento
// que a sessao de 2026-09-21 tentou e nao conseguiu concluir.
//
// O que sustenta 0.50, entao, e o efeito concreto sobre o roster atual, que
// TestPisoExcluiOsFracosEmAgenticoDoRosterReal fixa: ele exclui exatamente
// os dois modelos mais fracos em horizonte longo, e nenhum outro. Se o roster
// mudar de forma que esse teste quebre, o numero precisa ser revisto — e nao
// o teste.
const PisoTauMinimo = 0.50

// LimiarConfiancaDimensao e o corte abaixo do qual a rota deixa de confiar
// na dimensao escolhida. Acima dele o corte de percentil vale no indice
// daquela dimensao; abaixo, o candidato precisa passar o corte nos TRES
// indices — se nao sabemos qual capacidade a tarefa exige, o modelo tem de
// estar acima da linha em todas.
//
// NAO CALIBRADO. O valor vem da orientacao da documentacao do Jev, que usa
// 0.6 como piso abaixo do qual a resposta nao sustenta acao automatica.
// Calibra-lo exige confianca gravada junto com o desfecho do job, que so
// passou a ser guardada agora (Classificacao.Bruto): os oito jogos de
// 2026-09-21 rodaram descartando esse numero.
const LimiarConfiancaDimensao = 0.6

// LimiarConfiancaScore e o corte equivalente para o Score de complexidade.
// Mesmo valor, mesma origem, e a mesma ausencia de calibragem.
const LimiarConfiancaScore = 0.6

// Escolher mantem a assinatura original: rota sem ajuste de autocontencao.
func Escolher(ms []roster.Model, d Dimensao, percentil float64) (Escolha, error) {
	return EscolherCom(ms, d, percentil, Opcoes{})
}

func EscolherCom(ms []roster.Model, d Dimensao, percentil float64, opts Opcoes) (Escolha, error) {
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

	// Tarefa com decisao em aberto: o piso de tau_bench entra, e quem nao o
	// passa sai da disputa por preco. Aplicado ANTES do corte da dimensao,
	// porque sustentar enquadramento e pre-requisito, nao criterio de desempate.
	ambigua := opts.Autocontida > 0 && opts.Autocontida < LimiarAutocontida
	var motivoAmbiguidade string
	var pisoTau map[string]bool
	if ambigua && len(measured) > 0 {
		firmes := comPisoDeTau(measured, max(percentil, PisoTauMinimo))
		switch {
		case len(firmes) > 0:
			// Marca quem passou em vez de reduzir `measured`: o corte da
			// dimensao precisa ser calculado sobre o conjunto INTEIRO, senao
			// o percentil se recalcula dentro do subconjunto e exclui quem
			// deveria passar. Os dois cortes se intersectam no fim.
			pisoTau = make(map[string]bool, len(firmes))
			for _, m := range firmes {
				pisoTau[m.ID] = true
			}
			motivoAmbiguidade = fmt.Sprintf(
				"tarefa pouco autocontida (%.2f): piso de tau_bench aplicado, %d de %d candidatos medidos passaram",
				opts.Autocontida, len(firmes), len(measured))
			unmeasured = nil // sem nota nao compete em tarefa ambigua
		default:
			// Ninguem passa: nao se trava o trabalho, usa-se o melhor tau
			// disponivel e diz-se isso em voz alta.
			melhor := measured[0]
			for _, m := range measured[1:] {
				if m.Benchmark.TauBench > melhor.Benchmark.TauBench {
					melhor = m
				}
			}
			return Escolha{Modelo: melhor, Dimensao: d, Percentil: percentil,
				Motivo: fmt.Sprintf(
					"tarefa pouco autocontida (%.2f) e nenhum candidato passou o piso de tau_bench; "+
						"escolhido o de maior tau (%.3f) em vez de o mais barato",
					opts.Autocontida, melhor.Benchmark.TauBench)}, nil
		}
	}

	// Posição i/(n-1) na ordenação crescente: o melhor está no percentil 1,
	// o pior no 0 — o topo sempre passa qualquer corte válido.
	//
	// Calculada sobre MODELOS DISTINTOS, não sobre entradas do roster. O
	// mesmo modelo aparece em vários canais do proxy — glm-5.3-flash está
	// em cinco, medido em 2026-09-21 — e três entradas idênticas ocupariam
	// três posições na distribuição, empurrando as outras para baixo e
	// fazendo cópias do MESMO modelo caírem em lados opostos do mesmo
	// corte. Qualidade é do modelo; canal não muda peso.
	sort.SliceStable(measured, func(i, j int) bool {
		return indexOf(measured[i], d) < indexOf(measured[j], d)
	})
	posicao := posicaoPorModelo(measured, d)
	// Dimensao escolhida sem confianca: o corte deixa de valer so no indice
	// dela. Se nao sabemos qual capacidade a tarefa exige, o candidato
	// precisa estar acima da linha nos TRES indices — cortar por um eixo
	// escolhido no chute e pior que nao cortar.
	var emTodosOsEixos map[string]bool
	if c := opts.ConfiancaDimensao; c > 0 && c < LimiarConfiancaDimensao && len(measured) > 1 {
		emTodosOsEixos = map[string]bool{}
		for _, m := range measured {
			emTodosOsEixos[m.ID] = true
		}
		for _, eixo := range []Dimensao{Mecanica, Raciocinio, Agentica} {
			for _, id := range abaixoDoCorte(measured, eixo, percentil) {
				delete(emTodosOsEixos, id)
			}
		}
		if len(emTodosOsEixos) == 0 {
			emTodosOsEixos = nil // ninguem passa em tudo: vale o corte da dimensao
		} else {
			motivoAmbiguidade = juntaMotivo(motivoAmbiguidade, fmt.Sprintf(
				"dimensao %s escolhida com confianca %.2f: corte exigido nos tres indices, %d de %d passaram",
				d, opts.ConfiancaDimensao, len(emTodosOsEixos), len(measured)))
		}
	}

	var passing []roster.Model
	for _, m := range measured {
		if posicao[chaveDoModelo(m)] >= percentil && (pisoTau == nil || pisoTau[m.ID]) &&
			(emTodosOsEixos == nil || emTodosOsEixos[m.ID]) {
			passing = append(passing, m)
		}
	}

	// Porta dos sem-benchmark: o corte de percentil mede JULGAMENTO, e um
	// briefing que fecha as decisoes nao pede julgamento — pede execucao
	// fiel. Modelo sem numero de terceiro nao passa pelo corte por falta de
	// DADO, nao por falta de capacidade, e ficava fora da rota por um
	// detalhe de disponibilidade de benchmark.
	//
	// Quem declara `admite_sem_benchmark` no roster esta dizendo: acima
	// desta autocontencao, confio neste modelo para transcrever. A cascata
	// e a rede — desde 2026-09-21 ela escala por estagnacao tambem, entao
	// errar aqui custa uma escalada, nao o run.
	//
	// Autocontida nao informada (zero) mantem a porta fechada: a admissao
	// depende de uma medida que o gate faz, e sem ela nao ha o que julgar.
	// A porta tem DUAS chaves, e a segunda veio de medir a primeira sozinha:
	// admitir so por autocontencao entregava tudo ao sem-benchmark, porque a
	// preferencia de conta domina o desempate e ele nao tem numero de
	// qualidade com que ser comparado. Autocontencao diz que as decisoes
	// estao fechadas; nao diz que o que sobrou e facil.
	if opts.Autocontida > 0 {
		for _, m := range unmeasured {
			if m.AdmiteSemBenchmark <= 0 || opts.Autocontida < m.AdmiteSemBenchmark {
				continue
			}
			if m.AdmiteAtePercentil > 0 && percentil > m.AdmiteAtePercentil {
				continue
			}
			passing = append(passing, m)
			motivoAmbiguidade = juntaMotivo(motivoAmbiguidade, fmt.Sprintf(
				"%s entrou sem benchmark: autocontida %.2f >= %.2f e complexidade %.2f <= %.2f, limiares do roster",
				m.ID, opts.Autocontida, m.AdmiteSemBenchmark, percentil, m.AdmiteAtePercentil))
		}
	}

	// Intersecao vazia: o piso e pre-requisito, entao vale ele sozinho.
	if len(passing) == 0 && pisoTau != nil {
		for _, m := range measured {
			if pisoTau[m.ID] {
				passing = append(passing, m)
			}
		}
	}

	if len(passing) > 0 {
		// Entre os que passaram a MESMA barra de qualidade, primeiro a
		// conta, depois o trabalho. A ordem importa: o mesmo modelo aparece
		// em varios canais do proxy — glm-5.3-flash esta em cinco — com
		// qualidade identica e escassez completamente diferente. Desempatar
		// por tokens antes da conta escolheria o canal certo do modelo
		// errado, e gastaria cota de assinatura apertada onde havia
		// promocao sobrando.
		//
		// O trabalho continua desempatando DENTRO da mesma conta, que e
		// onde ele diz algo: dois modelos do mesmo pote, o que termina com
		// menos tokens deixa mais pote para o proximo job.
		best := passing[0]
		for _, m := range passing[1:] {
			if melhorQue(m, best) {
				best = m
			}
		}
		return Escolha{Modelo: best, Dimensao: d, Percentil: percentil,
			Motivo: motivoAmbiguidade}, nil
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

	// ConfiancaDimensao e a confianca da Choice que escolheu o eixo. Baixa
	// significa que o indice sobre o qual o corte vai operar e chute, e o
	// corte deixa de valer so nele — ver LimiarConfiancaDimensao.
	ConfiancaDimensao float64
	// ConfiancaComplexidade e a concentracao da distribuicao do Score.
	// Baixa significa massa espalhada entre niveis, e o percentil sobe
	// para o nivel seguinte: errar para cima custa um modelo melhor,
	// errar para baixo custa o run inteiro.
	ConfiancaComplexidade float64

	// Bruto sao as respostas inteiras — probabilidade, confianca e
	// distribuicao de cada pergunta, inclusive os atomos de complexidade
	// que ainda nao decidem nada. Guardadas para que mudar um peso da rota
	// seja uma conta sobre os jobs ja rodados, e nao uma nova rodada.
	Bruto jev.Answers
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

	// Os atomos de complexidade vao no mesmo request: perguntas
	// independentes sobre o mesmo state sao avaliadas em paralelo, entao
	// coleta-los nao custa ida a rede nem tempo de resposta.
	perguntas := map[string]jev.Question{
		"dimensao_dominante": qd,
		"complexidade":       qc,
		"volume":             qv,
	}
	todas := jev.RouteQuestions()
	for _, id := range []string{"alcance", "acoplamento", "sutileza"} {
		if q, ok := todas[id]; ok {
			perguntas[id] = q
		}
	}

	state := map[string]any{"tarefa": map[string]any{"texto": briefing}}
	res, err := a.Ask(ctx, state, perguntas)
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
	//
	// Confianca baixa significa massa espalhada entre niveis: a media nao
	// representa nenhum deles. Nesse caso a posicao sobe para o nivel
	// seguinte antes de normalizar. Os dois erros custam coisas diferentes
	// — pedir um modelo melhor do que o necessario custa tokens; pedir um
	// pior custa o run inteiro — e e por isso que o arredondamento e para
	// cima, e nao para o mais proximo.
	nivel := sa.Score
	if sa.Confidence > 0 && sa.Confidence < LimiarConfiancaScore {
		nivel = math.Min(math.Ceil(sa.Score), float64(len(sc.Criteria)-1))
	}
	percentil := min(1, max(0, nivel/float64(len(sc.Criteria)-1)))

	// Volume ausente não invalida a rota: cai no nível 1, que é a âncora do
	// orçamento base. Perder o ajuste de tamanho é pior que parar o trabalho.
	volume := 1.0
	if sv, ok := res.Answers.ScoreOf("volume"); ok {
		volume = sv.Score
	}
	return Classificacao{Dimensao: d, Percentil: percentil, Volume: volume,
		ConfiancaDimensao: ch.Confidence, ConfiancaComplexidade: sa.Confidence,
		Bruto: res.Answers}, res.Usage, nil
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

// trabalhoPorTarefa e o desempate entre os que passaram o corte de
// qualidade: quantos TOKENS o modelo precisa para terminar uma tarefa.
//
// O desempate era o menor custo por tarefa em dolar. Preco nao e criterio
// de rota: quem monta o roster ja decidiu o que cabe no orcamento ao
// escolher quais modelos entram. Entre os que passaram, o que interessa e
// quanto trabalho cada um precisa — menos tokens e menos tempo de parede,
// menos contexto queimado e, de quebra, menos fatura; o custo e o efeito,
// nao a causa.
//
// A diferenca e material no roster de 2026-09-21: por dolar, o deepseek
// (US$ 0,0075/tarefa) parece 66x melhor que o opus-5 (US$ 0,4930). Por
// trabalho, o deepseek precisa de 34k tokens para terminar e o opus de 30k.
// O preco dizia o contrario do que a eficiencia diz.
//
// Sem preco do benchmark nao da para desfazer o custo: o modelo vai para o
// fim da fila em vez de ganhar por um zero.
func trabalhoPorTarefa(m roster.Model) float64 {
	if m.Benchmark == nil {
		// Sem benchmark nao ha trabalho estimado. Vai para o fim da fila do
		// desempate — a conta ja o colocou onde ele deve estar.
		return math.Inf(1)
	}
	t := m.Benchmark.TokensPorTarefa()
	if t <= 0 {
		return math.Inf(1)
	}
	return t
}

// costMTok devolve o custo declarado por token; não declarado é o mais
// caro possível — null não é zero. Só decide onde NAO ha benchmark: sem
// indice nao ha sinal de eficiencia, e preco e o unico dado que sobrou.
func costMTok(m roster.Model) float64 {
	if m.CustoUSDPorMTok == nil {
		return math.Inf(1)
	}
	return *m.CustoUSDPorMTok
}

// comPisoDeTau devolve os candidatos cujo tau_bench fica no percentil pedido
// ou acima, dentro do proprio conjunto. Mesma mecanica do corte de dimensao:
// posicao relativa, porque os indices nao sao comparaveis entre si.
func comPisoDeTau(ms []roster.Model, percentil float64) []roster.Model {
	ordenado := append([]roster.Model(nil), ms...)
	sort.SliceStable(ordenado, func(i, j int) bool {
		return ordenado[i].Benchmark.TauBench < ordenado[j].Benchmark.TauBench
	})
	var passa []roster.Model
	for i, m := range ordenado {
		pos := 1.0
		if len(ordenado) > 1 {
			pos = float64(i) / float64(len(ordenado)-1)
		}
		if pos >= percentil {
			passa = append(passa, m)
		}
	}
	return passa
}

// abaixoDoCorte lista quem NAO alcanca o percentil no indice deste eixo. A
// posicao e sempre calculada sobre o conjunto inteiro: filtrar antes
// recalcularia o percentil dentro do subconjunto e excluiria quem deveria
// passar — o mesmo erro que o piso de tau evita marcando em vez de reduzir.
func abaixoDoCorte(ms []roster.Model, d Dimensao, percentil float64) []string {
	ordenado := append([]roster.Model(nil), ms...)
	sort.SliceStable(ordenado, func(i, j int) bool {
		return indexOf(ordenado[i], d) < indexOf(ordenado[j], d)
	})
	var fora []string
	for i, m := range ordenado {
		pos := 1.0
		if len(ordenado) > 1 {
			pos = float64(i) / float64(len(ordenado)-1)
		}
		if pos < percentil {
			fora = append(fora, m.ID)
		}
	}
	return fora
}

// juntaMotivo encadeia motivos sem perder o primeiro: a rota pode ter
// aplicado piso de tau E corte em todos os eixos, e o relatorio precisa
// dizer os dois.
func juntaMotivo(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

// melhorQue ordena dois candidatos que ja passaram o corte de qualidade:
// conta primeiro, trabalho depois.
func melhorQue(a, b roster.Model) bool {
	if oa, ob := a.Conta.Ordem(), b.Conta.Ordem(); oa != ob {
		return oa < ob
	}
	return trabalhoPorTarefa(a) < trabalhoPorTarefa(b)
}

// chaveDoModelo identifica o MODELO por trás de uma entrada do roster. O
// permaslug é o nome do peso no benchmark de terceiro, e é o que duas
// entradas do mesmo modelo em canais diferentes compartilham. Sem ele, a
// entrada é o próprio modelo.
func chaveDoModelo(m roster.Model) string {
	if m.Permaslug != "" {
		return m.Permaslug
	}
	return m.ID
}

// posicaoPorModelo devolve a posição de cada modelo distinto na ordenação
// crescente do índice da dimensão: o melhor em 1, o pior em 0. Canais do
// mesmo modelo recebem a mesma posição, porque têm o mesmo peso.
func posicaoPorModelo(ms []roster.Model, d Dimensao) map[string]float64 {
	type entrada struct {
		chave string
		idx   float64
	}
	vistos := map[string]bool{}
	var distintos []entrada
	for _, m := range ms {
		k := chaveDoModelo(m)
		if vistos[k] {
			continue
		}
		vistos[k] = true
		distintos = append(distintos, entrada{k, indexOf(m, d)})
	}
	sort.SliceStable(distintos, func(i, j int) bool { return distintos[i].idx < distintos[j].idx })

	pos := make(map[string]float64, len(distintos))
	for i, e := range distintos {
		p := 1.0
		if len(distintos) > 1 {
			p = float64(i) / float64(len(distintos)-1)
		}
		pos[e.chave] = p
	}
	return pos
}
