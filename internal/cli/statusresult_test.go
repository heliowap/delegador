// statusresult_test.go — os subcomandos de leitura: status monta o quadro
// do job (estado, modelo, veredito do verify mais recente, os dois custos,
// CANCELADO) e result imprime o result.txt ou explica a ausencia.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/heliowap/delegador/internal/job"
	"github.com/heliowap/delegador/internal/ledger"
	"github.com/heliowap/delegador/internal/verify"
)

// jobDeTeste cria um job no estado temporario, como o plan o deixaria.
func jobDeTeste(t *testing.T) *job.Job {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	j, err := job.Create(t.TempDir())
	if err != nil {
		t.Fatalf("job.Create: %v", err)
	}
	return j
}

func gravaVerify(t *testing.T, j *job.Job, n int, rep verify.Report) {
	t.Helper()
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	nome := "verify-" + strconv.Itoa(n) + ".json"
	if err := os.WriteFile(j.Path(nome), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatusMostraQuadroDoJob(t *testing.T) {
	j := jobDeTeste(t)
	j.Model = "barato"
	j.Dimensao = "mecanica"
	j.Escaladas = 1
	j.State = job.StateFailed
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}

	gravaVerify(t, j, 1, verify.Report{
		Added: 10, Removed: 2, Files: 2, MutationProved: true,
		Steps: []verify.Step{
			{Name: "teste", Command: "go test ./...", ExitCode: 0},
			{Name: "mutacao", ExitCode: 1, ExpectFail: true},
		},
	})
	// O verify-2 e o que conta: o status mostra o mais recente.
	gravaVerify(t, j, 2, verify.Report{
		Added: 12, Removed: 2, Files: 2,
		Steps: []verify.Step{{Name: "teste", ExitCode: 1}},
	})

	l := &ledger.Ledger{Path: j.Path("executor.jsonl")}
	if err := l.Record("turno", 1_000_000, 500_000, 0.10, 0.10); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := runStatus(context.Background(), []string{"--job", j.ID}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit = %d, quero 0\n%s", code, errBuf.String())
	}
	s := out.String()
	for _, querido := range []string{
		"estado:    failed", "modelo:    barato", "escaladas: 1",
		"veredito:  vermelho (verify-2.json", "executor $0.1500, jev $0.00000",
	} {
		if !strings.Contains(s, querido) {
			t.Errorf("faltou %q na saida:\n%s", querido, s)
		}
	}
}

func TestStatusMostraCancelReason(t *testing.T) {
	j := jobDeTeste(t)
	j.State = job.StateFailed
	j.CancelReason = &job.CancelReason{
		Signal: "sem_progresso", Probability: 0.91,
		TurnExcerpt:   "turno 6: tentando de novo",
		ResumeCommand: "delegador run --job " + j.ID,
		At:            time.Now(),
	}
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	if code := runStatus(context.Background(), []string{"--job", j.ID}, &out, &errBuf); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "CANCELADO pelo watchdog: sem_progresso") ||
		!strings.Contains(out.String(), "delegador run --job "+j.ID) {
		t.Errorf("o bloco CANCELADO nao apareceu:\n%s", out.String())
	}
}

func TestResultImprimeArquivoVerbatim(t *testing.T) {
	j := jobDeTeste(t)
	conteudo := "veredito: verde\n  diff: 2 arquivos, +10 -2\ncusto:    executor $0.01, jev $0.00010\n"
	if err := os.WriteFile(j.Path("result.txt"), []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := runResult(context.Background(), []string{"--job", j.ID}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if out.String() != conteudo {
		t.Errorf("saida nao e verbatim:\n%q", out.String())
	}
}

func TestResultSemArquivoExplicaPlanned(t *testing.T) {
	j := jobDeTeste(t) // planned, sem result.txt

	var out, errBuf bytes.Buffer
	code := runResult(context.Background(), []string{"--job", j.ID}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit = %d, quero 0 (job ainda nao rodou)", code)
	}
	if !strings.Contains(out.String(), "ainda nao rodou") || !strings.Contains(out.String(), "delegador run --job "+j.ID) {
		t.Errorf("a explicacao nao apareceu:\n%s", out.String())
	}
}

func TestResultSemArquivoEmJobTerminalErra(t *testing.T) {
	j := jobDeTeste(t)
	j.State = job.StateFailed
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := runResult(context.Background(), []string{"--job", j.ID}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit = %d, quero 1 (terminal sem result.txt)", code)
	}
	if !strings.Contains(out.String(), "falhou antes de renderizar") {
		t.Errorf("a explicacao nao apareceu:\n%s", out.String())
	}
}
