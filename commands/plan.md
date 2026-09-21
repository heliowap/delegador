---
description: Monta o briefing da tarefa e roda os gates de delegabilidade; devolve job id e modelo escolhido
argument-hint: "<o defeito em uma frase>"
allowed-tools: Bash(*/scripts/delegador:*)
---

Monte o JSONL de evidencias a partir do que voce **ja leu nesta sessao**:
trechos com `arquivo:linha`, a mensagem de erro exata, os comandos que voce
rodou e a saida deles, e a citacao do ADR ou spec que define o contrato
violado. Um objeto por linha:

```
{"kind":"trecho","ref":"pkg/svc/rota.go:42","text":"<o codigo, verbatim>"}
{"kind":"erro","text":"<a mensagem exata>"}
{"kind":"comando","text":"<comando e saida>"}
{"kind":"fonte","ref":"docs/adr/0007.md","text":"<o que a fonte diz>"}
```

Nao resuma nada nesse arquivo: o gate seleciona o que entra, e resumo perde
justamente o caminho e o erro que importam. Nao invente evidencia que voce
nao leu.

Grave o JSONL num arquivo temporario e rode:

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/delegador" plan \
  --task "$ARGUMENTS" \
  --evidence <arquivo.jsonl> \
  --worktree <worktree isolada> \
  --test-cmd "<comando de teste>" \
  --suite-cmd "<comando da suite>" \
  --lint-cmd "<comando de lint>" \
  --branch "<branch da worktree>"
```

Exit 3 significa gate reprovado: a saida nomeia o item que faltou. Corrija o
que faltou e rode de novo; nao tente contornar o gate. Exit 0 devolve o job
id e o modelo escolhido pela rota — prossiga com o `run`.
