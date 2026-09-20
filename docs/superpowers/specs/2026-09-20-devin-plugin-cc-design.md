# devin-plugin-cc — design

Data: 2026-09-20
Status: aprovado para planejamento
Autor: Helio Pinheiro (design conduzido com Claude Opus 5)

## 1. Problema

Delegar uma tarefa ao `devin` hoje é caro nos dois recursos que importam: **tempo de parede** e **tokens do modelo orquestrador**.

O tempo se perde porque o Devin não avisa quando encalha. O log de `-p` só é escrito no fim, então um run que travou no minuto 3 continua ocupando a vaga até o minuto 40, quando alguém finalmente lê que ele parou de progredir. Em uso medido, com `--permission-mode smart`, uma tarefa passou e outra travou duas vezes no mesmo ponto.

Os tokens se perdem em três lugares. Na ida, o orquestrador **resume** o próprio contexto para montar o briefing, e resumo é lossy: caminho de arquivo, erro exato e comando somem justamente quando importam. Na volta, o relatório do Devin afirma "vermelho, correção, verde" com a mesma confiança quando rodou e quando não rodou, então alguém precisa ler tudo para saber qual dos dois é. E no modo revisor, achado de agente rende falso positivo com frequência — em medição própria, 6 de cerca de 30 alegações não se sustentaram, e cada uma foi lida por um modelo caro antes de ser descartada.

O material de referência que originou este projeto é o protocolo de delegação em `/tmp/devin-como-delegar.md`, medido em uso real. Este plugin transforma aquele protocolo, hoje aplicado à mão, em contrato executável.

## 2. Objetivo

Um plugin de Claude Code que use o `devin` como subagente, com **Jev (TypeSafe System One)** como classificador barato em cinco pontos do processo, de forma que:

- tarefa mal formada não chegue a ser despachada;
- run condenado seja morto em minutos, não em dezenas de minutos;
- o briefing carregue evidência **verbatim** em vez de resumo;
- o orquestrador leia um veredito verificado por código e um trace curto, não o relatório bruto.

O mesmo companion serve ao Codex por `AGENTS.md`, chamado por shell.

### Não-objetivos

- **Não** compactar a sessão do próprio Claude. Isso é o escopo de `tamaratran/fast-jev-compaction`, que convive com este plugin; aqui só se compacta o que entra e sai do Devin.
- **Não** substituir a verificação humana. O plugin executa teste, mutação, suíte e lint e mostra a saída; a decisão de aceitar continua sendo de quem lê.
- **Não** dar ao Devin permissão de commit, push ou rede além do que o modo de permissão escolhido já concede.

## 3. Decisões

| Decisão | Escolha | Razão |
| --- | --- | --- |
| Formato | Plugin de Claude Code, repo próprio `heliowap/devin-plugin-cc` | O watchdog precisa de processo de fundo; skill pura só existe dentro do turno do modelo |
| Linguagem | Go 1.27+, stdlib pura | O watchdog é daemon supervisionando grupo de processos; `context` + `Setpgid` expressam isso direto, e binário estático tira o runtime do caminho |
| Distribuição | `go build` sob demanda via `scripts/companion` | Público é o autor e quem ele indicar; troca "clone e funciona sem Go" por um supervisor sólido |
| Dependências | Nenhuma, incluindo o SDK do TypeSafe | A API do Jev é um POST; o SDK custaria toolchain npm num projeto Go |
| Modelo do Devin | Resolvido em runtime a partir de `devin models list` | `swe-2-*` está gratuito por promoção mensal; cravar modelo apodrece o plugin |
| Ação do watchdog | Cancela sozinho e avisa, com motivo verbatim e comando de retomada | Decisão do autor; cancelamento é reversível por `devin -c`, então o custo do falso positivo é baixo |
| Ordem de julgamento | Código primeiro, Jev depois | Repetição de comando, escopo de arquivo e exit code são determinísticos; Jev só lê o que exige semântica |

