# Delegador — design v2

Data: 2026-09-20 · Status: para revisão
Supersede: [v1](2026-09-20-devin-plugin-cc-design.md), que embrulhava o `devin` CLI.

## 1. O que mudou, e por que

O v1 desenhou um companion que **embrulha um agente de CLI e o observa de
fora**. Oito tarefas implementadas e quatro falhas medidas depois, esse
desenho se mostrou errado em três pontos, todos pela mesma causa: estar do
lado de fora do laço.

**A permissão não é decidível de fora.** O `--permission-mode smart` do
`devin` é um modelo rápido julgando segurança, e ele julga **diferente diante
da mesma entrada**: recusou `chmod`, recusou `git commit`, e recusou uma vez
o mesmo `go test` que aprovara dezenas de vezes. Quatro recusas em nove
tarefas. Contra não-determinismo não existe contorno — só existe ser quem
decide. `dangerous`, a única alternativa oferecida, aprovaria `push`.

**A observação de fora é arqueologia.** O v1 mandava fazer poll do
`export.json` "a cada turno", seguindo a documentação. Medição: `--export` só
é escrito no encerramento; o stdout é que cresce. A documentação do fornecedor
e o protocolo de referência afirmavam coisas opostas, e **os dois estavam
trocados**. Um watchdog que depende de adivinhar o formato de saída de outro
programa está sempre a uma versão de ficar cego.

**Um executor só é barato até a promoção acabar.** O `swe-2` está gratuito
por promoção de setembro/2026. Amarrar o desenho a um executor específico é
herdar o preço dele.

O acesso direto à API resolve os três de uma vez, e está medido: o proxy local
em `http://127.0.0.1:8317/v1` expõe 238 modelos em formato OpenAI, e **os seis
do roster fazem tool call**. Quando nós somos o laço, a permissão é uma lista
em código, os turnos estão em memória, e o executor é um parâmetro.

## 2. Objetivo

Delegar uma tarefa de código a um modelo escolhido pela tarefa, executá-la num
laço que nós controlamos, verificar o resultado com código, e escalar para um
modelo mais forte só quando a verificação reprovar — com Jev nos pontos de
julgamento semântico e código em todo o resto.

### Não-objetivos

- **Não** reconstruir um IDE agêntico. O conjunto de ferramentas é o mínimo
  que fecha o ciclo: ler, escrever, editar, executar comando, listar, buscar.
- **Não** suportar "todos os modelos". Suportado é o que está no roster e
  passou na sondagem de viabilidade. Suporte é medição, não alegação.
- **Não** substituir a verificação humana. O plugin roda teste, mutação, suíte
  e lint e mostra a saída; aceitar continua sendo decisão de quem lê.
- **Não** compactar a sessão do orquestrador. Escopo de outro projeto.

## 3. Decisões

| Decisão | Escolha | Razão |
| --- | --- | --- |
| Executor | Laço próprio contra `/v1/chat/completions` | Permissão determinística, turnos em memória, executor parametrizável |
| Permissão | Allowlist em código, por padrão de comando | O único modo de não herdar o não-determinismo de um juiz externo |
| Escolha de modelo | Jev diz a **dimensão**, código faz a aritmética | Comparar número é código; ler tarefa é julgamento |
| Qualidade | Cascata: barato primeiro, escala por falha **provada** | `glm-5.3-flash` faz 0.758 no tau-bench a $0,0061/tarefa; `opus-5` faz 0.792 a $0,493 |
| Roster | Curadoria humana, viabilidade medida | Disponível ≠ utilizável ≠ pago por você |
| Origem de dado | `sondado`, `benchmark`, `fornecedor`, `humano`, nunca misturados | O fornecedor do `swe-2` publica 0.928 no TB2.1 e 0.273 no TB4 |
| Linguagem | Go 1.27+, stdlib pura | Mantido do v1, validado em 8 tarefas |

## 4. Arquitetura

```
cmd/delegador/main.go
internal/
  cli/        plan route run verify result roster doctor probe
  agent/      laço: turnos, tool calls, parada          ← o coração
  tools/      read write edit exec ls grep + allowlist  ← a permissão
  route/      dimensão → índice → roster → modelo
  roster/     leitura, sondagem de viabilidade, cache de benchmark
  jev/        client questions window budget            ← reaproveitado do v1
  job/        estado em disco, lockfile                 ← reaproveitado do v1
  verify/     teste, mutação, suíte, lint
  cascade/    política de escalada
  gate/       delegabilidade e briefing
  render/     saída de terminal
  ledger/     custo do executor, separado do custo do Jev
  gitx/       worktree, diff, apply -R
  safeenv/    resolução de chave e base URL sem vazar em log
  pluginfiles/ conformidade da superfície do plugin
```

