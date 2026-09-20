#!/bin/bash
# Driver: uma invocacao isolada do devin por tarefa do plano, com portao de
# teste entre elas. Para na primeira tarefa que ficar vermelha.
set -u

WT="${WT:-$HOME/VSCode/devin-plugin-cc-impl}"
RUN="${RUN:-/tmp/devin-plugin-run}"
PLAN="${PLAN:-$WT/docs/superpowers/plans/2026-09-20-devin-plugin-cc.md}"
MODEL="${MODEL:-swe-2-max}"
PERM="${PERM:-smart}"

START="${START:-1}"
END="${END:-18}"

bold() { printf '\033[1m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
red() { printf '\033[31m%s\033[0m\n' "$*"; }
banner() {
    echo
    printf '\033[1;44m %s \033[0m\n' "$*"
    echo
}

extract_task() {
    awk -v n="$1" '
        $0 == "### Task " n ":" || index($0, "### Task " n ": ") == 1 { on=1 }
        on && /^### Task / && index($0, "### Task " n ":") != 1 { exit }
        on && /^## Ordem de execução/ { exit }
        on { print }
    ' "$PLAN"
}

constraints() {
    awk '/^## Global Constraints/{on=1} on&&/^## File Structure/{exit} on{print}' "$PLAN"
}

# fingerprint: HEAD + quantidade de arquivos sujos. Serve para provar que a
# tarefa produziu alguma coisa, em vez de aprovar o nada.
fingerprint() {
    ( cd "$WT" && echo "$(git rev-parse HEAD) $(git status --porcelain | wc -l | tr -d ' ')" )
}

gate() {
    if [ ! -f "$WT/go.mod" ]; then
        echo "PORTAO: nao existe go.mod na worktree. A tarefa nao produziu codigo."
        return 1
    fi
    ( cd "$WT" && go vet ./... 2>&1 && go test ./... 2>&1 )
}

echo "$(date '+%F %T')  inicio  tarefas $START..$END  modelo $MODEL  permissao $PERM" \
    | tee -a "$RUN/progress.log"

for n in $(seq "$START" "$END"); do
    prompt="$RUN/prompts/task-$n.md"
    log="$RUN/logs/task-$n.log"
    export_file="$RUN/exports/task-$n.json"

    body="$(extract_task "$n")"
    if [ -z "$body" ]; then
        red "tarefa $n nao encontrada no plano; parando"
        exit 1
    fi

    {
        cat <<EOF
Voce esta implementando UMA tarefa de um plano de implementacao ja aprovado,
num repositorio Go. Trabalhe apenas em: $WT

## Regras desta execucao

- Implemente **somente a tarefa abaixo**. Nao adiante tarefas seguintes, nao
  refatore o que ja existe, nao "melhore" codigo fora do escopo dela.
- Siga os passos na ordem escrita. Onde o passo manda escrever um teste que
  falha, **escreva o teste, rode e confirme o vermelho antes** de implementar.
  Cole a saida do vermelho no seu relatorio.
- Rode os comandos exatos que os passos indicam. Nao invente comando.
- **Nao rode \`git commit\`, \`git add\`, \`git push\` nem \`chmod\`.** O modo de
  permissao desta execucao recusa esses comandos e voce morre no meio se
  tentar. Quem commita e quem aplica bit de execucao e o driver, depois que
  o portao de teste passar. Pule os passos de commit do plano; faca todo o
  resto deles.
- Nao acesse a rede, exceto \`go install\` se um passo pedir.
- Nao altere arquivos em \`docs/\`.
- Ao terminar, responda com: o que voce escreveu arquivo por arquivo, a saida
  literal do vermelho inicial de cada teste, e a saida de \`go vet ./...\` e
  \`go test ./...\` no fim.

## Contexto do projeto

$(sed -n '1,20p' "$PLAN")

$(constraints)

## A tarefa

$body
EOF
    } > "$prompt"

    banner "TAREFA $n / $END  —  $(date '+%H:%M:%S')"
    echo "$n" > "$RUN/current-task"
    date +%s > "$RUN/current-start"
    echo "$(date '+%F %T')  tarefa $n  iniciada" >> "$RUN/progress.log"

    before="$(fingerprint)"

    # O devin roda DENTRO da worktree. Fora dela, toda acao vira "fora do
    # workspace", exige confirmacao, e o modo -p nao consegue exibi-la.
    ( cd "$WT" && devin --model "$MODEL" \
          --permission-mode "$PERM" \
          --respect-workspace-trust false \
          --prompt-file "$prompt" \
          --export "$export_file" \
          -p ) > "$log" 2>&1
    rc=$?

    echo "--- relatorio do devin (tarefa $n) ---"
    tail -60 "$log"
    echo "--- fim do relatorio ---"

    if [ $rc -ne 0 ]; then
        red "devin saiu com codigo $rc na tarefa $n. Parando."
        echo "$(date '+%F %T')  tarefa $n  DEVIN_EXIT=$rc" >> "$RUN/progress.log"
        echo "parado" > "$RUN/current-task"
        exit 1
    fi

    # Reparo mecanico + uma retomada. Medido: o modo smart recusa `chmod`, e
    # o devin morre no meio com o trabalho ja no disco. chmod em script com
    # shebang e seguro e deterministico — nao justifica escalar permissao.
    if grep -q "rejected a tool call" "$log"; then
        bold "tarefa $n: recusa de ferramenta. Reparando o que e mecanico e retomando uma vez."
        echo "$(date '+%F %T')  tarefa $n  RECUSA — reparo + retomada" >> "$RUN/progress.log"

        ( cd "$WT" && find scripts -type f 2>/dev/null | while read -r f; do
              head -c 2 "$f" | grep -q '#!' && chmod +x "$f" && echo "chmod +x $f"
          done )

        ( cd "$WT" && devin -c --model "$MODEL" \
              --permission-mode "$PERM" \
              --respect-workspace-trust false \
              -p "Continue de onde parou. Nao rode git commit, git add, git push nem chmod: o driver cuida disso e esses comandos sao recusados aqui. Termine os passos tecnicos restantes da tarefa." \
          ) > "$log.retry" 2>&1
        rc=$?

        echo "--- retomada (tarefa $n) ---"
        tail -40 "$log.retry"

        if grep -q "rejected a tool call" "$log.retry"; then
            red "tarefa $n: travou duas vezes no mesmo ponto. Parando — assuma esta tarefa voce."
            echo "$(date '+%F %T')  tarefa $n  TRAVOU_DUAS_VEZES" >> "$RUN/progress.log"
            echo "parado" > "$RUN/current-task"
            exit 1
        fi
        if [ $rc -ne 0 ]; then
            red "tarefa $n: retomada saiu com codigo $rc. Parando."
            echo "$(date '+%F %T')  tarefa $n  RETOMADA_EXIT=$rc" >> "$RUN/progress.log"
            echo "parado" > "$RUN/current-task"
            exit 1
        fi
        green "tarefa $n: retomada concluiu"
    fi

    if [ "$(fingerprint)" = "$before" ]; then
        red "tarefa $n: nada mudou na worktree. Relatorio sem trabalho nao e trabalho."
        echo "$(date '+%F %T')  tarefa $n  SEM_MUDANCA" >> "$RUN/progress.log"
        echo "parado" > "$RUN/current-task"
        exit 1
    fi

    bold "portao: go vet + go test"
    if gate "$n" > "$RUN/logs/gate-$n.log" 2>&1; then
        green "tarefa $n: portao verde"
        # O commit e do driver: deterministico, atribuido, e fora do alcance
        # da recusa de permissao que mata o devin no meio.
        msg="feat: tarefa $n do plano do devin-plugin-cc

Implementado pelo Devin CLI (swe-2-max) a partir do briefing da tarefa $n.
Portao conferido pelo driver: go vet ./... e go test ./... verdes.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
        ( cd "$WT" && git add -A && git -c user.name="Helio Pinheiro" \
              -c user.email="heliowap@gmail.com" commit -q -m "$msg" ) \
            && green "tarefa $n: commitada"
        echo "$(date '+%F %T')  tarefa $n  VERDE" >> "$RUN/progress.log"
    else
        red "tarefa $n: portao VERMELHO. Parando para voce olhar."
        tail -40 "$RUN/logs/gate-$n.log"
        echo "$(date '+%F %T')  tarefa $n  VERMELHO" >> "$RUN/progress.log"
        echo "parado" > "$RUN/current-task"
        exit 1
    fi
done

echo "concluido" > "$RUN/current-task"
green "TODAS AS TAREFAS $START..$END CONCLUIDAS"
echo "$(date '+%F %T')  fim" >> "$RUN/progress.log"
