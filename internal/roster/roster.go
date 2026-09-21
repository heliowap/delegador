// Package roster carrega a lista curada de modelos do config/roster.yaml e
// decide quais entram na rota. O roster é curadoria humana, não descoberta
// automática: nada entra por estar disponível no proxy.
package roster

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Model é uma entrada do roster, com as três origens de dado que não se
// misturam: sondado (medido contra o proxy local), benchmark (terceiro) e
// humano (preenchido à mão — custo null não é zero, é "não declarado").
type Model struct {
	ID              string
	Permaslug       string
	Papel           string
	Mapeamento      string
	Sondado         Probe
	Benchmark       *Benchmark
	CustoUSDPorMTok *float64
	// CustoSaidaUSDPorMTok e opcional: ausente, cai no preco de entrada, o
	// que preserva o comportamento de quem nao o declarou. Existe porque
	// saida custa de 3 a 5 vezes a entrada e um numero so subestimava a
	// conta na mesma proporcao.
	CustoSaidaUSDPorMTok *float64
	Habilitado           bool
	// Conta e o pote de onde este modelo e pago, resolvido do bloco
	// `contas:` do arquivo. Zero quando o modelo nao declarou nenhuma —
	// e conta omitida nao vira preferida, ver Conta.Ordem.
	Conta Conta
}

// Probe é o que a sondagem mediu numa chamada mínima ao modelo. Perde
// validade sem aviso — `doctor --probe` remede.
type Probe struct {
	ToolCall         bool
	ReasoningContent bool
	TokensBase       int
	LatenciaS        float64
	Em               time.Time
}

// Benchmark são os números de terceiro (OpenRouter /api/v1/benchmarks),
// sobre uma configuração que pode não ser a sua. Os nomes JSON seguem o
// formato do cache em disco.
type Benchmark struct {
	CodingIndex       float64 `json:"coding_index"`
	IntelligenceIndex float64 `json:"intelligence_index"`
	AgenticIndex      float64 `json:"agentic_index"`
	TauBench          float64 `json:"tau_bench"`
	TauBenchDesvio    float64 `json:"tau_bench_desvio"`
	CustoPorTarefaUSD float64 `json:"custo_por_tarefa_usd"`

	// Os precos com que o benchmark de terceiro mediu CustoPorTarefaUSD.
	// Existem para dividir o custo por eles e recuperar o TAMANHO da
	// tarefa em tokens, que e o que a rota quer saber: quanto trabalho o
	// modelo precisa para terminar. O preco e do fornecedor do benchmark e
	// nao e o preco de quem roda — para isso valem os campos de `humano`.
	PrecoEntradaUSDPorMTok float64 `json:"preco_entrada_usd_por_mtok"`
	PrecoSaidaUSDPorMTok   float64 `json:"preco_saida_usd_por_mtok"`
}

// ProporcaoSaidaEntrada e quanta saida um laco agentico gera por token de
// entrada. MEDIDO em 2026-09-21 sobre 7.142.353 tokens de entrada e 93.556
// de saida, somando as catorze execucoes do eval contra o expr-lang/expr:
// a saida e 1,31% da entrada.
//
// O numero e pequeno porque o contexto e reenviado inteiro a cada turno,
// enquanto a resposta de cada turno e uma chamada de ferramenta curta. A
// primeira versao desta derivacao usava a media simples entre os dois
// precos — o equivalente a supor 50% de saida — e a medicao a falsificou.
const ProporcaoSaidaEntrada = 0.0131

// TokensPorTarefa estima quantos tokens o modelo gasta para terminar uma
// tarefa do benchmark, desfazendo o preco de dentro do custo medido.
//
// custo = entrada x preco_entrada + saida x preco_saida, e com
// saida = r x entrada isso vira entrada = custo / (preco_entrada +
// r x preco_saida). O total soma as duas pontas.
//
// A conta supoe que as tarefas do benchmark tem a mesma proporcao das
// nossas. Nao da para verificar — o benchmark publica custo, nao tokens —
// mas a suposicao agora e explicita e vem de medicao, em vez de ser um
// 50/50 escolhido por falta de dado.
//
// O VALOR ABSOLUTO nao serve para prever o custo de um run: medido, o
// opus-5 precisou de ~250k tokens onde esta conta preve ~85k, e o fable-5-1
// de 517k a 2.064k onde ela preve ~138k. O que a rota usa e a ORDEM, e essa
// se sustentou no unico confronto direto disponivel — a issue #685, em que
// os dois terminaram verdes: previsto 1,62x, medido 2,07x.
//
// Zero quando falta preco: sem ele nao ha o que desfazer.
func (b *Benchmark) TokensPorTarefa() float64 {
	if b == nil || b.CustoPorTarefaUSD <= 0 {
		return 0
	}
	entrada, saida := b.PrecoEntradaUSDPorMTok, b.PrecoSaidaUSDPorMTok
	if saida == 0 {
		saida = entrada
	}
	efetivo := entrada + ProporcaoSaidaEntrada*saida
	if efetivo <= 0 {
		return 0
	}
	return b.CustoPorTarefaUSD / efetivo * 1e6 * (1 + ProporcaoSaidaEntrada)
}

