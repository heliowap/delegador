#!/bin/bash
set -euo pipefail
SRC=/tmp/swebench/expr
while IFS=$'\t' read -r n sha pr iss; do
  dir=/tmp/swebench/c$n
  rm -rf "$dir"; mkdir -p "$dir"
  # arvore do commit PAI: o bug, sem nenhuma historia futura no object store
  git -C "$SRC" archive "$sha^" | tar -x -C "$dir"
  git -C "$dir" init -q -b main
  git -C "$dir" add -A
  git -C "$dir" -c user.name=eval -c user.email=eval@local commit -qm "expr@${sha}^ (antes da correcao)"
  # oraculo: os arquivos de teste como o mantenedor os escreveu
  arquivos=$(git -C "$SRC" show --name-only --format= "$sha" | grep -E '(_test\.go|test/mock/mock\.go)$' || true)
  for f in $arquivos; do
    mkdir -p "$dir/$(dirname "$f")"
    git -C "$SRC" show "$sha:$f" > "$dir/$f"
  done
  git -C "$dir" add -A
  git -C "$dir" -c user.name=eval -c user.email=eval@local commit -qm "oraculo: testes do mantenedor (PR #$pr, issue #$iss)"
  echo "caso $n: $(git -C "$dir" log --oneline | wc -l | tr -d ' ') commits; testes: $(echo $arquivos | tr ' ' '\n' | wc -l | tr -d ' ')"
done < /tmp/swebench/casos.tsv
