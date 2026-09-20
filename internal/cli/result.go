// result.go — imprime o result.txt do job, verbatim. O arquivo é render do
// run (spec §6.6); ausente, o subcomando diz por quê em vez de devolver um
// erro seco — "não rodou ainda" e "falhou antes de renderizar" são respostas
// diferentes para quem lê.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/heliowap/delegador/internal/job"
)

func runResult(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("result", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jobID := fs.String("job", "", "id do job")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 || *jobID == "" {
		fmt.Fprintln(stderr, "result: uso: result --job <id>")
		return ExitUsage
	}

	j, err := job.Load(*jobID)
	if err != nil {
		fmt.Fprintf(stderr, "result: %v\n", err)
		return 1
	}

	raw, err := os.ReadFile(j.Path("result.txt"))
	if err == nil {
		_, _ = stdout.Write(raw)
		return 0
	}
	if !os.IsNotExist(err) {
		fmt.Fprintf(stderr, "result: lendo result.txt: %v\n", err)
		return 1
	}

	// Sem arquivo: o motivo depende do estado. Num job vivo e normal;
	// num job terminal e sinal de que o run morreu antes do render —
	// dai o exit 1.
	switch j.State {
	case job.StatePlanned:
		fmt.Fprintf(stdout, "job %s ainda nao rodou. O relatorio nasce do run:\n  delegador run --job %s\n", j.ID, j.ID)
		return 0
	case job.StateRunning:
		fmt.Fprintf(stdout, "job %s esta em execucao; result.txt e gravado no fim do run.\n", j.ID)
		return 0
	default:
		fmt.Fprintf(stdout, "job %s esta %s mas nao ha result.txt — o run falhou antes de renderizar o relatorio (ou o arquivo foi removido).\n", j.ID, j.State)
		return 1
	}
}