O laço é curto e é o único lugar com estado de conversa:

```
monta o contexto → chama o modelo → recebe tool_calls
  → tools.Allow() decide, em código, o que executa
  → executa e devolve o resultado → repete
  → para por: resposta final, teto de turnos, ou veto do watchdog
```

## 5. Permissão — a correção central

`tools.Allow(call) (bool, motivo)`, avaliada **antes** de qualquer execução,
com três camadas e sem modelo em nenhuma:

1. **Escopo de caminho.** Escrita só dentro da worktree do job, e só nos
   prefixos declarados no briefing. Resolve symlink antes de comparar.
2. **Allowlist de comando**, por padrão e não por string: `go test`, `go
   build`, `go vet`, `pytest`, `npm test`, `chmod +x` dentro de `scripts/`.
   Configurável por repo.
3. **Negação dura, não sobreponível por configuração:** `git push`, `git
   commit`, `git reset --hard`, `rm -rf`, `curl`, `wget`, `ssh`, qualquer
   coisa com credencial no argumento, e o primeiro token comparado pelo
   **nome do binário** — `/usr/bin/curl` é `curl`, `../../bin/ssh` é `ssh`.
4. **Escrita em `.git` é negada em qualquer nível**, no caminho pedido **e**
   no caminho resolvido. Descoberto durante a implementação, não no desenho:
   um symlink dentro da worktree apontando para `.git` não tem o segmento no
   pedido e só o revela depois de resolver. Escrever em `.git/hooks/pre-commit`
   ou em `.git/config` é execução de código arbitrário na próxima operação de
   git — é escalada de privilégio, não edição de arquivo.

Recusa não mata o laço: volta ao modelo como resultado de ferramenta dizendo o
que foi negado e por quê. Ele tenta outro caminho, em vez de morrer no meio —
que é exatamente o que acontecia antes.

**Commit continua sendo do companion**, depois do portão verde, nunca do
modelo. Isso já era regra do v1 por desenho e virou obrigatório por medição.

## 6. Fluxo

### 6.1 `plan` — gates antes de qualquer token caro

Inalterado do v1 e já implementado: o orquestrador entrega o defeito em uma
frase mais evidência verbatim em JSONL; Jev seleciona a evidência item a item;
o código monta o `briefing.md` a partir de template fixo; Jev roda o gate de
delegabilidade e o gate de briefing sobre o artefato montado. Exit 3 reprova
com o nome do item que faltou.

### 6.2 `route` — qual modelo, decidido pela tarefa

Jev responde **três perguntas** sobre o briefing numa requisição, e o gate
(§6.1) entrega uma quarta, `tarefa_autocontida`, que a rota também consome:

- Choice `dimensao_dominante`: `mecanica` | `raciocinio` | `agentica`.
  As três opções existem porque as três têm coluna de benchmark. Dimensão sem
  medida correspondente seria resposta bonita e inútil.
- Score `complexidade` (já existe no v1): define o **percentil de corte**
  dentro do roster, não o modelo.
- Score `volume`: quantos pontos distintos e quantos ciclos a tarefa exige.
  **Não é dificuldade**, e os dois eixos decidem coisas diferentes: a
  complexidade escolhe **qual** modelo, o volume escolhe **quanto** ele pode
  gastar chegando lá. `route.OrcamentoPara` escala o teto de turnos e o teto
  de custo exponencialmente no volume, ancorado no nível 1 — nível 0 recebe
  metade do base, nível 3 recebe oito vezes —, com piso e teto protegendo
  contra resposta degenerada. Medido em 2026-09-21: uma correção de uma linha
  recebeu 15 turnos e US$ 2,53; uma migração mecânica em 31 sítios recebeu 118
  turnos e US$ 19,59, **com o mesmo modelo e a mesma dimensão**. Antes, as
  duas recebiam 30 turnos e US$ 5,00: a primeira desperdiçava folga e a
  segunda morria no turno 30 com o trabalho pela metade.

