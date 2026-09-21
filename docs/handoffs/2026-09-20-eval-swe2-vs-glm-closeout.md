# Closeout — eval `swe-2-max` × `glm-5-3-flash-max`

Data: 2026-09-20 · Executado a partir de [`2026-09-20-eval-swe2-vs-glm.md`](2026-09-20-eval-swe2-vs-glm.md)

## Veredito de uma linha

**A medição não sustenta uma recomendação.** O `glm-5-3-flash-max` não
executou uma única tarefa — quota diária da conta esgotada — então não houve
confronto. O que existe é meia medição, do `swe-2-max` sozinho, e mesmo ela
ficou truncada por dois defeitos de instrumento que o eval descobriu sobre si
mesmo.

## Execution Summary

| Item | Estado | Evidência |
| --- | --- | --- |
| Pré-condições (base `98bce8e`, suíte verde, dois modelos no CLI) | ✅ conferido | `go vet`/`go test` verdes em 5 pacotes |
| Braço A — `swe-2-max` tarefas 9–18 | ⚠️ parcial: 3 verdes, parou na 12 | `/tmp/eval-swe2/progress.log` |
| Braço A — `glm-5-3-flash-max` tarefas 9–18 | ❌ não executou | `resource_exhausted` em 3s |
| Braço B — `swe-2-max` tarefas 12/13/14 nuas | ⚠️ 0/3, todas mortas por recusa | `/tmp/eval-nua/judge-swe2-*.log` |
| Braço B — `glm-5-3-flash-max` | ❌ não executou | mesmo bloqueio de quota |
| Recomendação de padrão da cascata | ❌ **não emitida**, por falta de dado | — |

### Braço A — `swe-2-max` (operação)

| tarefa | veredito | tempo | linhas | passos | tok in/out | recusa real |
| --- | --- | --- | --- | --- | --- | --- |
| 9 | VERDE | 6m16s | 957 | 34 | 1.494.786 / 19.771 | não |
| 10 | VERDE | 9m03s | 506 | 42 | 1.974.876 / 24.348 | não |
| 11 | VERDE | 2m32s | 230 | 17 | 286.448 / 5.669 | não¹ |
| 12 (1ª) | abortada | 5m55s | — | — | — | não² |
| 12 (2ª) | TRAVOU_DUAS_VEZES | 15s + retomada | — | — | — | **sim, 2×** |
| 13–18 | não alcançadas | — | — | — | — | — |

¹ o driver marcou `RECUSA`, mas é falso positivo — ver Defeito 3.
² cortada pelo teto de taxa do tier gratuito, não por falha.

Custo: **$0,00** — `swe-2-*` está gratuito por promoção.

### Braço A — `glm-5-3-flash-max`

Zero tarefas. Falha na primeira invocação, em 3 segundos:

```
Error: Agent error: Your daily usage quota has been exhausted.
("cognition.ai/errorKind": "resource_exhausted")
```

### Braço B — `swe-2-max` (capacidade)

| tarefa nua | passou | alterou teste | tempo | passos | tok in/out | causa da parada |
| --- | --- | --- | --- | --- | --- | --- |
| 12 | não | **não** (arquivos ausentes) | 8m51s | 27 | 1.237.496 / 24.714 | recusa de permissão |
| 13 | não | **não** (teste escrito verbatim) | 9m55s | 20 | 457.932 / 28.917 | recusa no `go test` do Step 2 |
| 14 | não | **não** (arquivos ausentes) | 4m23s | 18 | 409.613 / 13.427 | recusa de permissão |

**Zero alterações de teste em três tarefas.** Nenhuma das três chegou a ser
uma medição de capacidade: as três morreram em recusa de permissão, e o
`run-nua.sh` — diferente do `run-plan.sh` — não tinha retomada.

## Os quatro defeitos que a medição encontrou

### 1. Quota, não capacidade, decidiu quem rodou

Os dois modelos foram barrados por infraestrutura, de formas diferentes:
`swe-2-max` pelo teto de taxa do tier gratuito (reversível, janela crescente —
11 min, depois 27 min), `glm-5-3-flash-max` pela quota diária Pro (irreversível
hoje sem compra de uso avulso). O custo assimétrico que o handoff mandou
"registrar mas não usar para escolher" acabou **escolhendo sozinho** quem foi
medido.

### 2. O isolamento do braço B é falso

Worktrees do git compartilham o banco de objetos. De dentro de
`eval-nua-swe2-12` o modelo leu as outras branches e disse, no próprio log:

> There are newer commits on other branches (`c34b740`). **The `eval/swe2`
> branch has tasks 9–11 done** and there's a corrected-plan commit on main.

Pior: o plano completo, com todos os blocos de implementação, é alcançável por
`git show main:docs/superpowers/plans/...`. O `strip-task.py` tira a
implementação do prompt e o git a devolve inteira. **O braço B nunca foi cego.**

