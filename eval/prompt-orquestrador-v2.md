# Executar o plano v2 do delegador

Você é o orquestrador. Invoque `/superpowers:subagent-driven-development` e
execute o plano abaixo despachando um subagente por tarefa, com revisão entre
elas. Não pause entre tarefas para perguntar se deve continuar — o plano é a
autorização. Registre rulings no ledger e siga.

## Onde

```
worktree:  ~/VSCode/delegador-v2        (branch v2/base)
plano:     ~/VSCode/delegador/docs/superpowers/plans/2026-09-20-delegador-v2.md
spec:      ~/VSCode/delegador/docs/superpowers/specs/2026-09-20-delegador-v2-design.md
roster:    ~/VSCode/delegador/config/roster.yaml
```

Trabalhe **apenas** em `~/VSCode/delegador-v2`. As worktrees `delegador-impl`,
`eval-swe2` e `eval-glm` são de outras sessões: não as toque, nem para ler
resultado.

## O que já está feito — não refaça

A **Task 1** e a **Task 3** estão implementadas e commitadas (`26f677d`,
`e0731c2`). Comece pela **Task 2** e siga: **2, 4, 5, 6, 7, 8, 9, 10, 11, 12,
13, 14, 15**.

Dois desvios do plano já aplicados na Task 1, para você não estranhar:

1. `internal/cli/doctor.go` **não** foi removido. Ele não depende do que foi
   podado, e removê-lo quebrava `TestRunUnknownSubcommandExits2`, que asserta
   a listagem de subcomandos. A Task 15 o reescreve.
2. A Task 3 ganhou `internal/tools/allow_adversarial_test.go`, com nove testes
   além dos onze do plano. Eles acharam dois furos reais — bypass de negação
   dura por caminho absoluto (`/usr/bin/curl`) e prefixo de escrita casando
   diretório irmão (`pkg/svc` autorizando `pkg/svcX`). **Esses testes são
   oráculo como qualquer outro.**

Confirme a base antes de começar:

```bash
cd ~/VSCode/delegador-v2 && go vet ./... && go test ./...
```

Esperado: verde, com `internal/cli`, `internal/jev`, `internal/job` e
`internal/tools` passando.

## As regras que não admitem julgamento

**Não altere teste.** O plano v2 entrega assinatura e teste; o corpo da
implementação é seu. Se um teste parecer errado, registre um ruling dizendo
por que e implemente para ele mesmo assim — ou pare, se for impossível. Nunca
adapte o oráculo ao código. Antes de cada commit:

```bash
git diff --stat -- '*_test.go'
```

Teste modificado que você não criou é reprovação da tarefa.

**Não faça `git push`.** Commits são bem-vindos, um por tarefa, com mensagem
dizendo o que a tarefa entregou.

**Não altere `docs/`, `config/roster.yaml` nem o plano.** Se achar defeito no
plano — e vai achar, ele tem —, registre o ruling e siga; quem corrige o
documento sou eu, depois.

**Zero dependências.** `go.mod` não ganha bloco `require`. Se parecer que
precisa de uma, o ruling é escrever as 80 linhas à mão ou mudar o formato do
dado de entrada. Isso vale especialmente para o YAML da Task 8.

## Armadilhas já medidas nesta máquina

- **`internal/jev` e `internal/job` estão verdes e são reaproveitados do v1.**
  Leia antes de reescrever: `jev.StateBudget`, `jev.Ledger` e `job.Create` já
  fazem o que as tarefas 7, 13 e 14 precisam.
- **A Task 2 é pré-requisito de tudo.** Sem o servidor falso, cada `go test`
  vira token gasto e dependência de rede. Não pule, não simplifique.
- **`tools.Allow` já existe e é a única porta entre o modelo e o disco.** A
  Task 4 chama `Allow` antes de qualquer execução, e negação vira
  `Result{IsError: true}` com o motivo — **nunca** erro de Go, nunca panic.
  Essa é a correção central do v2; se você fizer a negação matar o laço,
  reconstruiu o problema que o v2 existe para resolver.
- **O laço não passa comando por `sh -c`.** A allowlist já garantiu que não há
  metacaractere; passar por shell reabriria o que a Task 3 fechou.
- **O endpoint real é `http://127.0.0.1:8317/v1`**, formato OpenAI, chave em
  `~/VSCode/Lab/CLIProxyAPI/config.yaml` sob `api-keys:`. Você **não precisa**
  dele para nenhuma tarefa: tudo é testável contra o servidor falso da Task 2.
  Não gaste token com o endpoint real.

## Checkpoints

Três tarefas carregam o v2. Se uma delas ficar vermelha, pare e reporte em vez
de contornar:

- **Task 6**, `TestRunContinuesAfterDeniedCall` — recusa não mata o laço.
- **Task 12**, `TestRunFlagsTestThatProvesNothing` — a mutação pega teste que
  passa com a correção desfeita. Sustenta a cascata inteira.
- **Task 11**, `TestNaoEscalaPorVetoDeCusto` — recusa e teto de custo não são
  falha do modelo, e escalar neles é pagar caro por erro de ambiente.

## Ao terminar

Relatório com: tarefas concluídas e commits de cada uma; a saída de
`go vet ./...` e `go test ./...` no fim; os rulings que você registrou, com o
custo de cada um estar errado; e o que ficou por fazer, se algo ficou.

Se parar antes do fim, diga em qual tarefa, o que já estava commitado, e o que
exatamente bloqueou.
