#!/bin/bash
# Ablacao: o MESMO modelo, com e sem as camadas de julgamento do delegador.
# O juiz e externo e identico aos dois bracos — o teste do mantenedor,
# rodado pelo harness depois que o braco termina.
set -u
D="$HOME/VSCode/delegador/bin/delegador"
NU="$HOME/VSCode/delegador/bin/sem-plugin"
export DELEGADOR_BASE_URL=http://127.0.0.1:8317/v1
export DELEGADOR_API_KEY=$(grep -A1 "^api-keys:" "$HOME/VSCode/Lab/CLIProxyAPI/config.yaml" | tail -1 | sed -E 's/^[[:space:]]*-[[:space:]]*"?([^"[:space:]]+)"?.*/\1/')
export TYPESAFE_API_KEY=$(grep -E "^export TYPESAFE_API_KEY" "$HOME/.zshrc" | head -1 | sed "s/.*=//" | tr -d "\"' ")
RUN=/tmp/ablacao/run; mkdir -p "$RUN"; export XDG_STATE_HOME="$RUN/state"
MODELO="${MODELO:-cpa-claude-opus-5(high)}"
[ -f "$RUN/placar.tsv" ] || printf 'braco\ttarefa\tveredito\tdetalhe\tturnos\ttok_in\ttok_out\tseg\talterou_teste\tarq_extra\n' > "$RUN/placar.tsv"

# julga: roda o teste do mantenedor e a suite, na worktree, e ecoa o veredito
julga() {
  local wt=$1 caso=$2 sha=$3
  local tcmd scmd
  tcmd=$(sed -n 1p "/tmp/swebench/prep/$caso/cmds.txt")
  scmd=$(sed -n 2p "/tmp/swebench/prep/$caso/cmds.txt")
  local v=VERMELHO
  if ( cd "$wt" && bash -c "$tcmd" >/dev/null 2>&1 ) && ( cd "$wt" && bash -c "$scmd" >/dev/null 2>&1 ); then
    v=VERDE
  fi
  local teste extra mant meu
  teste=$( ( cd "$wt" && git diff --name-only | grep -cE '(_test\.go|^test/)' ) | tr -dc 0-9 )
  mant=$(git -C /tmp/swebench/expr show --name-only --format= "$sha" | grep '\.go$' | grep -vE '(_test\.go|^test/)' | sort)
  meu=$( ( cd "$wt" && git diff --name-only | grep -vE '(_test\.go|^test/)' | sort ) )
  extra=$(comm -13 <(echo "$mant") <(echo "$meu") | grep -c . | tr -dc 0-9)
  echo "$v $teste $extra"
}

for tf in ${TAREFAS:-facil medio dificil}; do
  IFS=$'\t' read -r _ CASO ISS SHA TURNOS < <(grep "^$tf	" /tmp/piloto/tarefas.tsv)
  P=/tmp/swebench/prep/$CASO
  CMDS="$(sed -n 1p "$P/cmds.txt");$(sed -n 2p "$P/cmds.txt");$(sed -n 3p "$P/cmds.txt")"

  for braco in ${BRACOS:-nu plugin}; do
    grep -q "^$braco	$tf	" "$RUN/placar.tsv" && { echo "[pula] $braco/$tf"; continue; }
    WT=/tmp/ablacao/wt/$braco-$tf
    echo "[$(date +%H:%M:%S)] $braco / $tf (issue #$ISS)"
    ( cd "$WT" && git reset -q && git clean -fdq && git checkout -q -- . )
    t0=$(date +%s)
    if [ "$braco" = nu ]; then
      "$NU" --modelo "$MODELO" --worktree "$WT" --tarefa "$(cat "$P/tarefa.txt")" \
        --evidencia "$P/evidencia.jsonl" --comandos "$CMDS" --max-turns 40 \
        --saida "$RUN/$braco-$tf.json" >"$RUN/$braco-$tf.log" 2>&1
      TURNS=$(python3 -c "import json;print(json.load(open('$RUN/$braco-$tf.json'))['turnos'])" 2>/dev/null || echo 0)
      TIN=$(python3 -c "import json;print(json.load(open('$RUN/$braco-$tf.json'))['tokens_entrada'])" 2>/dev/null || echo 0)
      TOUT=$(python3 -c "import json;print(json.load(open('$RUN/$braco-$tf.json'))['tokens_saida'])" 2>/dev/null || echo 0)
      DET=$(python3 -c "import json;print(json.load(open('$RUN/$braco-$tf.json'))['parada'])" 2>/dev/null || echo erro)
    else
      if [ "$braco" = cascata ]; then
        cp /tmp/ablacao/roster-cascata.yaml "$RUN/r.yaml"
        ESCALADAS=1
      else
        python3 /tmp/piloto/roster-de.py "$MODELO" 0.0 0.0 > "$RUN/r.yaml"
        ESCALADAS=0
      fi
      JSON=$("$D" plan --task "$(cat "$P/tarefa.txt")" --evidence "$P/evidencia.jsonl" \
          --worktree "$WT" --testes-prontos --roster "$RUN/r.yaml" \
          --test-cmd "$(sed -n 1p "$P/cmds.txt")" --suite-cmd "$(sed -n 2p "$P/cmds.txt")" \
          --lint-cmd "$(sed -n 3p "$P/cmds.txt")" --branch main --json 2>"$RUN/$braco-$tf.plan.err")
      if [ $? -ne 0 ]; then
        printf '%s\t%s\tPLAN_FALHOU\t-\t-\t-\t-\t%s\t-\t-\n' "$braco" "$tf" "$(( $(date +%s)-t0 ))" >> "$RUN/placar.tsv"
        echo "  -> plan reprovou"; continue
      fi
      JOB=$(echo "$JSON" | python3 -c 'import json,sys;print(json.load(sys.stdin)["job_id"])')
      "$D" run --job "$JOB" --roster "$RUN/r.yaml" --max-turns 40 --turnos-ociosos 25 \
          --max-escaladas "$ESCALADAS" >"$RUN/$braco-$tf.result.txt" 2>"$RUN/$braco-$tf.run.err"
      TURNS=$(python3 -c "
import json
try:
    print(sum(1 for _ in open('$RUN/state/delegador/jobs/$JOB/executor.jsonl')))
except OSError: print(0)")
      read -r TIN TOUT < <(python3 -c "
import json
tin=tout=0
try:
    for l in open('$RUN/state/delegador/jobs/$JOB/executor.jsonl'):
        d=json.loads(l); tin+=d['input_tokens']; tout+=d['output_tokens']
except OSError: pass
print(tin,tout)")
      DET=$(grep -oE 'CANCELADO pelo watchdog: [a-z_]+' "$RUN/$braco-$tf.result.txt" 2>/dev/null | sed 's/.*: //')
      [ -z "$DET" ] && DET=final
      if grep -q '^escalou:' "$RUN/$braco-$tf.result.txt" 2>/dev/null; then
        DET="$DET+escalou"
      fi
    fi
    dt=$(( $(date +%s)-t0 ))
    read -r VER TESTE EXTRA < <(julga "$WT" "$CASO" "$SHA")
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$braco" "$tf" "$VER" "$DET" "$TURNS" "$TIN" "$TOUT" "$dt" "$TESTE" "$EXTRA" >> "$RUN/placar.tsv"
    echo "  -> $VER ($DET) ${TURNS}t ${dt}s teste=$TESTE extra=$EXTRA"
  done
done
column -t -s$'\t' "$RUN/placar.tsv"
