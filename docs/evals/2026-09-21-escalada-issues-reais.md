# Escalada contra issues reais — expr-lang/expr

**2026-09-21.** Seis bugs fechados de um repositório de terceiro, cada um no
commit anterior à correção, com o teste do mantenedor já no disco.

## Veredito

**A cascata não escalou nenhuma vez, e o motivo não é o que eu supunha.**

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

## O que continua sem prova

A cascata **nunca escalou em trabalho real**. O mecanismo está coberto por
teste unitário (`TestEscalaComVerificacaoVermelha`,
`TestNaoEscalaPorVetoDeCusto`, `TestEscalaQuandoMutacaoNaoProvaNada`) e o
re-roteio exclui o modelo que falhou, então não repete. Mas em oito execuções
contra bugs reais ele não teve a oportunidade de rodar.

O experimento que faltou, e seu preço: **repetir o caso 4 com o teto em US$
15**. É o único run que chegou perto — entregou um diff de quatro arquivos com
suíte e lint vermelhos, e parou por orçamento a caminho do `final`. Com teto
suficiente ele termina, a verificação reprova, e a cascata escala para
`claude-fable-5-1`. Custo estimado: US$ 8 a 15.

## Harness

`/tmp/swebench/`: `montar.sh` (repositórios no commit pai), `prep.py`
(tarefa, evidência, triagem), `rodar.sh` (plan + run), `conferir.sh`
(comparação com o mantenedor), `casos.tsv`, `termos.tsv`, `cmds.tsv`.