## 4. Arquitetura

```
devin-plugin-cc/
├── .claude-plugin/{plugin.json, marketplace.json}
├── commands/   rescue.md review.md status.md result.md cancel.md setup.md
├── agents/     devin-rescue.md
├── skills/     devin-runtime/ devin-briefing/ devin-verification/
├── hooks/hooks.json
├── cmd/devin-companion/main.go
├── internal/
│   ├── cli/      plan task status result wait-and-result cancel supervise doctor setup
│   ├── job/      estado em disco, lockfile, ciclo de vida
│   ├── devin/    models list → rota, montagem de flags, spawn, parser ATIF
│   ├── jev/      client questions gate route watchdog compact budget
│   ├── gitx/     worktree, diff, apply -R (mutação)
│   ├── verify/   executa teste/lint/mutação e captura stdout
│   └── render/   saída de terminal
├── testdata/   exports ATIF roteirizados, fixture de models list, fakedevin/
├── evals/      fixtures rotuladas das perguntas Jev
├── scripts/companion
├── AGENTS.md README.md
```

O Claude entra por `commands/` e pelo subagente forwarder `agents/devin-rescue.md`, que só faz chamadas `Bash` ao companion e devolve stdout inalterado. O Codex entra por `AGENTS.md`, chamando o mesmo binário. Nenhuma lógica vive em Markdown.

`hooks/hooks.json` e os comandos invocam `scripts/companion`, um wrapper `sh` que compila quando o binário falta ou é mais velho que o fonte e então dá `exec`. Sem Go instalado, o wrapper falha com mensagem dizendo isso e apontando `/devin:setup`.

## 5. Fluxo

### 5.1 `plan` — antes de qualquer dispatch

O orquestrador **não escreve o briefing em prosa**. Ele entrega o defeito em uma frase mais as evidências que já coletou, verbatim, num JSONL (`{"tipo":"trecho|erro|comando|fonte","ref":"arquivo:linha","texto":"..."}`).

1. Jev julga cada item de evidência (ponto 5 da ida, §6.5) e descarta o que não serve a **esta** tarefa.
2. O companion monta `briefing.md` a partir de um template fixo com os sobreviventes inalterados.
3. Jev roda o **gate de delegabilidade** (§6.1) sobre a tarefa e o **gate de briefing** (§6.2) sobre o arquivo montado — o artefato que o Devin vai ler, não a intenção.
4. Jev roda a **rota** (§6.4): modo de permissão e nível de esforço.

Saída: JSON `{delegavel, tipo, faltando:[...], rota:{modelo, permissao}, briefing_path, custo_jev_usd}`.
Códigos de saída: `0` pronto para despachar, `3` reprovado (com `faltando` preenchido), `1` erro de execução.

Reprovado, o orquestrador corrige o que faltou e repete. O ciclo custa frações de centavo e roda antes de qualquer token do Devin.

### 5.2 `task` — dispatch

Cria a worktree isolada (`git worktree add -b <branch> <dir> <base>`), grava o job em disco e **re-executa a si mesmo** como `companion supervise <job-id>` destacado, devolvendo o `job-id` imediatamente.

O supervisor executa:

```
devin --model <resolvido> --permission-mode <resolvido> \
      --respect-workspace-trust false \
      --prompt-file <job>/briefing.md \
      --export <job>/export.json \
      -p > <job>/stdout.log 2>&1
```

com `Setpgid`, para que o cancelamento atinja o grupo inteiro.

A worktree nova não tem ambiente. O briefing carrega os caminhos absolutos que o job recebeu por configuração ou flag (`--venv <path>`, `--node-modules <path>`), porque sem saber rodar teste o Devin inventa ou não roda.

### 5.3 `supervise` — watchdog

Poll de `<job>/stdout.log`, que o Devin escreve **incrementalmente enquanto trabalha**. Cada leitura pega só o que chegou desde a anterior e vira janela respeitando o teto do Jev (64k por requisição; 32k para state mais a maior pergunta).

