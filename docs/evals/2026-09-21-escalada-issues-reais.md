# Escalada contra issues reais — expr-lang/expr

**2026-09-21.** Seis bugs fechados de um repositório de terceiro, cada um no
commit anterior à correção, com o teste do mantenedor já no disco.

## Veredito

**A cascata não escalou nenhuma vez nas oito execuções — e o motivo era um
defeito do desenho, não do modelo.** Corrigido no mesmo dia, ela disparou na
nona (ver *Adendo*, ao fim).

Não foi por o modelo ter acertado sempre: ele falhou em três dos seis casos.
Foi porque **nenhuma falha chegou na forma que a cascata exige**. A regra do
spec (§6.5, §14) é escalar só por verificação vermelha de um resultado
*entregue*; os três fracassos pararam por veto do watchdog antes disso — dois
por `sem_escrita`, um por `teto_de_custo`. Em cada um deles o veto foi a
decisão certa. Somados, porém, eles dizem uma coisa que o spec não previa:

> Em trabalho real, o watchdog é a restrição que corta primeiro. O gatilho da
> cascata — "entregou e a verificação reprovou" — pode ser raro na prática.

Três resultados verdes, dois deles com a correção **idêntica à do mantenedor**.

## Método, e por que ele não é enviesado

O defeito das tentativas anteriores foi eu desenhar a tarefa sabendo onde
estava a dificuldade. Aqui a tarefa, o oráculo e a seleção vêm de fora.

**Seleção, mecânica.** Dos 400 commits mais recentes do `expr-lang/expr`,
filtrei os que (a) tocam ao menos um `.go` de produção e um `_test.go`, (b)
citam um PR, e (c) cujo PR tem issue formalmente vinculada. Peguei os **seis
primeiros em ordem de recência**. Nenhum descarte por dificuldade aparente.

**Oráculo, do mantenedor.** Para cada caso montei um repositório novo com a
árvore do commit **pai** da correção — sem remote, sem object store com o
futuro, para o modelo não poder achar o commit que corrige. Sobre ele
apliquei só os **arquivos de teste** da correção e commitei. Os seis ficam
vermelhos, cada um na asserção que o mantenedor escreveu.

**Tarefa, do relator.** O texto é o título e o corpo da issue no GitHub,
verbatim. Nenhum deles nomeia arquivo ou linha.

**Triagem, mecânica.** O gate de delegabilidade exige `arquivo:linha` e uma
issue de usuário não traz isso — o papel de localizar é do orquestrador. Para
não localizar eu mesmo (o que seria vazar parte da resposta), a triagem é um
`grep` do identificador que **a própria issue nomeia**, um hit por arquivo,
ordem de caminho, no máximo 12. Os termos estão em `termos.tsv` do harness,
com a citação da issue de onde cada um saiu.

## Resultados

