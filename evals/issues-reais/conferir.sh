#!/bin/bash
# Compara o que o modelo escreveu com a correcao do mantenedor. Nao e
# criterio — o criterio e o teste do mantenedor, que ja rodou. Isto e so
# para o relatorio dizer se a rota foi a mesma ou outra igualmente valida.
while IFS=$'\t' read -r n sha pr iss; do
  echo "=== caso $n (issue #$iss) ==="
  M=$(git -C /tmp/swebench/expr show --numstat --format= "$sha" | grep -v '_test\.go\|test/issues' | awk '{a+=$1;d+=$2;f++} END{printf "%d arq, +%d -%d", f,a,d}')
  E=$(git -C /tmp/swebench/c$n diff --numstat | awk '{a+=$1;d+=$2;f++} END{printf "%d arq, +%d -%d", f,a,d}')
  echo "  mantenedor: $M"
  echo "  modelo:     $E"
  echo "  arquivos mantenedor: $(git -C /tmp/swebench/expr show --name-only --format= "$sha" | grep '\.go$' | grep -v '_test\.go\|test/issues' | tr '\n' ' ')"
  echo "  arquivos modelo:     $(git -C /tmp/swebench/c$n diff --name-only | tr '\n' ' ')"
done < /tmp/swebench/casos.tsv
