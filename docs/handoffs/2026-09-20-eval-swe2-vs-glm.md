# Handoff — eval: `swe-2-max` contra `glm-5-3-flash-max`

Data: 2026-09-20 · Origem: sessão de desenho do `devin-plugin-cc`
Destino: sessão nova, sem contexto desta. Tudo que você precisa está aqui.

## A pergunta

O roster em [`config/roster.yaml`](../../config/roster.yaml) classifica `devin/swe-2`
e `cpa-fw-glm-5.3-flash` ambos como `barato`, e essa classificação veio de
benchmark de terceiro, de blog de fornecedor e de observação solta — nunca de
um confronto controlado. **Qual dos dois executa melhor as tarefas deste
próprio plano, medido por código e não por opinião?**

Resultado esperado: um número por métrica, por modelo, e uma recomendação de
qual deve ser o padrão da cascata. Não é para implementar o plugin — é para
medir os dois executores.

## Estado verificado (confira antes de confiar)

```bash
cd ~/VSCode/devin-plugin-cc      # docs, spec, plano, roster, eval
cd ~/VSCode/devin-plugin-cc-impl # implementação; branch impl/plano-inicial
```

- `devin-plugin-cc-impl` está em `98bce8e`, tarefas 1 a 8 do plano feitas,
  `go vet ./...` e `go test ./...` verdes. **Este é o ponto de partida dos dois
  braços.** Confirme com `git log --oneline -1` e rode a suíte antes de começar.
- Tarefas 9 a 18 **não** foram executadas.
- `devin` 3000.10.31 em `~/.local/bin/devin`, autenticado.
- Os dois modelos existem no CLI: confirme com
  `devin models list | grep -E 'swe-2-max|glm-5-3-flash-max'`.

## Por que o eval tem dois braços

Medição feita nesta sessão: **16 das 18 tarefas do plano são 72% a 91% código
Go literal.** Executá-las mede fidelidade de transcrição, não capacidade —
nas tarefas 1 a 8, 98% das 1130 linhas commitadas estavam escritas no plano.
Só as tarefas 17 (24%) e 18 (30%) exigem montagem própria.

Por isso: um braço mede **operação**, outro mede **capacidade**.

## Braço A — operação (tarefas 9 a 18, como estão)

Mede o que importa no uso real: quantas tarefas fecham sozinhas, quantas
travam, quanto custa, quanto demora.

```bash
# worktrees irmãs, mesma base, uma por modelo
cd ~/VSCode/devin-plugin-cc
git -C ../devin-plugin-cc-impl worktree add -b eval/swe2 ~/VSCode/eval-swe2 98bce8e
git -C ../devin-plugin-cc-impl worktree add -b eval/glm  ~/VSCode/eval-glm  98bce8e

# um modelo por vez; rodar em paralelo distorce latência e quota
MODEL=swe-2-max        WT=~/VSCode/eval-swe2 RUN=/tmp/eval-swe2 \
  PLAN=~/VSCode/devin-plugin-cc/docs/superpowers/plans/2026-09-20-devin-plugin-cc.md \
  START=9 END=18 bash eval/run-plan.sh

MODEL=glm-5-3-flash-max WT=~/VSCode/eval-glm  RUN=/tmp/eval-glm \
  PLAN=~/VSCode/devin-plugin-cc/docs/superpowers/plans/2026-09-20-devin-plugin-cc.md \
  START=9 END=18 bash eval/run-plan.sh
```

Crie os diretórios de run antes (`mkdir -p /tmp/eval-swe2/{logs,prompts,exports}`).
O driver para na primeira tarefa que ficar vermelha — isso é intencional. Se
parar, registre onde e siga para o outro modelo; **não conserte a tarefa para
o modelo**, isso contamina a medição.

## Braço B — capacidade (tarefas 12, 13 e 14, sem a implementação)

Estas três são as conceitualmente mais difíceis: política do watchdog, teste
de mutação, e compactação por deleção. `eval/strip-task.py` remove os blocos
de implementação e **preserva os testes e o bloco `Interfaces:`**. O modelo
tem que autorar o código; os testes do plano são o juiz, e são objetivos.

```bash
mkdir -p /tmp/eval-nua
for n in 12 13 14; do
  python3 eval/strip-task.py \
    docs/superpowers/plans/2026-09-20-devin-plugin-cc.md $n > /tmp/eval-nua/task-$n.md
done
```

