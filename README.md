# delegador

Delega uma tarefa de código a um modelo escolhido **pela tarefa**, executa num
laço próprio, verifica o resultado com código, e escala para um modelo mais
forte só quando a verificação reprova.

[Jev](https://docs.typesafe.ai), o modelo System One da TypeSafe, entra onde
é preciso julgar significado. Código entra em todo o resto — inclusive na
permissão, que é o ponto onde delegação costuma quebrar.

## Por que existe

Delegar custa caro em dois recursos: tempo de parede, quando um run condenado
ocupa quarenta minutos antes de alguém perceber; e token do modelo caro,
quando ele lê um relatório inteiro para descobrir se "verde" era verdade.

O `swe-2` executou oito tarefas do plano v1 neste repositório. Quatro vezes
ele morreu no meio porque um juiz de permissão externo recusou `chmod`,
`git commit` e — uma vez, sem mudar nada — o mesmo `go test` que aprovara
dezenas de vezes. Nenhuma dessas falhas era do modelo.

## Como decide

O Jev diz **qual dimensão** a tarefa estressa — mecânica, raciocínio ou
agêntica — e quão autocontido é o briefing. O código faz a aritmética sobre os
benchmarks e o roster, e escolhe pelo **custo por tarefa**, não por preço por
token. Então o barato executa, a verificação em código julga (teste, mutação,
suíte, lint), e o forte só entra por falha provada.

A diferença que isso compra: `glm-5.3-flash` faz 0,758 no tau-bench a
**$0,0061 por tarefa**; `opus-5` faz 0,792 a **$0,493**.

## Documentos

- [Design v2](docs/superpowers/specs/2026-09-20-delegador-v2-design.md) — atual
- [Design v1](docs/superpowers/specs/2026-09-20-devin-plugin-cc-design.md) — supersedido; embrulhava o `devin` CLI
- [Plano v1](docs/superpowers/plans/2026-09-20-devin-plugin-cc.md) — tarefas 1 a 8 implementadas e verdes
- [Roster](config/roster.yaml) — modelos habilitados, com origem de cada dado
- [Handoff do eval](docs/handoffs/2026-09-20-eval-swe2-vs-glm.md)

## Estado

Tarefas 1 a 8 do plano v1 implementadas em `impl/plano-inicial`. Sobrevivem ao
v2 `internal/jev` e `internal/job`, cerca de 700 das 1130 linhas — a camada de
julgamento. O module path Go ainda é `devin-plugin-cc` e muda no v2.