O código faz o resto, e é aritmética: mapeia dimensão para o índice
(`mecanica`→`coding_index`, `raciocinio`→`intelligence_index`,
`agentica`→`tau_bench`), aplica o corte, filtra por viabilidade sondada e por
`custo_usd_por_mtok` preenchido, e entre os que sobram escolhe o de menor
`custo_por_tarefa_usd` — não o de menor preço por token, porque verbosidade é
custo.

Índices não são comparáveis entre si: `coding_index` vai a 81.6 com mediana
43.4; `agentic_index` vai a 57.9 com mediana 15.8. O corte é **percentil
dentro do roster**, nunca valor absoluto cruzando dimensões.

Modelo sem nota de terceiro entra elegível por viabilidade e custo, marcado
como não medido no relatório. Ausência de nota não é nota baixa.

#### O piso de autocontenção

Preço só pode decidir sozinho quando a tarefa está fechada. Quando
`tarefa_autocontida` fica **abaixo de 0,625**, quem executa vai precisar
sustentar o enquadramento por conta própria — decidir o que o briefing não
decidiu, sem derivar para outro assunto — e aí o mais barato deixa de ser
automaticamente o certo.

Nesse caso entra um **piso adicional de `tau_bench`**, no percentil 0,50 do
roster ou no da complexidade, o que for maior. A escolha do `tau_bench` como
proxy não é arbitrária: ele mede uso de ferramenta em ambiente multi-turno,
que é a coisa mais próxima de "não deriva quando a informação está no
ambiente" que o benchmark oferece. Usar dado que existe é melhor que inventar
um campo de robustez que ninguém mediu.

Duas consequências, e as duas são deliberadas:

**Modelo sem nota de terceiro sai da disputa** em tarefa ambígua. Numa tarefa
fechada ele compete por preço como qualquer outro; numa tarefa que exige
sustentar enquadramento, ausência de medição não vira aposta. Se for o único
disponível, ainda assim é usado — trabalho parado é pior que trabalho feito
por modelo imperfeito —, e o relatório diz que a escolha foi às cegas.

**O piso de ambiguidade não herda o percentil da complexidade.** Uma tarefa
simples e ambígua teria corte baixo e ficaria sem piso nenhum, e essa é
justamente a combinação mais perigosa: parece barata e é difícil de sustentar.
Ambiguidade é eixo próprio e tem mínimo próprio.

Os dois cortes — dimensão e `tau_bench` — são calculados sobre o conjunto
inteiro de candidatos e só então intersectados. Filtrar por um e depois
aplicar o percentil do outro recalcula a posição dentro do subconjunto e
exclui quem deveria passar; foi um defeito real, pego por teste.

**A evidência que motivou.** O `swe-2` fica a 1–3 pontos do topo em benchmark
de código delimitado — FrontierCode 1.1 a 50,0 contra 50,9 do Fable 5.1 — e a
cerca de metade da distância no Terminal-Bench 4, o mais agêntico: 27,3
contra 55,8. O número é do próprio fornecedor, contra o interesse dele. E foi
o que se observou em uso: tanto o `swe-2` quanto o `glm` derivaram de escopo,
escrevendo arquivos de tarefas vizinhas, quando a fronteira não estava
explícita no briefing. Dois modelos diferentes errando igual é sinal do
regime, não deles.

#### Calibragem dos dois limiares, e por que só um é calibrável

`LimiarAutocontida = 0,625` foi **medido** contra as sete fixtures rotuladas
de `evals/`, mediana de três execuções cada: a classe `true` ficou em
0,900–0,940 e a `false` em 0,050–0,350, um vão de 0,550. O corte é o ponto
médio. Se houvesse que descentrar seria para cima, porque os dois erros
custam coisas diferentes: tratar tarefa ambígua como fechada joga fora um run
inteiro, tratar fechada como ambígua compra um modelo um pouco melhor por
centavos.

`PisoTauMinimo = 0,50` **não foi calibrado por fixture, e não dá para ser.**
Ele não é um corte sobre resposta do Jev — é um percentil dentro do roster, e
não existe rótulo dizendo qual percentil de tau basta. Calibrá-lo exigiria
rodar tarefas ambíguas com modelos de tau diferente e medir onde a taxa de
sucesso cai, experimento que a sessão de 2026-09-21 tentou e não concluiu.

