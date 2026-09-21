---
description: Executa o job planejado no laco proprio — permissao em codigo, verificacao e cascata
argument-hint: "<job-id>"
allowed-tools: Bash(*/scripts/delegador:*)
---

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/delegador" run --job $ARGUMENTS
```

O run executa o laco inteiro de uma vez: chama o modelo, aplica a permissao
em codigo a cada tool call (recusa vira resultado de ferramenta, nao mata o
laco), verifica em codigo (diff, teste, sonda de mutacao, suite, lint) e
escala uma vez se a verificacao reprovar. No fim grava `result.txt` — leia
com `result`.

Exit 0 significa verde e verificado. Exit 1 significa falha: leia o
`result.txt` e o `status` antes de decidir entre retomar ou replanejar.