Correção necessária: clone isolado (`git clone --depth 1 --single-branch`) em
vez de worktree.

### 3. O detector de recusa lê a prosa do modelo

`run-plan.sh` decide que houve recusa com `grep -q "rejected a tool call"` no
stdout. A tarefa 11 implementa `watchdog.PermissionRejected`, o sinal que
procura esse mesmo marcador. O modelo descreveu a implementação no relatório e
o driver leu isso como evento de permissão, disparando uma retomada inútil de
36s numa tarefa que já estava verde.

Correção necessária: ler o canal de evento do `devin`, não o stdout misturado
com relatório. Qualquer tarefa que *fale* sobre permissão envenena a métrica.

### 4. `--permission-mode smart` é o maior destruidor de medição do conjunto

**Cinco recusas reais em nove invocações registradas.** Todas no mesmo padrão:
o `go test` do "Step 2 — confirme o vermelho", o mesmo comando aprovado dezenas
de vezes antes. A tarefa 12 do braço A rodou 5m55s sem recusa na primeira
tentativa e morreu em 15s na segunda, com prompt e base idênticos.

Isso atinge os dois modelos igualmente, então não invalida a comparação — mas
com metade das invocações morrendo por sorteio, a comparação precisaria de
muito mais repetições para separar sinal de ruído.

## O que o `swe-2-max` mostrou, apesar de tudo

Dois achados que sobrevivem aos defeitos, porque são reprodutíveis:

**Diagnosticou uma incoerência não-óbvia do plano, três vezes, em sessões
independentes.** Na tarefa 12 (duas execuções) e na 14, percebeu sozinho que o
plano corrigido manda o watchdog ler `stdout.log` enquanto o `fakedevin` no
cenário `permission-block` só escreve no `export.json` — o teste e2e do Step 8
seria impossível sem tocar no fixture. Nas três vezes **anunciou o desvio antes
de fazê-lo**, em vez de alterar o fixture em silêncio.

**Identificou o mesmo defeito de isolamento que invalida o braço B**, antes de
mim:

> `cp -R` de uma worktree linkada copia o *arquivo* `.git`, então `git checkout`
> dentro da cópia escreveria na worktree original.

Nenhum dos dois aparece em benchmark de terceiro.

## Correção de fato ao `config/roster.yaml`

A nota atual do `devin/swe-2` afirma:

> executou 8 tarefas deste plano com fidelidade, mas 98% do código estava
> escrito no plano, e **ele morreu na tarefa 9**, a primeira que exigia montagem
> própria.

**Isso está errado.** Medido hoje: a tarefa 9 fechou verde em 6m16s, 957 linhas
em 13 arquivos, sem recusa. As tarefas 10 e 11 também. Ele parou na 12, e por
recusa de permissão, não por incapacidade.

## Remaining Work

1. **Braço A do `glm-5-3-flash-max`, tarefas 9–18** — bloqueado por quota.
   Destrava com reset diário ou uso on-demand em `app.devin.ai/settings/usage`.
   Decisão de compra é do humano; não foi tomada.
2. **Braço A do `swe-2-max`, tarefas 12–18** — parou na 12 por recusa dupla.
   Retomar exige ou mais tentativas sob `smart`, ou aceitar a recusa como
   métrica e medir só até onde dá.
3. **Braço B inteiro, os dois modelos** — precisa dos consertos 2 e 4 antes de
   qualquer nova rodada. Como o braço B não produziu **nenhuma** medição válida,
   consertá-lo agora não é mexer no instrumento durante a medição.
4. **Recomendação de padrão da cascata** — não emitida, e não deve ser emitida
   até existir confronto.

## Notes

- Nada foi commitado nos repos reais. `devin-plugin-cc-impl` segue intocada em
  `98bce8e`. Não houve `push`, PR, nem alteração em `main`.
- `run-plan.sh` e `strip-task.py` **não foram alterados** durante a medição. A
  espera por throttle foi implementada fora deles (`/tmp/eval-resume.sh`), para
  que o GLM rode exatamente o mesmo driver quando a quota voltar.
- Worktrees descartáveis criadas: `eval/swe2`, `eval/glm`, `eval/nua-swe2-{12,13,14}`.
- Artefatos: `/tmp/eval-swe2/`, `/tmp/eval-glm/`, `/tmp/eval-nua/`,
  `/tmp/eval-metrics.py` (extrator de métricas a partir dos artefatos).
- Este projeto existe porque relatório não é prova. O relatório honesto de hoje
  é: não deu para provar nada sobre o GLM, e o que deu para provar sobre o
  `swe-2-max` contradiz, num ponto específico, o que o roster afirma sobre ele.
