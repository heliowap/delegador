---
description: Mostra o estado de um job — estado, modelo, veredito verificado, custos separados e motivo de cancelamento
argument-hint: "<job-id>"
allowed-tools: Bash(*/scripts/delegador:*)
---

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/delegador" status --job $ARGUMENTS
```

Se o job aparecer como cancelado, leia o sinal e o trecho para o usuario e
ofereca o comando de retomada que vem na saida. Nao retome por conta
propria.
