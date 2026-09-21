// runlock_test.go — a run.lock separa instancias do mesmo job: recusa um
// run concorrente, e recupera o job cujo executor morreu sem terminar.
package cli

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/job"
)

// pidMorto e um pid que nao pode existir: acima do pid_max de darwin e do
// default do linux, kill(pid, 0) devolve ESRCH com certeza — um pid morto
// de verdade, sem depender de sorte com processo recem-enterrado.
const pidMorto = 2000000000

// Executor morto no meio do run: o job ficou "running" com uma run.lock
// de pid que nao existe mais. O run marca failed e retoma — o caminho de
// recuperacao, nao um estado que trava para sempre.
func TestRunRecuperaJobRunningComPidMorto(t *testing.T) {
	env := setupRunEnv(t, cenarioQueEscreveTesteECorrige())

	j, err := job.Load(env.JobID)
	if err != nil {
		t.Fatal(err)
	}
	j.State = job.StateRunning
	j.PID = pidMorto
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.Path("run.lock"), []byte(strconv.Itoa(pidMorto)), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	if code := Run(context.Background(), []string{"run", "--job", env.JobID}, &out, &errBuf); code != 0 {
		t.Fatalf("a retomada do zumbi tinha que completar: exit %d: %s", code, errBuf.String())
	}

	depois, err := job.Load(env.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if !depois.State.Terminal() {
		t.Errorf("State = %q, quero estado terminal depois da retomada", depois.State)
	}
	if _, err := os.Stat(depois.Path("run.lock")); !os.IsNotExist(err) {
		t.Error("a run.lock tinha que sair no fim do run")
	}
}

// Trava viva de verdade — o pid do proprio teste — barra o segundo run,
// com o estado ainda planned: a recusa vem da run.lock, nao do state.
func TestRunRecusaComRunLockViva(t *testing.T) {
	env := setupRunEnv(t, cenarioQueEscreveTesteECorrige())

	j, err := job.Load(env.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.Path("run.lock"), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{"run", "--job", env.JobID}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("run concorrente tinha que ser recusado: exit %d", code)
	}
	if !strings.Contains(errBuf.String(), "ativo") {
		t.Errorf("o stderr tinha que nomear o run ativo: %q", errBuf.String())
	}
}

// Job running COM trava viva recusa na primeira checagem — sem sequer
// tocar o estado, porque o executor dono da trava esta trabalhando.
func TestRunRecusaJobRunningComTravaViva(t *testing.T) {
	env := setupRunEnv(t, cenarioQueEscreveTesteECorrige())

	j, err := job.Load(env.JobID)
	if err != nil {
		t.Fatal(err)
	}
	j.State = job.StateRunning
	j.PID = os.Getpid()
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.Path("run.lock"), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{"run", "--job", env.JobID}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("job running com executor vivo tinha que recusar: exit %d", code)
	}
	if !strings.Contains(errBuf.String(), "ativo") {
		t.Errorf("o stderr tinha que nomear o run ativo: %q", errBuf.String())
	}
	depois, err := job.Load(env.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if depois.State != job.StateRunning {
		t.Errorf("a recusa nao pode mexer no estado do run ativo: %q", depois.State)
	}
}

// Job orfao: o plan criou o registro mas morreu antes de rotear — sem
// modelo o run nao executa e diz por que.
func TestRunRecusaJobSemModelo(t *testing.T) {
	env := setupRunEnv(t, cenarioQueEscreveTesteECorrige())

	j, err := job.Load(env.JobID)
	if err != nil {
		t.Fatal(err)
	}
	j.Model = ""
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{"run", "--job", env.JobID}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("job sem modelo tinha que falhar: exit %d", code)
	}
	if !strings.Contains(errBuf.String(), "sem modelo") {
		t.Errorf("o erro tinha que nomear a causa: %q", errBuf.String())
	}
}