O que sustenta o número, então, é o **efeito verificável sobre o roster
real**, fixado em teste: ele exclui exatamente os dois modelos de pior
`agentic_index` — 29,0 e 41,0 — e nenhum outro. Se o roster mudar e o teste
quebrar, o número é que precisa ser revisto.

### 6.3 `run` — o laço

Executa até resposta final, teto de turnos, ou veto. A cada turno, antes da
próxima chamada, uma **pré-condição** avalia:

Código: a mesma chamada de ferramenta repetida com o mesmo resultado; escrita
fora do escopo; nenhuma escrita há N turnos; teto de custo do job atingido.

Jev, e só quando houver janela que justifique: noul `sem_progresso` sobre o
turno estruturado. Quando o modelo emitir `reasoning_content` — medido: dois
dos seis do roster emitem — o julgamento é sobre o raciocínio; quando não,
sobre a sequência de chamadas e resultados. **O watchdog deixa de ser vigia e
vira pré-condição**: ele não observa de fora, ele decide se há próximo turno.

Ao vetar, grava motivo com o trecho verbatim do turno e o comando de retomada.

### 6.4 `verify` — código, sem modelo

Inalterado do v1: `git diff`, o comando de teste, o **teste de mutação**
(`git apply -R` dos hunks de não-teste numa cópia descartável, exigindo
vermelho), a suíte do pacote, o lint. São fatos.

### 6.5 Cascata — onde a qualidade é comprada

Verificação verde: pronto. Verificação vermelha: **escala**, e a escolha do
próximo é a mesma rota com o corte elevado um degrau, com o diff e a saída da
falha entrando no contexto do modelo mais forte como evidência verbatim.

Regras: no máximo uma escalada por tarefa, por padrão; se o forte também
reprovar, para e entrega o caso ao humano com os dois diffs; se o barato
reprovou por recusa de permissão ou por teto de custo, **não escala** — o
modelo não falhou, o ambiente falhou, e escalar aqui é pagar caro por um erro
que não é do executor.

### 6.6 `result`

Bloco de veredito com os números de `verify.json`, flag de divergência quando
o relatório afirma verde e o exit code discorda, trace compactado por deleção
(Jev, dois nouls por interação, nunca reescrever), e a linha de custo: modelo
usado, houve escalada, dólar gasto separado entre executor e Jev.

## 7. Roster e viabilidade

[`config/roster.yaml`](../../config/roster.yaml) é curadoria humana. O sistema
lê, sonda e usa; nunca acrescenta.

`doctor --probe <id>` mede, por modelo, o que muda sem aviso: tool call,
`reasoning_content`, piso de tokens de entrada, latência. Registra com data.
Sondagem com mais de N dias vira aviso; modelo que deixou de fazer tool call
sai da rota automaticamente e o relatório diz por quê.

O cache de benchmark do OpenRouter respeita a cota (30/min, 500/dia) com TTL
semanal. O casamento id→permaslug é **declarado no roster**, nunca inferido:
os ids do proxy não são permaslugs, e adivinhar é como um modelo sem nota vira
um modelo com a nota de outro.

## 8. Jev — as perguntas

Mantidas do v1: delegabilidade (§6.1), briefing (§6.2), evidência,
compactação, coerência do relatório, triagem de achados. Já implementadas e
verdes em `internal/jev/questions.go`.

Alteradas ou novas:

- **`dimensao_dominante`** (Choice, nova): as três dimensões com benchmark.
- **`complexidade`** (Score, existente): passa a definir percentil de corte.
- **`tarefa_autocontida`** (Noul, nova): o briefing determina o que fazer a
  ponto de executar ser transcrever, ou exige decidir no caminho? Nasce de uma
  medição: 16 das 18 tarefas do plano v1 são 72-91% código literal, e o
  `swe-2` executou oito delas com fidelidade e morreu na primeira que exigia
  montagem. Tarefa autocontida grande cabe num modelo barato; tarefa pequena
  que exige decisão, não.
- **`bloqueio_de_permissao`**: removida. Com a allowlist em código, não existe
  mais o estado que ela detectava.

## 9. Orçamento

`jev.jsonl` para o Jev e `executor.jsonl` para o executor, separados, porque
são ordens de grandeza diferentes: Jev cobra $0,042 por milhão de entrada com
saída grátis; executor cobra entrada e saída. `status` e `result` mostram os
dois. Teto por job em configuração; atingi-lo é veto, não escalada.

