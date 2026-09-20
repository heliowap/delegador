---
name: delegador-verification
description: O que a verificacao em codigo prova sozinha — sonda de mutacao, flag de divergencia — e o que continua sendo seu
---

# Verificacao

O `run` executa e captura, sem modelo nenhum no caminho: o diff, o comando
de teste, a **sonda de mutacao** (desfaz a correcao numa copia descartavel
e exige que o teste fique vermelho — teste que continua verde sem a
correcao nao prova nada), a suite do pacote tocado e o lint. Cada tentativa
grava `verify-N.json` e `verify-N.diff` no diretorio do job.

Dois blocos merecem sua atencao imediata quando aparecem no `result`:

**DIVERGENCIA — relatorio afirma verde, comando deu erro.** O modelo relata
"vermelho, correcao, verde" com a mesma confianca quando rodou e quando
nao rodou. Aqui o exit code discorda dele. Leia a saida capturada.

**Mutacao nao provou nada.** O teste passa tambem com a correcao desfeita.
Ele pode estar fixando a funcao errada, ou nao tocar o caminho corrigido.

O que a verificacao nao faz por voce: ler o diff inteiro. Faca isso antes
de trazer qualquer coisa para a sua branch. E rode a suite do pacote, nao
so o arquivo tocado.

O `delegador` nunca faz commit nem push: o commit e do orquestrador, depois
do portao verde, nunca do modelo. Trazer o trabalho e decisao sua.