// Load lê o roster YAML. O formato é fixo e raso — escalares, uma lista de
// mapas com dois níveis e a nota dobrada — então o parse é feito à mão em
// vez de puxar uma dependência.
func Load(path string) ([]Model, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("roster: lendo %s: %w", path, err)
	}
	return parse(raw)
}

// secao é o mapa aninhado aberto dentro do modelo corrente.
type secao int

const (
	secaoNenhuma secao = iota
	secaoSondado
	secaoBenchmark
	secaoHumano
)

func parse(raw []byte) ([]Model, error) {
	var (
		ms           []Model
		atual        *Model
		emModelos    bool
		sec          secao
		pularMaisQue = -1 // indent do bloco a ignorar (nota dobrada, mapa desconhecido)
		asOfSondagem time.Time

		// contas: declaradas no topo, referenciadas por nome dentro de cada
		// modelo. Resolvidas no fim, com erro em nome desconhecido — um
		// typo virando "conta nao declarada" em silencio mudaria a rota.
		contas     = map[string]*Conta{}
		emContas   bool
		contaAtual *Conta
		refConta   = map[int]string{} // indice do modelo -> nome declarado
	)

	for i, linha := range strings.Split(string(raw), "\n") {
		errf := func(format string, args ...any) error {
			return fmt.Errorf("roster: linha %d: %s", i+1, fmt.Sprintf(format, args...))
		}

		linha = tiraComentario(linha)
		if strings.TrimSpace(linha) == "" {
			continue
		}
		indent := len(linha) - len(strings.TrimLeft(linha, " "))
		campo := strings.TrimSpace(linha)

		if pularMaisQue >= 0 {
			if indent > pularMaisQue {
				continue
			}
			pularMaisQue = -1
		}

		if indent == 0 {
			// Nível raiz: escalares soltos e a abertura da lista.
			atual = nil
			sec = secaoNenhuma
			k, v, _ := strings.Cut(campo, ":")
			emContas = false
			switch strings.TrimSpace(k) {
			case "modelos":
				emModelos = true
			case "contas":
				emContas = true
			case "as_of_sondagem":
				d, err := time.Parse("2006-01-02", unquote(strings.TrimSpace(v)))
				if err != nil {
					return nil, errf("as_of_sondagem: %v", err)
				}
				asOfSondagem = d
			}
			continue
		}
		if emContas {
			chave, valor, _ := strings.Cut(campo, ":")
			chave, valor = strings.TrimSpace(chave), strings.TrimSpace(valor)
			if indent <= 2 {
				c := &Conta{Nome: chave}
				contas[chave] = c
				contaAtual = c
				continue
			}
			if contaAtual == nil {
				return nil, errf("campo de conta fora de uma conta: %s", campo)
			}
			switch chave {
			case "escassez":
				contaAtual.Escassez = valorEscalar(valor)
			case "aperto":
				contaAtual.Aperto = valorEscalar(valor)
			case "orquestrador":
				contaAtual.Orquestrador = ehTrue(valor)
			case "prioridade":
				n, err := strconv.Atoi(valor)
				if err != nil {
					return nil, errf("prioridade: %v", err)
				}
				contaAtual.Prioridade = n
			case "ate":
				d, err := time.Parse("2006-01-02", unquote(valor))
				if err != nil {
					return nil, errf("ate: %v", err)
				}
				contaAtual.Ate = d
			}
			continue
		}
		if !emModelos {
			return nil, errf("conteúdo fora de 'modelos': %s", campo)
		}

		if indent <= 4 && strings.HasPrefix(campo, "- ") {
			ms = append(ms, Model{})
			atual = &ms[len(ms)-1]
			sec = secaoNenhuma
			campo = strings.TrimSpace(campo[2:])
			if campo == "" {
				continue
			}
		}
		if atual == nil {
			return nil, errf("campo fora de modelo: %s", campo)
		}

		chave, valor, _ := strings.Cut(campo, ":")
		chave, valor = strings.TrimSpace(chave), strings.TrimSpace(valor)

		if indent <= 4 {
			sec = secaoNenhuma
			switch chave {
			case "id":
				atual.ID = valorEscalar(valor)
			case "papel":
				atual.Papel = valorEscalar(valor)
			case "permaslug":
				atual.Permaslug = valorEscalar(valor)
			case "mapeamento":
				atual.Mapeamento = valorEscalar(valor)
			case "conta":
				refConta[len(ms)-1] = valorEscalar(valor)
			case "sondado":
				sec = secaoSondado
			case "benchmark":
				// Valor vazio abre o mapa; "benchmark: null" deixa nil,
				// que é o valor certo — não um Benchmark zerado.
				if valor == "" {
					sec = secaoBenchmark
				}
			case "humano":
				sec = secaoHumano
			default:
				// Chave desconhecida: valor inline (escalar ou flow map)
				// termina na própria linha; bloco embaixo (mapa ou escalar
				// dobrado `>`/`|`) se pula pelas linhas mais indentadas.
				if valor == "" || strings.HasPrefix(valor, ">") || strings.HasPrefix(valor, "|") {
					pularMaisQue = indent
				}
			}
			continue
		}

		switch sec {
		case secaoSondado:
			switch chave {
			case "tool_call":
				atual.Sondado.ToolCall = ehTrue(valor)
			case "reasoning_content":
				atual.Sondado.ReasoningContent = ehTrue(valor)
			case "em":
				// Data da sondagem DESTE modelo, gravada pelo `roster
				// --probe`/`doctor --probe`. Prevalece sobre o
				// as_of_sondagem do arquivo: uma re-sondagem parcial nao
				// pode empurrar a data dos modelos nao re-sondados.
				d, err := time.Parse("2006-01-02", unquote(valor))
				if err != nil {
					return nil, errf("em: %v", err)
				}
				atual.Sondado.Em = d
			case "tokens_base":
				n, err := strconv.Atoi(valor)
				if err != nil {
					return nil, errf("tokens_base: %v", err)
				}
				atual.Sondado.TokensBase = n
			case "latencia_s":
				f, err := strconv.ParseFloat(valor, 64)
				if err != nil {
					return nil, errf("latencia_s: %v", err)
				}
				atual.Sondado.LatenciaS = f
			}
		case secaoBenchmark:
			if atual.Benchmark == nil {
				atual.Benchmark = &Benchmark{}
			}
			var p *float64
			switch chave {
			case "coding_index":
				p = &atual.Benchmark.CodingIndex
			case "intelligence_index":
				p = &atual.Benchmark.IntelligenceIndex
			case "agentic_index":
				p = &atual.Benchmark.AgenticIndex
			case "tau_bench":
				p = &atual.Benchmark.TauBench
			case "tau_bench_desvio":
				p = &atual.Benchmark.TauBenchDesvio
			case "custo_por_tarefa_usd":
				p = &atual.Benchmark.CustoPorTarefaUSD
			case "preco_openrouter_usd_por_mtok":
				// Unico mapa em linha do arquivo: {entrada: 0.15, saida: 0.50}.
				// Vale um regex contido em vez de um terceiro nivel no parser.
				ent, sai, err := precoEmLinha(valor)
				if err != nil {
					return nil, errf("preco_openrouter_usd_por_mtok: %v", err)
				}
				atual.Benchmark.PrecoEntradaUSDPorMTok = ent
				atual.Benchmark.PrecoSaidaUSDPorMTok = sai
			}
			if p != nil {
				f, err := strconv.ParseFloat(valor, 64)
				if err != nil {
					return nil, errf("%s: %v", chave, err)
				}
				*p = f
			}
		case secaoHumano:
			switch chave {
			case "custo_usd_por_mtok":
				if !ehNulo(valor) {
					f, err := strconv.ParseFloat(valor, 64)
					if err != nil {
						return nil, errf("custo_usd_por_mtok: %v", err)
					}
					atual.CustoUSDPorMTok = &f
				}
			case "custo_saida_usd_por_mtok":
				if !ehNulo(valor) {
					f, err := strconv.ParseFloat(valor, 64)
					if err != nil {
						return nil, errf("custo_saida_usd_por_mtok: %v", err)
					}
					atual.CustoSaidaUSDPorMTok = &f
				}
			case "habilitado":
				atual.Habilitado = ehTrue(valor)
			}
		}
	}

	// Resolve as contas por nome. Nome desconhecido e erro: um typo
	// silenciosamente virando "conta nao declarada" mudaria a preferencia
	// da rota sem ninguem ver.
	for i, nome := range refConta {
		c, ok := contas[nome]
		if !ok {
			return nil, fmt.Errorf("roster: modelo %q declara conta %q, que nao esta em `contas:`",
				ms[i].ID, nome)
		}
		ms[i].Conta = *c
	}

	// A data da sondagem é do arquivo inteiro, mas só onde o modelo não
	// declarou a sua própria (`em`): o as_of_sondagem vale como a data da
	// sondagem original em bloco.
	for i := range ms {
		if ms[i].Sondado.Em.IsZero() {
			ms[i].Sondado.Em = asOfSondagem
		}
	}
	return ms, nil
}

