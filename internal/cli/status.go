// status.go — leitura do estado de um job sem tocar nele: estado, modelo,
// escaladas, o veredito do verify mais recente, os dois custos (spec §9) e
// o bloco CANCELADO quando o watchdog cortou o laço.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/job"
	"github.com/heliowap/delegador/internal/ledger"
	"github.com/heliowap/delegador/internal/verify"
)

// ultimoVerify devolve o verify-N.json de maior N do job — cada tentativa
// deixa o seu, e o que interessa ao status é o mais recente.
func ultimoVerify(j *job.Job) (verify.Report, string, bool) {
	entries, err := os.ReadDir(j.Dir())
	if err != nil {
		return verify.Report{}, "", false
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "verify-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if k, err := strconv.Atoi(name[len("verify-") : len(name)-len(".json")]); err == nil && k > n {
			n = k
		}
	}
	if n == 0 {
		return verify.Report{}, "", false
	}
	nome := fmt.Sprintf("verify-%d.json", n)
	raw, err := os.ReadFile(j.Path(nome))
	if err != nil {
		return verify.Report{}, "", false
	}
	var rep verify.Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		return verify.Report{}, "", false
	}
	return rep, nome, true
}

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jobID := fs.String("job", "", "id do job")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 || *jobID == "" {
		fmt.Fprintln(stderr, "status: uso: status --job <id>")
		return ExitUsage
	}

	j, err := job.Load(*jobID)
	if err != nil {
		fmt.Fprintf(stderr, "status: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "job:       %s\n", j.ID)
	fmt.Fprintf(stdout, "estado:    %s\n", j.State)
	if j.Model != "" {
		fmt.Fprintf(stdout, "modelo:    %s\n", j.Model)
	}
	if j.Dimensao != "" {
		fmt.Fprintf(stdout, "rota:      dimensao %s, percentil %.2f\n", j.Dimensao, j.Percentil)
	}
	fmt.Fprintf(stdout, "escaladas: %d\n", j.Escaladas)
	fmt.Fprintf(stdout, "worktree:  %s\n", j.Worktree)

	if rep, nome, ok := ultimoVerify(j); ok {
		veredito := "vermelho"
		if rep.Green() {
			veredito = "verde"
		}
		fmt.Fprintf(stdout, "veredito:  %s (%s", veredito, nome)
		if rep.MutationProved {
			fmt.Fprint(stdout, ", mutacao provou")
		}
		fmt.Fprintln(stdout, ")")
		fmt.Fprintf(stdout, "  diff: %d arquivos, +%d -%d\n", rep.Files, rep.Added, rep.Removed)
		for _, s := range rep.Steps {
			if s.Skipped {
				fmt.Fprintf(stdout, "  %s: pulado\n", s.Name)
				continue
			}
			if s.ExpectFail && s.ExitCode != 0 {
				fmt.Fprintf(stdout, "  %s: exit %d (esperado)\n", s.Name, s.ExitCode)
				continue
			}
			fmt.Fprintf(stdout, "  %s: exit %d\n", s.Name, s.ExitCode)
		}
	} else {
		fmt.Fprintln(stdout, "veredito:  sem verificacao registrada")
	}

	// Os dois custos, separados como manda o spec §9: jev.jsonl cobra só a
	// entrada a $0.042/M, executor.jsonl cobra os dois lados.
	_, jevUSD, jevErr := (&jev.Ledger{Path: j.Path("jev.jsonl")}).Total()
	execUSD, execErr := (&ledger.Ledger{Path: j.Path("executor.jsonl")}).Total()
	switch {
	case jevErr != nil:
		fmt.Fprintf(stdout, "custo:     jev.jsonl ilegivel: %v\n", jevErr)
	case execErr != nil:
		fmt.Fprintf(stdout, "custo:     executor.jsonl ilegivel: %v\n", execErr)
	default:
		fmt.Fprintf(stdout, "custo:     executor $%.4f, jev $%.5f\n", execUSD, jevUSD)
	}

	if j.CancelReason != nil {
		cr := j.CancelReason
		fmt.Fprintf(stdout, "\nCANCELADO pelo watchdog: %s (noul %.2f)\n", cr.Signal, cr.Probability)
		if cr.TurnExcerpt != "" {
			fmt.Fprintf(stdout, "  trecho do turno: %s\n", cr.TurnExcerpt)
		}
		if cr.ResumeCommand != "" {
			fmt.Fprintf(stdout, "  retomar com:     %s\n", cr.ResumeCommand)
		}
	}
	return 0
}
