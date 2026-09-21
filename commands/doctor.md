---
description: Confere o ambiente antes de despachar — proxy, chave, roster carregavel e sondagens em dia
allowed-tools: Bash(*/scripts/delegador:*)
---

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/delegador" doctor
```

Confere e falha com a causa, em vez de deixar um job morrer no meio: proxy
alcancavel em `DELEGADOR_BASE_URL` (ou o padrao `http://127.0.0.1:8317/v1`),
`TYPESAFE_API_KEY` presente, roster carregavel e quantos modelos estao
elegiveis — com o motivo de cada exclusao.

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/delegador" doctor --probe
```

`--probe` re-mede os modelos com sondagem vencida e grava o resultado no
arquivo do roster.
