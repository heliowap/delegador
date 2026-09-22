#!/bin/bash
# Piloto de capacidade: uma execucao por celula (modelo x tarefa), com a
# cascata DESLIGADA. O que se mede e o modelo, nao o resgate.
set -u
D="$HOME/VSCode/delegador/bin/delegador"
export DELEGADOR_BASE_URL=http://127.0.0.1:8317/v1
export DELEGADOR_API_KEY=$(grep -A1 "^api-keys:" "$HOME/VSCode/Lab/CLIProxyAPI/config.yaml" | tail -1 | sed -E 's/^[[:space:]]*-[[:space:]]*"?([^"[:space:]]+)"?.*/\1/')
export TYPESAFE_API_KEY=$(grep -E "^export TYPESAFE_API_KEY" "$HOME/.zshrc" | head -1 | sed "s/.*=//" | tr -d "\"' ")
RUN=/tmp/piloto/run; mkdir -p "$RUN"; export XDG_STATE_HOME="$RUN/state"
[ -f "$RUN/placar.tsv" ] || printf 'modelo\ttarefa\tveredito\tparada\tturnos\ttok_in\ttok_out\tseg\talterou_teste\tarq_extra\tjob\n' > "$RUN/placar.tsv"

ARMS="${ARMS:-swe2 glm deepseek muse gemini38 opus46}"
TAREFAS="${TAREFAS:-facil medio dificil}"

for arm in $ARMS; do
  IFS=$'\t' read -r _ MID PIN POUT _ < <(grep "^$arm	" /tmp/piloto/modelos.tsv)
  python3 /tmp/piloto/roster-de.py "$MID" "$PIN" "$POUT" > "$RUN/r-$arm.yaml"
  for tf in $TAREFAS; do
    IFS=$'\t' read -r _ CASO ISS SHA TURNOS < <(grep "^$tf	" /tmp/piloto/tarefas.tsv)
    WT=/tmp/piloto/wt/$arm-$tf; P=/tmp/swebench/prep/$CASO
    grep -q "^$arm	$tf	" "$RUN/placar.tsv" && { echo "[pula] $arm/$tf"; continue; }
    echo "[$(date +%H:%M:%S)] $arm / $tf (issue #$ISS)"
    ( cd "$WT" && git reset -q && git clean -fdq && git checkout -q -- . )
    t0=$(date +%s)
    JSON=$("$D" plan --task "$(cat "$P/tarefa.txt")" --evidence "$P/evidencia.jsonl" \
        --worktree "$WT" --testes-prontos --roster "$RUN/r-$arm.yaml" \
        --test-cmd "$(sed -n 1p "$P/cmds.txt")" --suite-cmd "$(sed -n 2p "$P/cmds.txt")" \
        --lint-cmd "$(sed -n 3p "$P/cmds.txt")" --branch main --json 2>"$RUN/$arm-$tf.plan.err")
    rc=$?
    if [ $rc -ne 0 ]; then
      printf '%s\t%s\tPLAN_%s\t-\t-\t-\t-\t%s\t-\t-\t-\n' "$arm" "$tf" "$rc" "$(( $(date +%s)-t0 ))" >> "$RUN/placar.tsv"
      echo "  -> plan saiu $rc"; echo "$JSON" > "$RUN/$arm-$tf.plan.json"; continue
    fi
    echo "$JSON" > "$RUN/$arm-$tf.plan.json"
    JOB=$(echo "$JSON" | python3 -c 'import json,sys;print(json.load(sys.stdin)["job_id"])')
    "$D" run --job "$JOB" --roster "$RUN/r-$arm.yaml" --max-turns "$TURNOS" --max-escaladas 0 --turnos-ociosos "${OCIOSOS:-0}" \
        >"$RUN/$arm-$tf.result.txt" 2>"$RUN/$arm-$tf.run.err"; rrc=$?
    dt=$(( $(date +%s)-t0 ))
    R="$RUN/$arm-$tf.result.txt"
    if grep -q '^veredito: verde' "$R" 2>/dev/null; then VER=VERDE; else VER=VERMELHO; fi
    PARADA=$(grep -oE 'CANCELADO pelo watchdog: [a-z_]+' "$R" 2>/dev/null | sed 's/.*: //')
    [ -z "$PARADA" ] && PARADA=final
    TESTE=$( ( cd "$WT" && git diff --name-only | grep -cE '(_test\.go|^test/)' ) | tr -dc 0-9 )
    MANT=$(git -C /tmp/swebench/expr show --name-only --format= "$SHA" | grep '\.go$' | grep -vE '(_test\.go|^test/)' | sort)
    MEU=$( ( cd "$WT" && git diff --name-only | grep -vE '(_test\.go|^test/)' | sort ) )
    EXTRA=$(comm -13 <(echo "$MANT") <(echo "$MEU") | grep -c . | tr -dc 0-9)
    read -r TIN TOUT TURNS < <(python3 -c "
import json,sys
p='$RUN/state/delegador/jobs/$JOB/executor.jsonl'
tin=tout=n=0
try:
    for l in open(p):
        d=json.loads(l); tin+=d['input_tokens']; tout+=d['output_tokens']; n+=1
except OSError: pass
print(tin,tout,n)")
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$arm" "$tf" "$VER" "$PARADA" "$TURNS" "$TIN" "$TOUT" "$dt" "$TESTE" "$EXTRA" "$JOB" >> "$RUN/placar.tsv"
    echo "  -> $VER ($PARADA) ${TURNS}t ${dt}s teste_tocado=$TESTE extra=$EXTRA"
  done
done
column -t -s$'\t' "$RUN/placar.tsv"
