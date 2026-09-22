package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/heliowap/delegador/internal/job"
)

// runCancel encerra um job e devolve a worktree.
//
// Medido em 2026-09-21, no piloto de capacidade: um run morto por `pkill`
// nunca executou o `job.Release` diferido, e a worktree ficou travada para
// sempre. `plan` recusava com "ja esta em uso pelo job X", `run` so recupera
// quando o ESTADO e `running`, e nao havia comando nenhum que soltasse a
// trava — a saida era apagar arquivo de estado a mao.
//
// A trava e uma protecao real: dois jobs na mesma worktree se atropelam. O
// que faltava era a porta de saida, e ela confere o pid antes de abrir: run
// vivo continua dono da worktree, e cancelar por engano um trabalho em curso
// seria pior que a trava presa.
func runCancel(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jobID := fs.String("job", "", "id do job a encerrar")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 || *jobID == "" {
		fmt.Fprintln(stderr, "cancel: uso: cancel --job <id>")
		return ExitUsage
	}

	j, err := job.Load(*jobID)
	if err != nil {
		fmt.Fprintf(stderr, "cancel: %v\n", err)
		return 1
	}
	if pid := runLockPIDComGraca(j); pidVivo(pid) {
		fmt.Fprintf(stderr, "cancel: job %s tem run ativo (pid %d); pare o processo antes\n", j.ID, pid)
		return 1
	}
	if j.State.Terminal() {
		// Estado terminal ja devia ter soltado a trava; soltar de novo e
		// inofensivo e cobre o caso em que o Release anterior falhou.
		if err := job.Release(j.ID); err != nil {
			fmt.Fprintf(stderr, "cancel: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "job %s ja estava %s; worktree liberada\n", j.ID, j.State)
		return 0
	}

	j.State = job.StateCancelled
	j.PID = 0
	if err := j.Save(); err != nil {
		fmt.Fprintf(stderr, "cancel: %v\n", err)
		return 1
	}
	if err := job.Release(j.ID); err != nil {
		fmt.Fprintf(stderr, "cancel: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "job %s cancelado; worktree %s liberada\n", j.ID, j.Worktree)
	return 0
}