// Elegiveis devolve os modelos utilizáveis e, para cada excluído, o motivo.
// Exclusão silenciosa é o pior modo de falha aqui: o usuário precisa saber
// que o modelo dele saiu da rota e por quê.
func Elegiveis(ms []Model, maxIdade time.Duration, agora time.Time) ([]Model, []string) {
	var elegiveis []Model
	var motivos []string
	for _, m := range ms {
		if r := exclusao(m, maxIdade, agora); r != "" {
			motivos = append(motivos, m.ID+": "+r)
		} else {
			elegiveis = append(elegiveis, m)
		}
	}
	return elegiveis, motivos
}

// exclusao devolve o primeiro motivo que tira o modelo da rota, na ordem em
// que o operador diagnosticaria: desligado, sem custo, sondagem vencida,
// sondagem sem tool call.
func exclusao(m Model, maxIdade time.Duration, agora time.Time) string {
	switch {
	case !m.Habilitado:
		return "desabilitado pelo humano"
	case m.CustoUSDPorMTok == nil:
		return "humano.custo_usd_por_mtok é null: sem custo declarado não compete por preço"
	case agora.Sub(m.Sondado.Em) > maxIdade:
		return fmt.Sprintf("sondagem de %s mais velha que %s",
			m.Sondado.Em.Format("2006-01-02"), maxIdade)
	case !m.Sondado.ToolCall:
		return "sondagem não registrou tool call"
	}
	return ""
}

