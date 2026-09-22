# Piloto de capacidade — seis modelos, três degraus

**2026-09-21.** Dezoito células. O que se queria medir era a capacidade dos
modelos disponíveis. O que o piloto mediu primeiro foi **o delegador**.

## Veredito

**Na primeira passada, 14 de 17 células morreram em `sem_escrita`.** Quase
todas exatamente no turno 10 ou 13 — o piso do watchdog. Num repositório
grande e desconhecido, ler dez arquivos antes da primeira escrita é preâmbulo
normal, não ociosidade; o limiar foi calibrado em worktree de tarefa única.

Com o limiar em 25, o quadro muda de lado:

| modelo | fácil | médio | difícil | verdes | tok/tarefa | s/tarefa | fidelidade |
|---|---|---|---|---:|---:|---:|---:|
| `cpa-ag-claude-opus-4-6-thinking` | ok | ok | **ok** | **3/3** | 505k | 119 | 2/3 |
| `devin/swe-2` | ok | ok | entregou errado | 2/3 | 636k | 172 | 2/3 |
| `cpa-ocgo-deepseek-v4.1-flash` | ok | ok | `sem_escrita` | 2/3 | 702k | 108 | 3/3 |
| `cpa-ag-gemini-3.8-flash-high` | ok | ok | `sem_escrita` | 2/3 | 762k | 110 | 3/3 |
| `cpa-ocgo-muse-spark-1.3(xhigh)` | ok | ok | entregou errado | 2/3 | 883k | 308 | 3/3 |
| `cpa-ocgo-glm-5.3-flash` | desistiu | canal caiu | `sem_escrita` | 0/3 | — | 3/3 |

A mesma célula que na primeira passada era "o modelo não conseguiu" virou
verde na segunda. `swe-2` / médio: `sem_escrita` aos 10 turnos → **verde aos
20**. Ele resolvia a tarefa o tempo todo; o watchdog o matava a dez turnos do
fim.

Isso contamina o eval das issues reais da manhã: os dois `sem_escrita` de
#836 e #685 eram provavelmente a mesma coisa.

## Método

Três tarefas do `evals/issues-reais`, escolhidas pela dificuldade **já
medida** com o opus-5 naquele eval, não por intuição: fácil (#857, 5 turnos),
média (#888, 8-10 turnos), difícil (#836, falhou duas vezes e precisou de
escalada). Oráculo do mantenedor, repositório no commit pai.

Um roster de **um modelo** por braço, para a rota deixar de ser variável, e
`--max-escaladas 0`: se a cascata resgatasse, o resultado da célula seria do
resgate.

## O que a capacidade diz

**O candidato para o lado difícil existe.** `cpa-ag-claude-opus-4-6-thinking`
fez 3 de 3 — incluindo a tarefa que o `opus-5(low)` falhou duas vezes de
manhã e que só saiu com escalada para o fable. **Na cota do Antigravity, não
na do Claude Max.** É o degrau que faltava para os 47% das decisões de rota
que hoje só têm para onde ir dentro da conta do orquestrador.

**O degrau barato aguenta mais do que se supunha.** Quatro dos seis modelos
resolveram fácil e médio. A diferença aparece só no difícil, e ali ela é
nítida: um resolve, dois entregam errado, dois nem chegam a escrever.

**Entregar errado é melhor sinal que ser vetado.** `swe-2` e `muse` chegaram
ao fim da tarefa difícil com uma resposta que a verificação reprovou. Isso é
exatamente o gatilho da cascata — falha provada por verificação de resultado
entregue. `deepseek` e `gemini` pararam por ociosidade, que é ambiente.

**O `glm` é o resultado que mais importa, e é ruim.** Ele vence **132 de 252**
combinações da rota hoje, e fez 0 de 3. Das três, uma não conta (o canal caiu)
e duas são falha real: na fácil ele parou no meio da exploração e devolveu
texto em vez de continuar; na difícil, 25 turnos sem escrever nada.

## Fidelidade: o resultado limpo

**Zero alterações de teste em 35 execuções, com seis modelos.** O briefing com
`--testes-prontos` está segurando o oráculo.

Dois desvios de escopo, os dois na tarefa difícil: `opus-4-6` escreveu um
arquivo além dos que o mantenedor tocou, `swe-2` escreveu dois. Nenhum tocou
em teste.

## Quatro defeitos do delegador, encontrados usando-o

**1. HTTP 400 que é falha de upstream não era retentado.** O
`cpa-ocgo-glm-5.3-flash` devolveu seis 400 seguidos com `"Upstream request
failed"` — rotação de contas do proxy caindo numa ruim — e doze OK na mesma
requisição minutos depois. Sem retry, um desses matava o laço **no turno
zero**, com veredito vermelho e nenhuma linha de trace que explicasse.
Corrigido reconhecendo pelo texto, não pelo status: tratar todo 400 como
transitório esconderia requisição malformada atrás de quatro tentativas
idênticas.

**2. O briefing mandava rodar o que a execução recusava.** A tarefa média
declarava `go test -run TestCheck ./checker/ && go test ./test/issues/888/`.
O briefing mandava rodá-lo, a allowlist o continha inteiro, e `tools.Allow` o
recusava — `&&` é metacaractere de shell, e essa recusa é regra de segurança
que não se negocia.

O resultado foi o pior tipo de falha: **o delegador entregou ao executor uma
ordem que ele estava proibido de cumprir.** O `opus-4-6` tentou o comando
literal do briefing, levou recusa, tentou de novo e foi vetado por
`sem_progresso` em seis turnos. O placar registrou "o modelo não conseguiu".
O modelo obedeceu; quem estava errado era o plano. Com o comando corrigido,
ele fez a tarefa em 14 turnos.

**3. A tolerância a ociosidade não era ajustável.** O número vinha do volume
e só de lá. Agora há `run --turnos-ociosos N`.

**4. Job morto travava a worktree para sempre.** Um run encerrado por `pkill`
nunca executa o `Release` diferido; `plan` recusava, `run` só recupera com
estado `running`, e não havia comando que soltasse. Uma célula ficou sem
medição por isso. Agora há `delegador cancel --job <id>`, que confere o pid
antes de soltar.

## O que este piloto NÃO mede

Uma execução por célula, três tarefas, todas do mesmo repositório em Go.
Separa "não faz" de "faz"; **não ordena modelos próximos**. A variância medida
hoje na issue #685 — verde, veto e verde em três execuções idênticas — é maior
que a diferença entre a maioria destas linhas.

`tok/tarefa` é **faturado acumulado**, não tamanho de contexto: o contexto é
reenviado a cada turno. Para janela, o número é outro — a maior requisição
única de todos os runs de hoje foi de 100.453 tokens, aos 38 turnos, crescendo
~2.638 tokens por turno.

E o `glm` teve uma das três células perdida para a instabilidade do canal, no
canal que eu mesmo escolhi como preferido hoje à tarde. `cpa-fw-*` e `cpa-or-*`
responderam 12 de 12 na mesma sondagem em que o `cpa-ocgo-*` falhou.

## Harness

`evals/piloto/`, com os dois placares — o da primeira passada fica guardado
porque a diferença entre as duas é o achado.
