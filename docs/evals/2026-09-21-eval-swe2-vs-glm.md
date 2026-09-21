# Eval `swe-2` × `glm-5.3-flash` — relatório

Data: 2026-09-21 · Status: **inconclusivo como comparação de modelos**
Handoff de origem: [2026-09-20-eval-swe2-vs-glm.md](../handoffs/2026-09-20-eval-swe2-vs-glm.md)

## Veredito, primeiro

**O eval não produziu a comparação que pedia.** Das dezoito execuções
tentadas em quatro rodadas, **duas** produziram resultado atribuível ao
modelo, e elas nem são da mesma tarefa. Não há base para decidir o `papel` de
nenhum dos dois no [roster](../../config/roster.yaml), que permanece como
estava.

**O eval entregou outra coisa, e ela vale mais:** cinco defeitos do próprio
delegador, nenhum dos quais teste unitário pegaria, mais um defeito de método
sobre como medir perguntas do Jev.

## Os dois resultados atribuíveis

| modelo | tarefa | veredito | custo | tempo | observação |
| --- | --- | --- | --- | --- | --- |
| `cpa-fw-glm-5.3-flash` | 13 (verify + gitx) | **aprovado** | US$ 0,299 | 7min10 | teste do plano idêntico byte a byte; 4 de 4 passando, incluindo o de mutação |
| `devin/swe-2` | 12 (watchdog) | **reprovado** | US$ 0,084 | 4min23 | escreveu arquivos da tarefa 14; vetado por `sem_escrita` no turno 11 |

Duas execuções, tarefas diferentes, dificuldades diferentes. Isso é anedota,
não medição. Quem ler esta tabela e concluir que um modelo é melhor que o
outro estará inventando o que o dado não diz.

## Por que as outras dezesseis não contam

**Cinco por 503 do Jev.** A TypeSafe oscilou: cinco sondagens seguidas deram
`503 503 200 200 200`. O cliente tinha 3 tentativas com base de 500ms,
cobrindo 3,5 segundos — curto demais. Um `plan` faz uma chamada por item de
evidência, então um único 503 derrubava o `plan` inteiro e o braço junto.

**Quatro por contaminação entre execuções.** `gitx.Diff` roda `git add -AN`
para incluir arquivo novo no diff e não desfazia. Nesse estado `git clean`
não remove o arquivo e `git checkout -- .` o **trunca para zero byte**. Uma
tarefa herdava stubs vazios da anterior e falhava por um motivo que não era
dela. Reproduzido em repositório limpo antes de acreditar.

**Quatro por briefing ruim do arnês de medição.** Comandos duplicados que o
gate `comandos_copiaveis` reprovou — com razão, aquilo não era instrução
copiável. E um `ref` de evidência apontando para o plano de 7400 linhas
**dentro da worktree**, o que fez um modelo gastar cinco dos oito turnos
brigando com leitura truncada até ser vetado.

**Três por cota do Devin esgotada**, na primeira rodada, antes de a execução
migrar para o proxy local. O handoff mandava rodar o modelo gratuito primeiro
e o pago depois — ordem que dava ao gratuito acesso irrestrito e deixava o
pago competindo com o resto do dia.

## O que o eval de fato mediu: o delegador

Cinco defeitos, todos encontrados pondo o artefato para trabalhar de verdade,
nenhum ao alcance de teste unitário porque todos exigem tarefa de tamanho real.

**Tolerância de ociosidade que não escalava.** Numa tarefa de volume 2,01 com
60 turnos concedidos, o modelo gastou 10 turnos lendo o contrato antes da
primeira escrita e o sinal `sem_escrita`, fixo em 10, o matou no turno 10 de
60. O limiar servia para "corrija um defeito", não para autorar.

**Seleção de evidência descartando o que um gate exige.** O selecionador
jogou fora a fonte do contrato — 2 de 3 itens numa execução, 1 de 3 na
seguinte — e o gate `cita_fonte_do_contrato` reprovou por falta dela. Duas
etapas discordando garante reprovação; virou garantia em código.

**`gitx` deixando o índice sujo.** Descrito acima. Efeito invisível até
destruir trabalho.

**`criterio_de_pronto` oscilando no limiar.** A pergunta falava de
`tarefa.texto` enquanto via também `briefing.texto`, e os dois discordavam:
0,540 a 0,610, lado errado em 2 de 6. Depois de reescrita para julgar os dois
como um artefato só, 0,960 cravado.

**Retry do Jev curto demais.** Descrito acima.

## O defeito de método, que é o achado mais transferível

Ao investigar a oscilação do `criterio_de_pronto`, medi a pergunta isolada e
obtive 0,130 a 0,160, perfeitamente estável — e **concluí, errado, que não
havia oscilação**. A medição omitia `briefing.texto`, que o gate real inclui.
Com o estado de verdade, a mesma pergunta devolvia 0,540 a 0,610.

**Medir uma pergunta contra uma reconstrução do estado, em vez do estado que
o sistema monta, esconde o defeito e produz confiança injustificada.** A
camada `estabilidade` em [`evals/`](https://github.com/heliowap/delegador/blob/v2/base/evals/README.md)
existe por causa disso, e cada caso dela manda o estado real.

## O que funcionou, e merece registro

**O watchdog matou laço travado duas vezes**, pelos motivos certos: uma por
`sem_progresso` a 0,84 quando o modelo se afogou no documento, outra por
`sem_escrita` quando vagou para a tarefa errada. Custaram US$ 0,04 e US$ 0,08
em vez dos 39 e 36 turnos concedidos.

**Os gates reprovaram briefings ruins quatro vezes, e os quatro eram meus.**
Eles funcionam contra quem escreveu o sistema.

**O relatório declarou o que não sabia.** Quando o `swe-2` foi escolhido, a
saída trouxe *"modelo não medido: sem benchmark de terceiro, entrou por
viabilidade e custo"* em vez de deixar a ausência de nota implícita.

## Para fazer a comparação direito

1. **Delimitar escopo no briefing.** Os dois modelos vagaram para tarefas
   vizinhas — sinal de briefing, não de modelo. O arnês precisa dizer
   "implemente apenas estes arquivos, ignore o resto do repositório".
2. **Mais tarefas, e pareadas por dificuldade.** Três tarefas e dois modelos
   não têm poder para decidir nada, ainda que tudo corra bem. Comparar exige
   o mesmo conjunto de tarefas nos dois braços, e tarefas suficientes para
   que uma falha isolada não domine.
3. **Isolar o arnês antes de medir modelo.** Das dezoito execuções, treze
   morreram por causa do instrumento. Um eval deveria começar por uma tarefa
   de calibragem conhecida, cujo resultado esperado já se sabe.

## Custo

Cerca de US$ 1 em inferência e aproximadamente duas horas de execução, quase
todas gastas medindo o instrumento. Não foi desperdício: cinco defeitos do
artefato e um de método saíram daí, e nenhum deles apareceria de outro jeito.
Mas foi caro para o que se pretendia, e vale dizer.
