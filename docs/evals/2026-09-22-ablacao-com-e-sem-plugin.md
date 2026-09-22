# O mesmo modelo, com e sem o plugin

**2026-09-22.** `cpa-claude-opus-5(high)`, três tarefas, seis execuções.

## Veredito

**O plugin não mudou o resultado de nenhuma das três.** Os dois braços
fizeram 3 de 3, e os dois passaram na sonda de mutação.

| braço | tarefa | veredito | turnos | tokens entrada | seg | arq. extra |
|---|---|---|---:|---:|---:|---:|
| sem plugin | fácil | verde | 10 | 199.328 | 128 | 0 |
| **com plugin** | fácil | verde | 18 | 393.412 | 195 | 0 |
| sem plugin | médio | verde | 16 | 670.528 | 167 | **1** |
| **com plugin** | médio | verde | 17 | 701.466 | 184 | 0 |
| sem plugin | difícil | verde | 34 | 2.477.085 | 507 | 0 |
| **com plugin** | difícil | verde | 34 | 2.525.092 | 548 | 0 |

Total: **3.346.941 tokens e 802 s** sem o plugin, **3.619.970 e 927 s** com
ele. O plugin custou **+8% de tokens e +16% de relógio**.

O sobrecusto não é uniforme: **+97% de tokens na tarefa fácil, +2% na
difícil**. Ele é quase todo custo fixo — os gates, a seleção de evidência e
a rota rodam igual num trabalho de dez turnos e num de trinta e quatro.
Quanto maior a tarefa, menos ele pesa.

## O que o plugin entregou que o controle não entregou

**A prova, automaticamente.** Os dois braços produziram correções reais —
apliquei a sonda de mutação à mão nos três resultados do controle, e os três
ficam vermelhos sem a correção. Mas **eu só soube fazer isso porque o plugin
define o padrão.** O controle devolve um teste verde; ele não diz nada sobre
o teste valer alguma coisa. A saída do plugin é uma afirmação com evidência
anexada: `teste: exit 0 | mutacao: exit 1 (esperado) | suite: exit 0 | lint:
exit 0`, mais `relato: 3 de 3 comandos conferem com o trace` nas três.

**Um desvio de escopo a menos.** Na tarefa média o controle escreveu um
arquivo além dos que o mantenedor tocou; o braço do plugin, não. Um caso —
não é tendência.

**Um piso quando dá errado.** O controle rodou 34 turnos na tarefa difícil
sem teto de custo, sem checagem de ociosidade e sem cascata. Deu certo. Um
run do controle que der errado não tem o que o interrompa.

## O que isto NÃO autoriza a concluir

**Três tarefas, um repositório, uma linguagem, um modelo.** E um modelo
forte: o mesmo `opus-5` em esforço `low` falhou a tarefa difícil duas vezes
em 2026-09-21 e só saiu com escalada. **O esforço foi a variável dominante,
muito acima da presença do plugin.**

E o braço do controle usou o mesmo modelo de propósito, para isolar as
camadas. Isso deixa de fora justamente a proposta principal do plugin —
fazer o trabalho sair de uma conta mais barata. Essa comparação é outra, e
ainda não foi feita.

## Terceiro braço: o plugin roteando para o barato

A comparação acima usou o mesmo modelo nos dois lados, de propósito, para
isolar as camadas. Isso deixava de fora a proposta principal do plugin. O
terceiro braço a mede: roster informado pelo piloto, `devin/swe-2` no
primeiro degrau e `opus-5(high)` como alvo de escalada, cascata **ligada**.

| braço | executor | conta | verdes | tokens entrada | seg |
|---|---|---|---:|---:|---:|
| sem plugin | `opus-5(high)` | **Claude Max** | 3/3 | **3.346.941** | 802 |
| com plugin, mesmo modelo | `opus-5(high)` | **Claude Max** | 3/3 | 3.619.970 | 927 |
| **com plugin, cascata** | `devin/swe-2` | **Devin (promoção)** | **3/3** | **1.561.872** | 785 |

**Mesmo resultado, 100% da cota de fronteira preservada, e o relógio
empatado.** O `swe-2` resolveu as três sozinho — inclusive a difícil, que o
`opus-5(low)` falhou duas vezes em 2026-09-21. Os quatro passos da
verificação ficaram verdes nas três, com a sonda de mutação provando o teste.

**A cascata não precisou disparar.** Não foi "barato primeiro, escala na
falha": foi "o barato bastou". O mecanismo de escalada ficou sem exercício
neste braço.

### As ressalvas, e elas são grandes

**O `swe-2` falhou esta mesma tarefa difícil no piloto**, ontem, entregando
uma resposta errada em 38 turnos. Hoje resolveu em 23. É a mesma variância
que já apareceu na issue #685 — verde, veto e verde em três execuções
idênticas. Com n=1 por célula, esta tabela não distingue "resolve" de "deu
certo desta vez".

**O roster deste braço foi escrito por mim, informado pelo piloto.** Não é o
que o `config/roster.yaml` produz: com o corte por benchmark de terceiro,
estas três tarefas vão todas para o Claude Max. A economia medida aqui
depende de uma mudança de rota que ainda não está no produto.

**E a conferência de veracidade acusou o que não devia.** Duas células
saíram com "o relatorio DIVERGE" quando o que havia era conferência
inconclusiva a 0,77 e 0,78 — logo abaixo do corte de 0,8. O `Incerto` é
definido como "não acusa nem absolve", e o relatório o estava tratando como
acusação. Corrigido.

## A leitura honesta

Com cota de fronteira sobrando e disposição para conferir o trabalho à mão,
**o `opus-5(high)` sozinho resolve estas três tarefas e custa 8% menos.**

O plugin se paga quando o executor **não** é o modelo caro, e quando ninguém
vai conferir à mão. A primeira agora tem número: **1,56 M de tokens de uma
conta promocional contra 3,35 M da cota que roda esta sessão** — mesmo
resultado, mesmo tempo.

Dito de outro jeito: comparado com o modelo caro sozinho, o plugin custa 8% a
mais **quando executa no mesmo modelo**, e economiza a cota inteira **quando
não executa**. O valor dele nunca esteve em executar melhor; está em não
precisar do caro para executar.

O que falta para isso valer no produto, e não só neste eval, é a rota saber
disso — hoje ela decide por benchmark de terceiro, e o benchmark manda estas
três tarefas para o Claude Max.
