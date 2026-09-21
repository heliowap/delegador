# delegador

Delega uma tarefa de código a um modelo escolhido **pela tarefa**, executa num
laço próprio, verifica o resultado com código, e escala para um modelo mais
forte só quando a verificação reprova.

[Jev](https://docs.typesafe.ai), o modelo System One da TypeSafe, entra onde é
preciso julgar significado. Código entra em todo o resto — inclusive na
permissão, que é o ponto onde delegação costuma quebrar.

## Por que existe

Delegar custa caro em dois recursos: tempo de parede, quando um run condenado
ocupa quarenta minutos antes de alguém perceber; e token do modelo caro,
quando ele lê um relatório inteiro para descobrir se "verde" era verdade.

A primeira versão embrulhava um agente de CLI e o observava de fora. Em oito
tarefas, quatro morreram no meio porque um juiz de permissão externo recusou
`chmod`, `git commit` e — uma vez, sem mudar nada — o mesmo `go test` que
aprovara dezenas de vezes. Nenhuma dessas falhas era do modelo.

O v2 põe o laço do nosso lado. A permissão vira lista em código, os turnos
ficam em memória, e o executor vira parâmetro.

## Como decide

O Jev responde três perguntas sobre o briefing, numa requisição:

- **dimensão** — mecânica, raciocínio ou agêntica. Define qual índice de
  benchmark corta o roster.
- **complexidade** — define o percentil de corte. Escolhe **qual** modelo.
- **volume** — quantos pontos e quantos ciclos. Escolhe **quanto** ele pode
  gastar chegando lá: teto de turnos e teto de custo.

Complexidade e volume são eixos distintos de propósito. Uma migração mecânica
em trinta arquivos é fácil e grande; uma linha sutil de concorrência é difícil
e pequena. Medido: uma correção de uma linha recebe 15 turnos e US$ 2,53; uma
migração em 31 sítios recebe 118 turnos e US$ 19,59, **com o mesmo modelo**.

Depois o código faz a aritmética — filtra o roster, aplica o corte, escolhe
pelo **custo por tarefa**, não por preço por token. O barato executa, a
verificação em código julga, e o forte só entra por falha provada.

A diferença que isso compra: `glm-5.3-flash` faz 0,758 no tau-bench a
**US$ 0,0061 por tarefa**; `opus-5` faz 0,792 a **US$ 0,493**.

## Como verifica

Sem modelo no caminho: `git diff`, o comando de teste, o **teste de mutação**
— desfaz a correção numa cópia descartável e exige que o teste fique vermelho
—, a suíte do pacote e o lint. Se o relatório afirmar verde e um exit code
discordar, ou se o teste passar também com a correção desfeita, o resultado
abre com um bloco de divergência.

## Estado

O plano v2 está implementado na branch `v2/base`, e validado de ponta a ponta
contra o proxy local: uma correção completa, com a mutação provando, por
**US$ 0,0034**.

| | |
| --- | --- |
| Testes | 189, mais 25 fixtures contra o Jev real |
| Pacotes | 18, ~11 mil linhas de Go |
| Dependências externas | nenhuma |

O que ainda **não** aconteceu: o plugin nunca foi instalado num Claude Code de
verdade; a cascata só rodou em teste, nunca escalou em trabalho real; e o
`config/roster.yaml` está com os seis `custo_usd_por_mtok` em `null`, o que
deixa zero modelos elegíveis até alguém preencher o próprio custo.

## Começando

```bash
export DELEGADOR_BASE_URL=http://127.0.0.1:8317/v1
export DELEGADOR_API_KEY=...        # a chave do seu proxy
export TYPESAFE_API_KEY=...         # para os gates e o watchdog

go build -o bin/delegador ./cmd/...
bin/delegador doctor
```

O `doctor` diz o que falta, e lista **o motivo de cada modelo excluído** em
vez de só dizer que nenhum serve.

## Documentos

- [Design v2](docs/superpowers/specs/2026-09-20-delegador-v2-design.md) — atual, com os desvios medidos no §15
- [Plano v2](docs/superpowers/plans/2026-09-20-delegador-v2.md) — executado
- [Roster](config/roster.yaml) — modelos habilitados, com a origem de cada dado marcada
- [Evals](https://github.com/heliowap/delegador/blob/v2/base/evals/README.md) — fixtures de calibragem e de estabilidade
- [Design v1](docs/superpowers/specs/2026-09-20-devin-plugin-cc-design.md) e [plano v1](docs/superpowers/plans/2026-09-20-devin-plugin-cc.md) — supersedidos; embrulhavam o `devin` CLI

## Uma nota sobre o método

Três defeitos sérios deste projeto foram encontrados **atacando a própria
implementação depois que os testes já estavam verdes**: um bypass da negação
dura por caminho absoluto, um bypass da proteção de `.git` por symlink, e uma
pergunta do gate oscilando em cima do limiar porque via evidência
contraditória. Nenhum apareceu na primeira bateria de testes, e dois estavam
em código escrito e revisado por quem depois os encontrou.

É por isso que a verificação aqui é código, e não relatório.