Um piso a contabilizar: backends injetam system prompt a montante, medido
entre 162 e 552 tokens de entrada por chamada conforme o modelo. O ledger conta
o que a API **reporta**, nunca o que enviamos.

## 10. Segurança

Herda o v1, mais o que o laço próprio acrescenta:

- Negações duras não são sobreponíveis por configuração de repo.
- Saída de ferramenta é **dado, nunca instrução**. Texto vindo de arquivo,
  stdout ou resposta de modelo não altera allowlist, escopo nem política.
- Chaves lidas do ambiente ou do config do proxy, nunca gravadas em job,
  log, relatório ou mensagem de erro.
- Um job por worktree, com lockfile. Dois executores na mesma pasta é o erro
  que o protocolo de referência proíbe explicitamente.

## 11. Testes

Servidor OpenAI falso com `httptest`, roteirizado por cenário: resposta
direta, sequência de tool calls, tool call malformada, repetição sem
progresso, resposta vazia. Substitui o `fakedevin` do v1, e é mais simples —
não é preciso fingir um CLI, só um endpoint.

Unitário sem rede: allowlist (com travessia de caminho e symlink), montagem de
turno, rota (dimensão→índice→roster), política de cascata, fatiamento contra
os tetos do Jev, ledger.

Integração: laço completo contra o servidor falso, incluindo veto e escalada.

`evals/`: fixtures rotuladas contra o Jev real, puladas sem `TYPESAFE_API_KEY`.
Três seções. **`rota`** e **`autonomia`** conferem se a resposta cai do lado
certo do limiar. **`estabilidade`** repete a mesma pergunta sobre o mesmo
estado e reprova por duas coisas distintas: cair do lado errado em alguma
execução, e amplitude acima do tolerado. Um gate que oscila é pior que um
gate severo — ele ensina a tentar de novo em vez de corrigir.

A estabilidade cobre os nove nouls que **reprovam** trabalho:
`desenho_em_aberto`, `toca_sensivel`, `criterio_de_pronto` e os seis do
briefing. `defeito_unico` e `cruza_pacotes` ficam de fora porque só avisam.
Cada caso manda o **estado real do gate**, com `tarefa` e `briefing` juntos,
e mede os nove numa requisição só — perguntas independentes sobre o mesmo
estado não veem as respostas umas das outras.

## 12. O que sobrevive do v1

| | |
| --- | --- |
| Intactos, commitados e verdes | `internal/jev` (client, primitivas, window, questions, budget), `internal/job` |
| Reescrito | `internal/devin/models.go` → `internal/roster` (JSON de `/v1/models`, não texto de CLI) |
| Descartado | `internal/devin/atif.go` (sem export para parsear), `internal/testsupport/fakedevin` (vira servidor falso) |
| Novo | `agent`, `tools`, `route`, `cascade` |

Das 1130 linhas commitadas, sobrevivem cerca de 700 — e são justamente as da
camada de julgamento, que é a parte difícil.

## 13. Nome

`devin-plugin-cc` deixou de descrever o que isto é: o executor é um parâmetro,
e o Devin é um entre seis. Proponho **`delegador`**. Renomear o repositório é
decisão do autor; este documento já usa o nome novo nos caminhos.

## 14. Riscos

| Risco | Mitigação |
| --- | --- |
| O laço próprio erra onde um agente maduro acerta | Escopo mínimo de ferramentas; o servidor falso cobre tool call malformada e resposta vazia; a verificação em código é a rede, e ela não depende do laço |
| Modelo do roster deixa de fazer tool call | `doctor --probe` com data; sai da rota sozinho, com motivo no relatório |
| Benchmark desatualizado ou de configuração diferente | `as_of` no roster; origem do dado marcada; fornecedor nunca no mesmo campo que terceiro |
| Cascata vira desculpa para sempre escalar | Uma escalada por tarefa; escalada só por falha **de verificação**, nunca por recusa de permissão ou teto de custo |
| Allowlist estreita demais trava tarefas legítimas | Recusa volta ao modelo como resultado, não mata o laço; o que foi negado é registrado, e o registro é a fonte para afrouxar com dado |
| Proxy local indisponível | `doctor` confere antes do dispatch e falha com a causa, em vez de deixar o job morrer no meio |

## 15. Desvios medidos na implementação