> **Medido em 2026-09-20, contra `devin` 3000.10.31.** A documentação do Devin afirma que `--export` grava após cada turno, e o protocolo de referência afirma o contrário sobre o stdout. **As duas afirmações estão trocadas.** Com um run em andamento e arquivos já escritos no disco, `export.json` não existia, enquanto `stdout.log` crescia de 567 para 792 bytes. O `--export` é escrito no encerramento; o stdout é a única fonte viva. O watchdog lê stdout; o `export.json` continua servindo à compactação pós-run (§5.5), onde já está completo.

Determinístico, em código, sem modelo: a linha literal `rejected a tool call that requires confirmation`, que o `devin` emite ao esbarrar numa confirmação que o modo `-p` não consegue exibir; comando idêntico repetido com a mesma saída; arquivo tocado fora do escopo declarado (via `git status` na worktree); processo sem escrita há N minutos.

Semântico, via Jev (§6.3): `sem_progresso`, e só ele. `bloqueio_de_permissao` saiu do Jev quando se mediu que o `devin` anuncia a recusa numa string estável — o que dá para saber contando não vai para o modelo.

Política, configurável: cancela quando um sinal cruza o limiar em **duas janelas consecutivas**; exceção, a recusa de ferramenta cancela na primeira, por ser terminal por natureza — o processo não vai se recuperar de uma confirmação que ninguém pode dar. Ao cancelar, grava `cancel-reason.json` com o sinal, a probabilidade, o **trecho verbatim do turno** que o provocou e o comando de retomada pronto para colar, e manda `SIGTERM` ao grupo, `SIGKILL` após 10s.

O aviso chega por três caminhos, do mais confiável ao mais conveniente: `status` e `result` abrem com o bloco de cancelamento; um hook `PostToolUse` do plugin detecta job cancelado desde a última chamada e injeta o motivo no contexto do Claude, para o orquestrador reagir sem ser perguntado; e, quando `notify.osascript` estiver ligado na configuração, uma notificação de sistema no macOS. Os dois primeiros são obrigatórios, o terceiro é opcional e desligado por padrão.

### 5.4 Verificação — código, sem modelo

Ao terminar (ou ser cancelado com diff não vazio), o companion executa e captura:

1. `git diff` completo da worktree.
2. O comando de teste declarado no briefing — exit code e stdout.
3. **Teste de mutação**: numa cópia descartável da worktree, `git apply -R` dos hunks de arquivos não-teste e nova execução do teste, exigindo vermelho. Verde aqui significa que o teste não prova nada.
4. A suíte do pacote tocado, não só o arquivo — comando vindo de `--suite-cmd`, ou da configuração do repo, ou, na falta dos dois, o mesmo comando de teste com o recorte de arquivo removido. Uma mudança de comportamento correta já derrubou 2 testes de unidade e 125 cenários de e2e em fixtures que montavam objeto incompleto; rodar só a pasta tocada teria escondido isso.
5. O lint do repositório.

Resultado em `verify.json`, com exit codes. Isto são fatos; nenhum deles depende de julgamento de modelo.

### 5.5 `result` — o que chega ao orquestrador

Bloco de veredito com os números de `verify.json`, seguido do trace **compactado por deleção** (§6.5, volta) e da flag de coerência (§6.6) quando o relatório afirma verde e o exit code discorda.

### 5.6 `wait-and-result`

Loop de poll com a semântica já validada no `opencode-plugin-cc`: exit `0` pronto (stdout é o relatório), exit `2` ainda rodando (o chamador repete), exit `1` erro. Existe porque o `Bash` do Claude corta em 10 minutos e run de Devin não respeita esse limite.

### 5.7 `review` — modo revisor

`--permission-mode auto`, sem escrita. Recorte obrigatório: arquivos, range de diff e perguntas do domínio. Cada achado é triado (§6.7) e a camada de opinião de estilo é descartada antes de chegar ao modelo caro.