| # | issue | veredito | parada | turnos | US$ | correção vs. mantenedor |
|---|---|---|---|---|---|---|
| 1 | [#950](https://github.com/expr-lang/expr/issues/950) | **verde** | final | 13 | 1,69 | **idêntica** (2 `OpPop` em `emitLoopBackwards`) |
| 2 | [#823](https://github.com/expr-lang/expr/issues/823) | verde, vetado | `teto_de_custo` | 19 | 3,65 | outra rota, também válida (`checker.go` em vez de `expr.go`+patcher) |
| 3 | [#888](https://github.com/expr-lang/expr/issues/888) | **verde** | final | 8 | 1,40 | **idêntica byte a byte** (`+fnInOffset` em `InElem`) |
| 4 | [#836](https://github.com/expr-lang/expr/issues/836) | vermelho | `sem_escrita` | 12 | 2,17 | nada escrito |
| 4″ | #836 (teto US$ 12, pós-revisão) | **verde, escalado** | final | 13+28 | 24,34 | **mesmos 2 arquivos** do mantenedor |
| 4′ | #836 (repetição) | vermelho | `teto_de_custo` | 22 | 6,74 | 4 arquivos, suíte e lint quebrados |
| 5 | [#685](https://github.com/expr-lang/expr/issues/685) | vermelho | `sem_escrita` | 10 | 1,61 | nada escrito |
| 5′ | #685 (repetição) | **verde** | final | 10 | 1,32 | outra rota, suíte inteira verde |
| 6 | [#857](https://github.com/expr-lang/expr/issues/857) | **verde** | final | 5 | 0,28 | outra rota, maior (+22 −10 contra +6 −4) |

Executor US$ 18,86. Jev US$ 0,041 — **0,2% do custo**, e é ele que decide
tudo que não é a execução.

Todos os oito runs rotearam para `cpa-claude-opus-5(low)`, dimensão
`raciocinio`. Nenhum outro modelo do roster foi escolhido uma única vez.

## O que isso mede

**A cascata nunca teve chance de disparar, e a razão é estrutural.** O corte
de complexidade manda bug real de compilador para o topo do roster. Do topo
não se escala — e, mais importante, quem está no topo raramente termina com
"entreguei e a verificação reprovou": termina estourando o orçamento, porque
é o modelo mais caro rodando contra um teto em dólares fixo.

Há um acoplamento aí que o spec não trata: **rota cara + teto absoluto = o
veto de custo corta antes de a tarefa acabar**. O caso 4′ é o exemplo: 22
turnos, US$ 6,74, quatro arquivos mexidos, suíte e lint vermelhos, cortado
pelo teto. Se o teto fosse maior, esse run teria terminado em `final` com
verificação vermelha — que é exatamente o gatilho da cascata.

**A variância entre dois runs idênticos é maior que a margem do watchdog.**
Caso 5 vetou por `sem_escrita` numa execução e fechou verde em nove turnos na
outra, mesmo modelo, mesmo prompt. Caso 4 vetou por ociosidade numa e gastou
o dobro do teto na outra. O limiar de ociosidade está dentro do ruído do
próprio modelo.

## Defeitos encontrados — quatro, todos corrigidos

Nenhum deles aparecia em teste unitário. Todos vieram de rodar contra um
repositório de verdade.

**1. A rede de evidência preservava tipo, não propriedade.** Havia dois
`trecho`: o diff do teste, sem linha, e a triagem, com `arquivo:linha`. A
seleção descartou os dois; a rede resgatou o primeiro; o gate
`aponta_arquivo_linha` reprovou a tarefa. O comentário no código afirmava
"`trecho` supre aponta_arquivo_linha" — não supre. Agora há uma segunda rede
sobre a **propriedade**, com escopo no trecho de propósito: um `erro` quase
sempre cita a linha do teste que falhou, que é o sintoma, não o defeito.

**2. O briefing anunciava os testes errados como critério de aceite.** A lista
de "testes já escritos" eram os seis primeiros `*_test.go` em ordem
alfabética. Numa worktree de tarefa única isso acerta; no `expr` anunciou
`ast/find_test.go` e `bench_test.go`. Truncar uma lista é apresentar palpite
como fato: agora ou ela vai completa, ou não vai.

**3. Veto era contado como entrega reprovada.** No caso 2 os quatro passos
ficaram verdes e a sonda de mutação provou o teste; só então o custo passou do
teto. O relatório dizia `CANCELADO` no cabeçalho e `veredito: verde` três
linhas abaixo, e o run saía com 1 — o mesmo código de uma entrega reprovada.
São desfechos diferentes e pedem decisões diferentes. Veto agora sai com 4, e
quando ele chega depois do verde o relatório reconcilia as duas linhas.

**4. Um run vetado não guardava como se defender.** Os dois `sem_escrita`
deixaram só o trace compactado — dois turnos de doze. A compactação seleciona
o que importa para a **entrega**, e um run vetado não tem entrega. E a
pergunta do operador tem duas respostas muito diferentes, porque o contador do
`sem_escrita` só zera com escrita **bem-sucedida**: ou o modelo estava lendo
para entender, ou tentando escrever e falhando. Agora o `turns.jsonl` guarda o
bruto. Foi com ele que se soube que, no caso 4, o modelo começou a escrever no
turno 10 — o veto da primeira execução cortou um turno antes disso.

## Adendo: a cascata disparou (mesmo dia, depois da revisao do invariante)

Com o §6.5 revisado — veto de **estagnacao** deixa de curto-circuitar e cai
na mesma verificacao que julga qualquer entrega — a issue #685 foi repetida.

```
1a tentativa  cpa-claude-opus-5(low)   veto sem_escrita, 10 turnos, diff vazio
              verificacao: vermelha
escalou:      cpa-claude-opus-5(low) -> claude-fable-5-1
              (sem_escrita e verificacao vermelha)
2a tentativa  claude-fable-5-1         9 turnos
              teste exit 0 | mutacao exit 1 (esperado) | suite exit 0 | lint exit 0
relato:       3 de 3 comandos conferem com o trace
```

**Verde.** E os tres arquivos que o modelo escalado tocou — `checker/checker.go`,
`checker/nature/nature.go`, `vm/vm.go` — sao **exatamente os tres** do commit
`775fc3ac` do mantenedor. Conferido por fora: 51 pacotes verdes.

Total do job US$ 7,05 em 156 segundos, somando as duas tentativas.

Duas correcoes tiveram que entrar junto para que isso fosse possivel:

- **O gatilho.** `sem_escrita` nao e o ambiente falhando: e o watchdog
  medindo o executor empacado, que e a situacao que a cascata existe para
  resolver. A estagnacao nao decide sozinha — ela so deixa de
  curto-circuitar, e a verificacao continua sendo quem prova a falha.
- **O orcamento.** O contador de custo e acumulado por job, entao a segunda
  tentativa comecava gastando o que a primeira gastou; `MaxEscaladas: 1` era
  promessa que o codigo nao cumpria. Cada tentativa recebe o teto de novo.

Uma escalada bem-sucedida nao e uma taxa de acerto. O que ela prova e que o
mecanismo existe fora do teste unitario, que o re-roteio troca de modelo de
verdade e que a verificacao pos-escalada e a mesma. Quantas vezes vale a
pena escalar continua sem medida.

### O caso 4, com o teto levantado

O #836 era o que faltava: parava por teto de custo, que e orcamento nosso e
por definicao nao escala. Com `--teto-usd 12` e `--max-turns 45`:

```
1a tentativa  cpa-claude-opus-5(low)   13 turnos, US$  2,33, 441k tokens de entrada
              veto sem_escrita, diff vazio, verificacao vermelha
escalou:      cpa-claude-opus-5(low) -> claude-fable-5-1
2a tentativa  claude-fable-5-1         28 turnos, US$ 22,02, 2.030k tokens de entrada
              teste exit 0 | mutacao exit 1 (esperado) | suite exit 0 | lint exit 0
relato:       go test ./... — conferencia inconclusiva (confianca 0,72)
```

**Verde**, em dois arquivos — `checker/checker.go` e `compiler/compiler.go` —
que sao **exatamente os dois** do commit do mantenedor. 53 pacotes verdes
conferidos por fora. Total US$ 24,34 em 596 segundos.

Tres coisas que a execucao ensinou, todas contra previsoes minhas:

**A previsao de qual veto dispararia estava errada.** Eu disse que o #836
parava por teto de custo. Com o teto levantado, quem disparou foi
`sem_escrita`, aos 13 turnos e US$ 2,33 — bem longe do teto. O teto anterior
so era o limite binding porque cortava antes.

**A conta de tokens por tarefa do benchmark nao prevê um laco agentico.** O
roster diz que o `fable-5-1` termina uma tarefa com ~48k tokens. Esta gastou
**2,03 milhoes de tokens de entrada** — 42x — porque o contexto e reenviado
inteiro a cada turno e cresce. O numero do benchmark serve para ORDENAR
modelos entre si (§16.1); nao serve para prever o custo de um run, e o
desempate que ele alimenta continua sem validacao no nosso harness.

**Por turno, o modelo escalado foi o mais caro dos dois**: 72,5k tokens de
entrada por turno contra 34k do opus. O que justifica a escalada nao e
eficiencia por turno — e ter terminado, contra um que nao terminou.

E a conferencia de veracidade fez o que devia sem exagerar: marcou
`go test ./...` como **inconclusiva a 0,72**, abaixo do corte de 0,8. Nao
acusou nem absolveu.

## A rota do fable, conferida com o custo medido

A rota desempata por tokens por tarefa, derivados do benchmark de terceiro
(§16.1 do spec). Catorze execuções depois, dá para conferir a derivação
contra o que realmente aconteceu. `evals/issues-reais/medir-tokens.py`
reproduz a conta a partir dos ledgers.

**A premissa estava errada, e a medição a corrigiu.** A derivação supunha
que o custo se divide meio a meio entre entrada e saída. Medido sobre
7.142.353 tokens de entrada contra 93.556 de saída: **a saída é 1,31% da
entrada**. O contexto é reenviado inteiro a cada turno; a resposta de cada
turno é uma chamada de ferramenta curta. A conta passou a usar a proporção
medida, e isso trocou `deepseek` e `opus` de lugar na ordem — são os dois
únicos do roster com proporção de preço diferente. O deepseek nunca rodou
aqui, então essa parte da ordem segue sem validação.

**O valor absoluto não serve para prever o custo de um run.**

| modelo | previsto | medido (só onde terminou verde) |
|---|---:|---|
| `opus-5(low)` | 85k | ~250k — n=5, de 50k a 320k |
| `claude-fable-5-1` | 138k | 517k a 2.064k — n=2 |

Erra de 3x a 15x, e o erro **não é fator constante**: erra mais no fable que
no opus. Qualquer leitura do número como estimativa de conta está errada.

**A ordem, que é o que a rota usa, se sustentou.** Há exatamente um confronto
direto: a issue #685, em que os dois modelos terminaram verdes na **mesma
tarefa**.

| | tokens | US$ | turnos |
|---|---:|---:|---:|
| `opus-5(low)` | 249.846 | 1,32 | 11 |
| `claude-fable-5-1` | 517.138 | 5,43 | 12 |

Razão prevista pelo roster **1,62x**; razão medida **2,07x**. Mesma direção,
magnitude próxima. A escolha da rota — opus primeiro — estava certa nas duas
moedas: menos tokens e quatro vezes mais barato, para o mesmo resultado
verde.

Isso também esclarece o que a escalada é e o que não é. O fable não foi
chamado por ser mais eficiente: ele é o **menos** eficiente dos dois, por
turno e no total. Foi chamado porque o opus não terminou.

**A ressalva que sobra:** n=1 no confronto direto, e as duas tarefas que o
fable resolveu são justamente as que o opus não resolveu. A cascata só lhe
manda o que é difícil, então a média dele é de tarefas difíceis por
construção. A média por modelo neste eval não é comparável; só o par da #685
é.

## O que continua sem prova

A cascata escalou **uma vez**. Uma amostra não diz com que frequência escalar
compensa, nem se o degrau de 0,25 é o certo, nem o que acontece quando o
modelo escalado também falha — `MaxEscaladas: 1` manda o caso para o humano e
isso nunca foi exercido em trabalho real.

**Quanto custa uma escalada continua sem previsao.** As duas que rodaram
custaram US$ 7,05 e US$ 24,34 — um fator de 3,5 entre elas, e nenhum numero
do roster antecipava nem a ordem de grandeza. Enquanto isso, o teto e um
palpite do operador, e `--teto-usd` existe para que ele seja um palpite
consciente.

Fica tambem sem medida o que acontece quando o modelo escalado **tambem**
falha: `MaxEscaladas: 1` manda o caso para o humano com os dois diffs, e
esse caminho nunca rodou em trabalho real.

## Harness

`/tmp/swebench/`: `montar.sh` (repositórios no commit pai), `prep.py`
(tarefa, evidência, triagem), `rodar.sh` (plan + run), `conferir.sh`
(comparação com o mantenedor), `casos.tsv`, `termos.tsv`, `cmds.tsv`.
