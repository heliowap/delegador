#!/bin/bash
# Escalada com issues reais: seis bugs fechados do expr-lang/expr, cada um
# no commit PAI da correcao, com os testes do mantenedor ja no disco. O
# oraculo nunca foi escrito por mim nem pelo modelo.
set -u
D="$HOME/VSCode/delegador/bin/delegador"
export DELEGADOR_BASE_URL=http://127.0.0.1:8317/v1
export DELEGADOR_API_KEY=$(grep -A1 "^api-keys:" "$HOME/VSCode/Lab/CLIProxyAPI/config.yaml" | tail -1 | sed -E 's/^[[:space:]]*-[[:space:]]*"?([^"[:space:]]+)"?.*/\1/')
export TYPESAFE_API_KEY=$(grep -E "^export TYPESAFE_API_KEY" "$HOME/.zshrc" | head -1 | sed "s/.*=//" | tr -d "\"' ")
RUN=/tmp/swebench/run
mkdir -p "$RUN"; export XDG_STATE_HOME="$RUN/state"
[ -f "$RUN/placar.tsv" ] || printf 'caso\tissue\tmodelo\tveredito\tescalou\tturnos\texec_usd\tjev_usd\tseg\tdetalhe\n' > "$RUN/placar.tsv"

for N in "$@"; do
  IFS=$'\t' read -r _ SHA PR ISS < <(grep "^$N	" /tmp/swebench/casos.tsv)
  WT=/tmp/swebench/c$N; P=/tmp/swebench/prep/$N
  echo "[$(date +%H:%M:%S)] caso $N (issue #$ISS): preparando"
  ( cd "$WT" && git reset -q && git clean -fdq && git checkout -q -- . )
  t0=$(date +%s)
  JSON=$("$D" plan --task "$(cat "$P/tarefa.txt")" --evidence "$P/evidencia.jsonl" \
      --worktree "$WT" --testes-prontos \
      --test-cmd "$(sed -n 1p "$P/cmds.txt")" \
      --suite-cmd "$(sed -n 2p "$P/cmds.txt")" \
      --lint-cmd  "$(sed -n 3p "$P/cmds.txt")" \
      --branch main ${TETO:+--teto-usd "$TETO"} --json 2>"$RUN/$N.plan.err")
  rc=$?
  if [ $rc -ne 0 ]; then
    MOT=$( [ $rc -eq 3 ] && echo GATE_REPROVOU || echo PLAN_ERRO )
    printf '%s\t#%s\t-\t%s\t-\t-\t-\t-\t%s\t%s\n' "$N" "$ISS" "$MOT" "$(( $(date +%s)-t0 ))" \
      "$(echo "$JSON" | tr '\n' ' ' | head -c 200)$(head -c 200 "$RUN/$N.plan.err" | tr '\n' ' ')" >> "$RUN/placar.tsv"
    echo "  -> $MOT"; echo "$JSON" > "$RUN/$N.plan.json"; continue
  fi
  echo "$JSON" > "$RUN/$N.plan.json"
  JOB=$(echo "$JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["job_id"])')
  MOD=$(echo "$JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["modelo"])')
  echo "  plan ok: job $JOB, modelo $MOD"
  "$D" run --job "$JOB" ${TURNOS:+--max-turns "$TURNOS"} >"$RUN/$N.result.txt" 2>"$RUN/$N.run.err"; rrc=$?
  dt=$(( $(date +%s)-t0 ))
  R="$RUN/$N.result.txt"
  ESC=$(grep -c '^escalou:' "$R" 2>/dev/null); ESC=${ESC:-0}
  VER=$( [ $rrc -eq 0 ] && echo VERDE || echo VERMELHO )
  printf '%s\t#%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$N" "$ISS" "$MOD" "$VER" "$ESC" \
    "$(grep -oE '[0-9]+ turnos' "$R" | head -1 | tr -dc 0-9)" \
    "$(grep -oE 'executor \$[0-9.]+' "$R" | head -1 | tr -dc 0-9.)" \
    "$(grep -oE 'jev \$[0-9.]+' "$R" | head -1 | tr -dc 0-9.)" \
    "$dt" "$(grep -E '^(escalou|cascata):' "$R" | tr '\n' ';' | head -c 160)" >> "$RUN/placar.tsv"
  echo "  -> $VER (escalou=$ESC) em ${dt}s"
done
column -t -s$'\t' "$RUN/placar.tsv"