Rode cada tarefa nua em worktree própria a partir de `98bce8e`, usando o mesmo
prompt-molde do driver (a seção "Regras desta execução" dele). Critério de
aprovação, sem subjetividade: `go test ./...` verde **com os testes originais
do plano, não modificados**. Se o modelo alterar um teste, isso é reprovação —
confira com `git diff 98bce8e -- '*_test.go'`.

## O que registrar, por modelo e por tarefa

| Métrica | Onde sai |
| --- | --- |
| tarefa fechou sozinha | `RUN/progress.log`, linha `VERDE` |
| travou, e em quê | `RUN/progress.log` (`RECUSA`, `TRAVOU_DUAS_VEZES`, `VERMELHO`) e o fim de `RUN/logs/task-N.log` |
| precisou de retomada | existe `RUN/logs/task-N.log.retry` |
| tempo por tarefa | diferença entre carimbos do `progress.log` |
| linhas autorais | `git diff --stat` da tarefa contra o que o plano entregava |
| custo | quota do Devin antes/depois (`/usage` na sessão interativa) |

Para o braço B, some: **passou nos testes originais** (sim/não) e **alterou
teste** (sim/não, reprovação automática).

## Armadilhas já medidas — não redescubra

- **`--permission-mode smart` não é determinístico.** Ele recusou `chmod`,
  `git commit` e, uma vez, `go test` — o mesmo `go test` que aprovara dezenas
  de vezes antes. Quatro recusas em nove tarefas. Isso afeta os dois modelos
  igualmente, então não invalida a comparação, mas **conte as recusas como
  métrica** em vez de tratá-las como ruído.
- **Não escale para `dangerous` para destravar.** Ele aprovaria `push`.
- **O driver é dono dos commits**, não o modelo. O prompt proíbe
  `git commit`, `git add`, `git push` e `chmod`; o driver commita depois do
  portão verde. Não desfaça isso.
- **O `-p` grava o stdout incrementalmente**, mas o `--export` só no fim.
  Para acompanhar ao vivo, siga `RUN/logs/task-N.log`, não o export.
- **Custo assimétrico:** `swe-2-*` está gratuito por promoção de setembro/2026
  e não consome quota; `glm-5-3-flash-max` custa $0,15/M entrada e $0,50/M
  saída e consome quota Pro. Registre, mas não use isso para escolher — o
  gratuito acaba.

## Limites

- Não faça `push`, não abra PR, não toque em `main` de nenhum dos repos.
- Não altere `docs/`, `config/roster.yaml` nem o plano durante o eval.
  Mexer no instrumento durante a medição invalida a medição.
- As worktrees `eval/*` são descartáveis. A `devin-plugin-cc-impl` **não é** —
  ela tem o trabalho real e fica intocada.
- Se um modelo travar duas vezes na mesma tarefa, pare esse braço e registre.
  Não assuma a tarefa você: quem executa é o modelo sob teste.

## Entrega

Feche com o formato de closeout da skill `handoff-executor`: tabela de
Execution Summary, Remaining Work, Notes. Além dela, entregue:

1. **Tabela comparativa** — por tarefa e por modelo: fechou, travou em quê,
   retomadas, tempo.
2. **Resultado do braço B** — quantas das três o modelo autorou com os testes
   originais passando.
3. **Recomendação** de qual vira padrão da cascata, com o número que a
   sustenta. Se os dois empatarem dentro do ruído, **diga que empataram** —
   empate é resultado, não falha do eval.
4. **Patch para `config/roster.yaml`**: atualizar `papel` e acrescentar um
   bloco `eval:` com data e números. Não commite; deixe o diff para revisão.

Se a medição não sustentar uma conclusão, diga isso. Este projeto inteiro
existe porque relatório não é prova.

## Depois do eval

O projeto original retoma em duas frentes já decididas nesta sessão:
reescrever o spec com o companion sendo o laço de agente (acesso direto à API
pelo proxy local em `http://127.0.0.1:8317/v1`, com tool calling confirmado),
e o roteador por dimensão usando os benchmarks do OpenRouter. Nenhuma das
duas depende deste eval — ele só resolve qual modelo fica no papel de padrão.
