---
name: delegador-briefing
description: Como reunir a evidencia que vira briefing do executor, e o que nunca delegar
---

# Evidencia para o briefing

Voce nao escreve o briefing: voce entrega o defeito em uma frase e a
evidencia verbatim. O `plan` seleciona e monta.

Reuna, verbatim, da sua propria sessao:

1. **Trecho com `arquivo:linha`** — onde esta o defeito. Sem isto, o gate
   reprova, porque o executor segue a fonte quando voce a nomeia e
   improvisa quando nao nomeia.
2. **Fonte do contrato** — o ADR, spec, issue ou comentario normativo que
   diz qual e o comportamento certo, com o texto do que ele diz.
3. **Erro observado** — a mensagem exata, nao a sua parafrase dela.
4. **Comandos e saidas** — o que voce rodou e o que apareceu.

Passe tambem os comandos de teste, de suite e de lint, copiaveis
(`--test-cmd`, `--suite-cmd`, `--lint-cmd`), e o que a worktree nova
precisa para rodar (`--venv`, `--node-modules`): sem saber rodar teste o
executor inventa ou nao roda. Os comandos declarados viram a allowlist de
exec do laco — o que nao estiver la sera negado ao modelo.

Nao delegue: decisao de desenho em aberto, mudanca que cruza pacotes,
qualquer coisa que toque credencial, segredo, dado pessoal ou deploy, nem
tres temas diferentes no mesmo pedido. Dois defeitos que sao a mesma causa
funcionam; tres assuntos, nao.

Tarefa autocontida grande cabe num modelo barato; tarefa pequena que exige
decisao no caminho, nao. Se a sua tarefa exige que o executor decida o que
fazer — e nao transcreva o que ja esta decidido — espere escalada.

Quando o trabalho for do executor, diga isso no relatorio ou no commit, e
diga o que voce conferiu. Uma linha honesta e barata.
