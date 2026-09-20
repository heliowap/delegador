---
description: Imprime o relatorio verificado do job — veredito, trace compactado e a linha de custo
argument-hint: "<job-id>"
allowed-tools: Bash(*/scripts/delegador:*)
---

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/delegador" result --job $ARGUMENTS
```

Devolva a saida como esta. Se houver um bloco DIVERGENCIA, leia-o em voz
alta para o usuario antes de qualquer resumo: ele indica que o relatorio do
modelo nao bate com o que a verificacao mediu. Sem `result.txt` o
subcomando explica a ausencia — job que ainda nao rodou nao tem relatorio.
