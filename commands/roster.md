---
description: Lista os modelos do roster com elegibilidade e idade da sondagem; re-sonda com --probe
argument-hint: "[--probe <modelo-id>]"
allowed-tools: Bash(*/scripts/delegador:*)
---

Lista os modelos com status de elegibilidade, motivo de cada exclusao e
idade da sondagem:

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/delegador" roster
```

Re-sonda um modelo e grava o resultado no arquivo do roster:

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/delegador" roster --probe <id>
```

Sem id, `--probe` re-sonda todos os habilitados com sondagem vencida. A
sondagem vencida tira o modelo da rota ate ser remediada — e o proprio
arquivo do roster recebe a data nova, entao aponte `--roster` quando nao
for o padrao.