Registrados depois de executar o plano v2 inteiro e validar de ponta a ponta
contra o proxy real em 2026-09-21. O spec descreve a intenção; esta seção
registra onde a realidade discordou e quem venceu.

### Estado em disco

O v1 previa `verify.json` e `result.md` únicos. A implementação grava
`verify-N.json` e `verify-N.diff` **numerados por tentativa**, mais
`result.txt`. A cascata produz uma verificação por tentativa, e um arquivo só
apagaria a evidência da primeira — que é justamente a que explica por que
escalou. A implementação está certa e o spec estava errado.

Sumiram `export.json` e `stdout.log`: eram do CLI que o v2 não usa. Entraram
`executor.jsonl` e `run.lock`.

O `job.json` ganhou `WritePrefixes`, `AllowCommands`, `TestCmd`, `SuiteCmd`,
`LintCmd` e `TestGlobs`. A `tools.Policy` e a `verify.Config` precisam
sobreviver entre o `plan` e o `run`, e o desenho não dizia onde guardá-las.

### Pré-condição e escopo

O sinal `fora_do_escopo` do §6.3 ficou deferido durante a execução, por um
defeito da assinatura `Precondition func(turns []Turn) *Veto`, que não dava
acesso à política. Corrigido depois: a política entra pelo `PreConfig`, que é
aditivo, e o sinal reusa `tools.Allow` em vez de reimplementar a regra de
escopo. `Allow` **nega** a escrita; o sinal acusa o modelo **insistindo** nela.
Uma tentativa é engano, a segunda é sintoma; sem prefixo declarado o sinal
fica desligado.

### Ambiente

`DELEGADOR_BASE_URL` e `DELEGADOR_API_KEY` para o executor;
`TYPESAFE_BASE_URL` e `TYPESAFE_API_KEY` para o Jev. O `doctor` distingue
"há servidor na ponta" de "a chave funciona": um 401 prova a primeira e
refuta a segunda, e as duas aparecem no relatório.

### Calibragem que o e2e expôs

A primeira execução real — corrigir uma função de uma linha que subtraía em
vez de somar — fechou verde a **US$ 0,0034** (executor 0,0023, Jev 0,0011),
com o teste de mutação provando a correção. Mas duas respostas do Jev
destoaram e precisam de fixture em `evals/`:

- `dimensao_dominante` respondeu **agêntica** para uma troca de sinal de um
  caractere. O esperado era `mecanica`. Rota errada aqui compra modelo caro
  para trabalho trivial, que é exatamente o desperdício que a cascata existe
  para evitar.
- `tarefa_autocontida` respondeu **0,19**, ou seja "exige decidir no
  caminho", para uma tarefa cujo briefing trazia o trecho com `arquivo:linha`,
  o contrato e os comandos. O esperado era alto.

A seleção de evidência manteve 1 de 3 itens — descartou a fonte do contrato e
o erro observado. Defensável, porque o texto da tarefa já citava os dois, mas
merece fixture para confirmar que o critério é esse e não agressividade.

Nenhuma dessas é defeito de código: são as perguntas precisando de calibragem
contra dados reais, que é o que `evals/` existe para fazer.

### O buraco de volume, encontrado e fechado

Calibrar expôs um defeito de taxonomia que o desenho não tinha visto:
`agentica` misturava **informação que só existe executando** com **alteração
que se espalha por muitos pontos**. Não é o mesmo eixo, e o roteamento manda
`agentica` para `tau_bench`, que mede uso de ferramenta em ambiente
multi-turno, não extensão. Extensão saiu da dimensão.

Isso deixou um buraco: com a extensão fora da dimensão, e com a complexidade
medindo dificuldade por mudança e não tamanho — medido, o Jev responde 0,30
de complexidade para uma migração de 31 sítios, e está certo —, **nada no
roteamento capturava volume**. Uma migração de dezenas de pontos e uma
correção de uma linha recebiam o mesmo teto de turnos e de custo.

Fechado pelo Score `volume` (§6.2). A prova de que os eixos ficaram separados
são duas fixtures pareadas em `evals/`: uma com complexidade baixa e volume
alto (a migração), outra com complexidade alta e volume baixo (a janela de
concorrência entre cache e expiração). As duas passando significa que o
sistema não confunde mais tamanho com dificuldade.

### Oscilação: encontrada, explicada e travada

