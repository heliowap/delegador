#!/bin/bash
# Uma copia do repositorio por CELULA (modelo x tarefa): o delegador trava
# uma worktree por job, e rodar dois bracos na mesma serializaria o piloto.
set -euo pipefail
SRC=/tmp/swebench/expr
while IFS=$'\t' read -r nome caso iss sha turnos; do
  while IFS=$'\t' read -r arm mid _ _ _; do
    dir=/tmp/piloto/wt/$arm-$nome
    rm -rf "$dir"; mkdir -p "$dir"
    git -C "$SRC" archive "$sha^" | tar -x -C "$dir"
    git -C "$dir" init -q -b main
    git -C "$dir" add -A
    git -C "$dir" -c user.name=p -c user.email=p@l commit -qm "antes da correcao"
    for f in $(git -C "$SRC" show --name-only --format= "$sha" | grep -E '(_test\.go|test/mock/mock\.go)$' || true); do
      mkdir -p "$dir/$(dirname "$f")"; git -C "$SRC" show "$sha:$f" > "$dir/$f"
    done
    git -C "$dir" add -A
    git -C "$dir" -c user.name=p -c user.email=p@l commit -qm "oraculo do mantenedor"
  done < /tmp/piloto/modelos.tsv
done < /tmp/piloto/tarefas.tsv
echo "celulas montadas: $(ls /tmp/piloto/wt | wc -l | tr -d ' ')"