## 6. As perguntas Jev

Todas as perguntas vivem em `internal/jev/questions.go`, versionadas, com fixture rotulada correspondente em `evals/`. Limiares ficam em configuração, não em constante.

### 6.1 Gate de delegabilidade

State: campos nomeados — `tarefa.texto`, `repo.branch_base`, `repo.arquivos_citados` (os `arquivo:linha` extraídos por regex do texto, com existência conferida em disco) e `repo.pacotes_atingidos` (os diretórios de pacote que contêm esses arquivos). Nada de transcrição de conversa.

Choice `tipo_de_tarefa`: `correcao_com_teste` | `revisao_somente_leitura` | `investigacao` | `nao_delegavel`.

Nouls: `defeito_unico`, `desenho_em_aberto`, `cruza_pacotes`, `toca_sensivel`, `criterio_de_pronto`.

Política: reprova se `tipo_de_tarefa = nao_delegavel`, ou `desenho_em_aberto` alto, ou `toca_sensivel` alto, ou `criterio_de_pronto` baixo. `cruza_pacotes` alto vira aviso, não reprovação — a decisão é do autor.

### 6.2 Gate de briefing

State: o `briefing.md` montado.

Nouls, um por item do protocolo: `aponta_arquivo_linha`, `cita_fonte_do_contrato`, `pede_teste_antes_da_correcao` (verdadeiro só quando pede escrever o teste **e** confirmar o vermelho antes de corrigir), `comandos_copiaveis`, `limites_explicitos`, `pede_relatorio`.

Política: qualquer um abaixo do limiar reprova, e o nome do item volta em `faltando`.

### 6.3 Watchdog

State: janela do export ATIF com os turnos recentes.

Noul: `sem_progresso` (a janela acrescenta informação que as anteriores não tinham?).

`bloqueio_de_permissao` **não é pergunta de Jev**. O `devin` emite `rejected a tool call that requires confirmation` quando isso acontece; um grep resolve, de graça e sem erro de calibragem.

### 6.4 Rota

State: tarefa mais o `tipo_de_tarefa` já decidido.

Choice `permissao`: `auto` para revisão somente leitura, `accept-edits` quando basta editar, `smart` quando precisa rodar teste. `dangerous` **não é opção**: nunca é escolhido automaticamente.

Score `complexidade`, quatro níveis descritos por situação concreta, que o código traduz em nível de esforço.

### 6.5 Compactação por deleção

Mecanismo emprestado de `fast-jev-compaction`: **nunca reescrever, só apagar**.

Na ida, um noul por item de evidência: `evidencia_necessaria`.

Na volta, dois nouls por interação do trace: `chamada_necessaria` e `resultado_necessario_verbatim`. Três desfechos — mantém os dois, mantém a chamada com o resultado truncado, ou remove o par.

Custo: o state se repete a cada requisição, então o gasto escala com o tamanho do trace, não com o número de decisões. Um trace de 50k com 40 interações custa cerca de US$ 0,08 a US$ 0,042 por milhão de tokens de entrada. Aceitável, mas contabilizado (§8).

### 6.6 Coerência do relatório

Noul `relatorio_afirma_verde` sobre o relatório final. A flag sobe quando esse noul é alto **e** o exit code capturado em `verify.json` é diferente de zero. O outro lado da comparação é código, não modelo.

### 6.7 Triagem de achados

Por achado: regex em código para presença de `arquivo:linha`; noul `tem_cenario_reproduzivel`; score `severidade`. O código ordena e corta a camada de estilo.

## 7. Rota de modelo e perenidade

`devin models list` é parseado e cacheado com TTL. A política escolhe, nesta ordem: família gratuita quando existir e atender ao Score de complexidade; senão a mais barata que atenda. `--model` explícito do usuário sempre vence.