// tiraComentario remove o comentário de fim de linha: '#' no início ou após
// espaço, fora de aspas.
func tiraComentario(s string) string {
	var aspas byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case aspas != 0:
			if c == aspas {
				aspas = 0
			}
		case c == '"' || c == '\'':
			aspas = c
		case c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return strings.TrimRight(s[:i], " \t")
		}
	}
	return s
}

// valorEscalar devolve o escalar sem aspas; null vira string vazia.
func valorEscalar(v string) string {
	if v = unquote(v); ehNulo(v) {
		return ""
	}
	return v
}

func unquote(v string) string {
	if len(v) >= 2 && v[0] == v[len(v)-1] && (v[0] == '"' || v[0] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

func ehNulo(v string) bool {
	return v == "" || v == "null" || v == "~"
}

func ehTrue(v string) bool {
	return v == "true"
}

// precoEmLinha le `{entrada: 0.15, saida: 0.50}`. Ausencia de um dos dois
// nao e erro: saida vazia cai no preco de entrada em TokensPorTarefa.
func precoEmLinha(v string) (entrada, saida float64, err error) {
	campo := func(nome string) (float64, error) {
		m := regexp.MustCompile(nome + `\s*:\s*([0-9.]+)`).FindStringSubmatch(v)
		if m == nil {
			return 0, nil
		}
		return strconv.ParseFloat(m[1], 64)
	}
	if entrada, err = campo("entrada"); err != nil {
		return 0, 0, err
	}
	if saida, err = campo("saida"); err != nil {
		return 0, 0, err
	}
	return entrada, saida, nil
}
