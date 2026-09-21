---
name: delegador-runtime
description: Contrato interno para chamar o binario do delegador a partir do Claude Code — o laco, a permissao em codigo e os exit codes
user-invocable: false
---

# Runtime do delegador

Use apenas dentro do subagente `delegador-rescue`.

O binario e sempre chamado pelo wrapper:

`"${CLAUDE_PLUGIN_ROOT}/scripts/delegador" <subcomando>`

Subcomandos: `plan`, `run`, `status`, `result`, `roster`, `doctor`.

Como o laco funciona:

- A permissao e uma lista em codigo, avaliada antes de cada tool call:
  escrita so dentro da worktree do job; comandos so os declarados no
  briefing; e negacoes duras que nenhuma configuracao sobrepoe — push,
  commit, `rm -rf`, acesso a rede, credencial em argumento. As negacoes
  olham o argv: nao cobrem o que um comando PERMITIDO executa — codigo de
  teste rodando dentro de `go test`, Makefile ou script tem rede e fs
  livres; a fronteira real e a worktree, a allowlist e o ambiente sem
  chaves que o filho recebe.
- Recusa nao mata o laco: volta ao modelo como resultado de ferramenta, e
  ele tenta outro caminho em vez de morrer no meio.
- O watchdog e pre-condicao, nao vigia: antes de cada turno decide se ha
  proximo turno. Um veto grava motivo, trecho e comando de retomada no job.
- A verificacao e codigo, sem modelo: diff, teste, sonda de mutacao, suite
  e lint.
- Verificacao vermelha escala uma vez, para um modelo mais forte com o
  diff e a falha como evidencia. Vermelha de novo, o caso vai ao humano
  com os dois diffs.

Codigos de saida que mudam o seu comportamento:

- `plan` exit 3 — gate reprovou; a saida nomeia o que faltou.
- exit 1 — erro real ou verificacao vermelha. Leia o stderr e o
  `result.txt`.
- exit 2 — invocacao invalida: flag errada ou subcomando desconhecido.

Variaveis de ambiente:

- `TYPESAFE_API_KEY` — obrigatoria: sem ela gates, watchdog e compactacao
  nao rodam.
- `DELEGADOR_BASE_URL` — proxy do executor (padrao
  `http://127.0.0.1:8317/v1`).
- `DELEGADOR_API_KEY` — so se o proxy pedir chave.
- `DELEGADOR_ROSTER` — roster alternativo; a flag `--roster` tem
  precedencia.

Nunca sugira modos de execucao que aprovem operacoes irreversiveis ou
contornem a lista em codigo: a permissao do `delegador` existe porque juiz
externo de permissao nao e deterministico — foi a medida que gerou o v2.