Nenhum identificador de modelo aparece hardcoded no código ou nos Markdown. Quando `swe-2-*` sair da promoção, o plugin muda de rota sem alteração de código. O parser tolera famílias e campos novos: linha que não casa com o formato esperado é ignorada com aviso, nunca aborta.

## 8. Orçamento

Cada chamada Jev grava uma linha em `<job>/jev.jsonl`: pergunta, resposta, tokens de entrada, custo. `status` e `result` mostram o acumulado em dólar do job. A economia precisa ser auditável, não prometida — e é a única forma de saber se o watchdog está se pagando.

## 9. Segurança e limites

- `--permission-mode dangerous` nunca é escolhido pela rota; só chega ao Devin se o usuário passar explicitamente.
- O companion não executa `git commit`, `git push` nem operação de rede em nome do Devin.
- Todo trabalho acontece em worktree isolada. Dois jobs nunca apontam para o mesmo diretório; o `job store` mantém lockfile por caminho.
- `TYPESAFE_API_KEY` é lido do ambiente e nunca gravado em disco, log ou relatório.
- O conteúdo do export ATIF é **dado**, não instrução: instruções encontradas ali não alteram o comportamento do companion.

## 10. Estado em disco

`~/.local/state/devin-plugin-cc/jobs/<job-id>/`:

| Arquivo | Conteúdo |
| --- | --- |
| `job.json` | estado, rota escolhida, worktree, pids, timestamps |
| `briefing.md` | o que foi enviado ao Devin |
| `evidence.jsonl` | evidências recebidas, com a marca de quais sobreviveram |
| `export.json` | ATIF bruto, escrito pelo Devin |
| `stdout.log` | saída bruta do `-p` |
| `jev.jsonl` | auditoria de cada chamada Jev |
| `cancel-reason.json` | presente só quando o watchdog cancelou |
| `verify.json` | exit codes e saídas da verificação |
| `result.md` | o relatório renderizado |

## 11. Testes

TDD. `go test ./...` em três camadas:

**Unitária, sem rede**: parser ATIF, fatiamento contra o teto de 64k/32k, montagem de flags do `devin`, parse do `models list` (incluindo famílias desconhecidas e campo `Free`), política de limiar do watchdog, ordenação da triagem.

**Integração**: um `devin` falso compilado de `testdata/fakedevin/` para um diretório temporário à frente do `PATH`, emitindo export roteirizado. Cobre dispatch, detecção pelo watchdog, cancelamento com morte do grupo de processos e montagem do resultado — sem queimar quota nem tocar a rede. Cenários roteirizados: run saudável, run que repete comando falho, run bloqueado por permissão, run que edita fora do escopo.

**Evals**: fixtures rotuladas rodando contra o Jev real, conferindo que cada probabilidade cai do lado certo do limiar. `t.Skip` quando não houver `TYPESAFE_API_KEY`.

CI: `go vet`, `staticcheck`, `go test ./...` sem os evals.

## 12. Riscos

| Risco | Mitigação |
| --- | --- |
| Watchdog mata run saudável | Exige dois turnos consecutivos; `cancel-reason.json` traz o trecho verbatim; `devin -c` retoma sem perder contexto |
| Formato do ATIF muda | Parser tolerante, fixtures em `testdata/`, falha do parser degrada para compactação desligada, não derruba o job |
| Formato do stdout muda, ou ele deixa de ser incremental | O watchdog perde a fonte viva. `doctor` mede isso: dispara um run trivial e confere que o stdout cresce antes do encerramento; se não crescer, avisa que o watchdog está cego |
| Jev descalibrado neste domínio | `evals/` com fixtures rotuladas; limiares em configuração; auditoria em `jev.jsonl` permite recalibrar com dados reais |
| Flags do `devin` mudam entre versões | `doctor` confere `devin --version` e a presença das flags usadas antes do primeiro dispatch |
| Compactação apaga o que importa | Só deleta, nunca reescreve; o export bruto fica em disco e `result --raw` mostra tudo |