Uma execução do gate aprovou um briefing e outra reprovou o mesmo, em
`criterio_de_pronto`. Medir a pergunta isolada mostrou 0,130 a 0,160,
perfeitamente estável, e produziu a conclusão errada de que não havia
oscilação. **A medição estava errada porque o estado estava errado:** ela
omitia `briefing.texto`, que o gate real inclui. Com o estado de verdade, a
mesma pergunta devolvia **0,540 a 0,610** — em cima do limiar de 0,60, caindo
do lado errado em 2 de 6 execuções.

A causa era um defeito de especificação. `criterio_de_pronto` falava de
`tarefa.texto` enquanto via também `briefing.texto`, e os dois discordavam: a
tarefa não dizia como saber que terminou, o briefing dizia, com a ordem de
confirmar o vermelho e o verde e os comandos copiáveis. O modelo ficava
honestamente incerto, e um Noul perto de 0,5 é isso — probabilidade parecida
para sim e não —, não ruído.

A pergunta passou a julgar os dois **como um artefato só**, que é o que o
executor recebe, e a reconhecer que "escreva o teste que expõe o defeito e
faça passar" é critério de término: o verde é a prova. Depois da correção,
0,960 cravado no mesmo estado.

Varredura posterior dos nove gates bloqueantes não encontrou outra oscilação.
Num briefing bom, amplitude máxima 0,070 e menor margem de 0,16 até o limiar;
num briefing **quase bom** — com `arquivo:linha` presente mas contrato vago,
um comando só, limites frouxos e sem pedido de relatório — as cinco
reprovações ficam entre 0,030 e 0,240, com amplitude de no máximo 0,040. O
caso quase bom é o que importa: hesitar no briefing obviamente ruim não custa
nada.

**A lição de método sobrevive ao caso:** medir uma pergunta contra uma
reconstrução do estado, em vez do estado que o sistema monta, esconde o
defeito e produz confiança injustificada.

### O que só apareceu usando o artefato em trabalho real

Em 2026-09-21 o delegador foi posto para executar tarefas do plano v1 com a
implementação removida — os testes do plano no disco, o modelo autorando
apenas o código. Quatro defeitos apareceram, e **nenhum deles seria pego por
teste unitário**, porque os quatro exigem trabalho de tamanho real.

**A tolerância de ociosidade não escalava.** Numa tarefa de volume 2,01, com
60 turnos concedidos, o modelo gastou 10 turnos lendo o contrato e os testes
antes da primeira escrita, e o sinal `sem_escrita` — fixo em 10 — o matou no
turno 10 de 60. O limiar estava calibrado para "corrija um defeito", onde se
lê dois arquivos e edita. Autorar exige ler antes de escrever, e o volume já
sabia que a tarefa era grande. `TurnosOciosos` passou a escalar junto com
turnos e custo.

**Duas etapas discordavam entre si.** A seleção de evidência descartou a fonte
do contrato — manteve 2 de 3 itens numa execução e 1 de 3 na seguinte — e o
gate `cita_fonte_do_contrato` reprovou por falta dela. O selecionador podia
jogar fora justamente o item que um gate bloqueante exige. Não é julgamento, é
acoplamento, e virou garantia em código: quando nenhum item de um tipo exigido
sobrevive, o melhor pontuado daquele tipo volta. Sem item do tipo, nada é
inventado.

**`gitx` deixava o índice sujo.** `Diff` e `DiffStat` rodam `git add -AN`, que
é o que faz arquivo novo aparecer no diff, e nunca desfaziam. O companion
terminava deixando o repositório do usuário com arquivos em *intent-to-add* —
e nesse estado `git clean` não remove e `git checkout -- .` **trunca para zero
byte**. Numa medição real, uma tarefa herdou dois arquivos vazios da execução
anterior e falhou por um motivo que não era dela. Agora o efeito é desfeito,
mas só quando nada estava staged antes: índice ocupado é trabalho de outra
pessoa.

**E um defeito que não era do artefato, e vale igual.** O arnês de medição
passava `go test ./...` como comando de teste **e** como suíte, deixando a
seção de Comandos do briefing com a mesma linha repetida; o gate
`comandos_copiaveis` reprovou duas tarefas. O gate estava certo — aquilo não
é instrução copiável, é ruído. Vale registrar porque mostra que os gates
pegam briefing ruim mesmo quando quem o escreveu foi o próprio autor do
sistema.
