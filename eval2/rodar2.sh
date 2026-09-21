#!/bin/bash
# Roda a cadeia 12->14 de tarefas nuas para um modelo, pelo proxy, usando o
# proprio delegador. Os testes vem do plano e ficam no disco antes: o juiz
# nunca e o teste que o modelo escreveu.
set -u
ARM="$1"                      # glm | swe2
WT="$HOME/VSCode/eval2-$ARM"
RUN="/tmp/eval2/$ARM"
PLANO="$HOME/VSCode/delegador/docs/superpowers/plans/2026-09-20-devin-plugin-cc.md"
D="$HOME/VSCode/delegador-v2/bin/delegador"
ROSTER="/tmp/eval2/roster-$ARM.yaml"

export DELEGADOR_BASE_URL=http://127.0.0.1:8317/v1
export DELEGADOR_API_KEY=$(grep -A1 "^api-keys:" "$HOME/VSCode/Lab/CLIProxyAPI/config.yaml" | tail -1 | sed -E 's/^[[:space:]]*-[[:space:]]*"?([^"[:space:]]+)"?.*/\1/')
export TYPESAFE_API_KEY=$(grep -E "^export TYPESAFE_API_KEY" "$HOME/.zshrc" | head -1 | sed "s/.*=//" | tr -d "\"' ")
export XDG_STATE_HOME="$RUN/state"

mkdir -p "$RUN"
: > "$RUN/placar.tsv"
printf 'tarefa\tveredito\tturnos\tcusto_exec\tcusto_jev\tseg\tdetalhe\n' >> "$RUN/placar.tsv"

for N in 12 13 14; do
  echo "[$(date +%H:%M:%S)] $ARM tarefa $N: preparando"
  # git reset ANTES do clean: o gitx.Diff do delegador roda `git add -AN`
  # e deixa arquivo novo em intent-to-add. Nesse estado, `git clean` nao o
  # remove e `git checkout -- .` o TRUNCA para zero byte — foi assim que a
  # tarefa 14 herdou dois arquivos vazios da execucao anterior.
  ( cd "$WT" && git reset -q && git clean -fdq && git checkout -q -- . 2>/dev/null )
  python3 "$HOME/VSCode/delegador/eval2/preparar.py" "$PLANO" "$N" "$WT" "$RUN/t$N" >"$RUN/t$N.prep" 2>&1 || {
    printf '%s\tPREP_FALHOU\t-\t-\t-\t-\t-\n' "$N" >> "$RUN/placar.tsv"; continue; }

  t0=$(date +%s)
  # exit 3 = gate reprovou (com `faltando`); exit 1 = erro de execucao,
  # tipicamente o Jev fora do ar. Confundir os dois faz culpar o modelo por
  # uma queda de servico de terceiro — aconteceu em 2026-09-21, 503 da
  # TypeSafe derrubou um braco inteiro rotulado como GATE_REPROVOU.
  JSON=$("$D" plan --task "$(cat "$RUN/t$N/tarefa.txt")" --evidence "$RUN/t$N/evidencia.jsonl" \
      --worktree "$WT" --roster "$ROSTER" \
      --test-cmd "$(sed -n 1p "$RUN/t$N/cmds.txt")" \
      --suite-cmd "$(sed -n 2p "$RUN/t$N/cmds.txt")" \
      --lint-cmd "$(sed -n 3p "$RUN/t$N/cmds.txt")" \
      --test-glob "*_test.go" --json 2>"$RUN/t$N.planerr")
  RC_PLAN=$?
  if [ "$RC_PLAN" -eq 1 ]; then
    MOTIVO=$(head -1 "$RUN/t$N.planerr" | cut -c1-90)
    echo "[$(date +%H:%M:%S)] $ARM tarefa $N: ERRO_PLAN — $MOTIVO"
    echo "[$(date +%H:%M:%S)] $ARM: esperando 120s e tentando esta tarefa mais uma vez"
    sleep 120
    JSON=$("$D" plan --task "$(cat "$RUN/t$N/tarefa.txt")" --evidence "$RUN/t$N/evidencia.jsonl" \
        --worktree "$WT" --roster "$ROSTER" \
        --test-cmd "$(sed -n 1p "$RUN/t$N/cmds.txt")" \
        --suite-cmd "$(sed -n 2p "$RUN/t$N/cmds.txt")" \
        --lint-cmd "$(sed -n 3p "$RUN/t$N/cmds.txt")" \
        --test-glob "*_test.go" --json 2>>"$RUN/t$N.planerr")
    RC_PLAN=$?
    if [ "$RC_PLAN" -eq 1 ]; then
      printf '%s\tERRO_PLAN\t-\t-\t-\t-\t%s\n' "$N" "$MOTIVO" >> "$RUN/placar.tsv"
      echo "[$(date +%H:%M:%S)] $ARM: erro persiste; parando o braco em vez de fabricar reprovacao"
      break
    fi
  fi
  JOB=$(printf '%s' "$JSON" | python3 -c "import json,sys;print(json.load(sys.stdin).get('job_id',''))" 2>/dev/null)
  TURNOS=$(printf '%s' "$JSON" | python3 -c "import json,sys;print(json.load(sys.stdin).get('max_turns',0))" 2>/dev/null)
  if [ -z "$JOB" ] || [ "${TURNOS:-0}" = "0" ]; then
    FALTA=$(printf '%s' "$JSON" | python3 -c "import json,sys;print(','.join(json.load(sys.stdin).get('faltando',[])))" 2>/dev/null)
    echo "[$(date +%H:%M:%S)] $ARM tarefa $N: GATE_REPROVOU $FALTA"
    printf '%s\tGATE_REPROVOU\t-\t-\t-\t-\t%s\n' "$N" "$FALTA" >> "$RUN/placar.tsv"
    continue
  fi

  echo "[$(date +%H:%M:%S)] $ARM tarefa $N: rodando ($JOB, $TURNOS turnos)"
  "$D" run --job "$JOB" --roster "$ROSTER" > "$RUN/t$N.result" 2>&1
  seg=$(( $(date +%s) - t0 ))

  CE=$(grep -oE 'executor \$[0-9.]+' "$RUN/t$N.result" | tail -1 | tr -d 'executor $')
  CJ=$(grep -oE 'jev \$[0-9.]+' "$RUN/t$N.result" | tail -1 | tr -d 'jev $')

  # juiz: teste do plano intocado E suite verde
  INTOCADO=$(python3 "$HOME/VSCode/delegador/eval2/conferir.py" "$PLANO" "$N" "$WT")
  if ( cd "$WT" && go vet ./... >/dev/null 2>&1 && go test ./... >/dev/null 2>&1 ); then VERDE=verde; else VERDE=vermelho; fi
  if [ "$INTOCADO" = "INTOCADO" ] && [ "$VERDE" = verde ]; then V=APROVADO; else V=REPROVADO; fi

  echo "[$(date +%H:%M:%S)] $ARM tarefa $N: $V ($VERDE, $INTOCADO, ${seg}s)"
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s/%s\n' "$N" "$V" "$TURNOS" "${CE:-?}" "${CJ:-?}" "$seg" "$VERDE" "$INTOCADO" >> "$RUN/placar.tsv"

  [ "$V" = APROVADO ] && ( cd "$WT" && git add -A && \
    git -c user.name=eval -c user.email=eval@local commit -qm "eval $ARM: tarefa $N" )
done
echo "[$(date +%H:%M:%S)] $ARM: fim"
