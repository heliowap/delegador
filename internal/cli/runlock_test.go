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
	"time"

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

// A corrida que a graca cobre: um contender le a trava no intervalo entre
// o O_EXCL do dono e a gravacao do pid — arquivo existe mas esta vazio.
// Tratar vazio como morto removeria a trava do dono e duplicaria o
// executor. A releitura tem que ver o pid chegar e recusar, com a trava
// do dono intacta.
func TestAcquireTrataTravaVaziaComoFresca(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	j, err := job.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lock := j.Path("run.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// O dono termina a gravacao no meio da graca — o pid vivo do proprio
	// teste faz o papel do executor que esta nascendo.
	go func() {
		time.Sleep(runLockWriteGrace / 3)
		_ = os.WriteFile(lock, []byte(strconv.Itoa(os.Getpid())), 0o644)
	}()

	err = acquireRunLock(j)
	if err == nil {
		t.Fatal("trava que ganhou pid vivo durante a graca tinha que recusar")
	}
	if !strings.Contains(err.Error(), "ativo") {
		t.Errorf("a recusa tinha que nomear o run ativo: %v", err)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Error("a trava do dono nao pode ter sido removida pela releitura")
	}
}

// Vazio que ninguem completa e create abandonado: o dono morreu entre o
// O_EXCL e o pid. Depois da graca a trava conta como velha e a vaga e
// nossa — e a escolha documentada em runLockPIDComGraca.
func TestAcquireAssumeTravaVaziaAbandonada(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	j, err := job.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.Path("run.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := acquireRunLock(j); err != nil {
		t.Fatalf("create abandonado tinha que ceder a vaga: %v", err)
	}
	if pid := runLockPID(j); pid != os.Getpid() {
		t.Errorf("a trava nova tinha que carregar o nosso pid: %d", pid)
	}
}

// Soltar a trava confere o dono, como o Release da worktree: conteudo com
// pid alheio e a trava de outra instancia que tomou a vaga depois da nossa
// saida — remover apagaria a protecao dela e abriria a porta para um
// terceiro run.
func TestReleaseRunLockSoRemoveTravaPropria(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	j, err := job.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lock := j.Path("run.lock")

	// Trava nossa: sai.
	if err := os.WriteFile(lock, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	releaseRunLock(j)
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Error("a trava com o nosso pid tinha que ser removida")
	}

	// Trava alheia: fica intacta, byte a byte.
	if err := os.WriteFile(lock, []byte(strconv.Itoa(pidMorto)), 0o644); err != nil {
		t.Fatal(err)
	}
	releaseRunLock(j)
	b, err := os.ReadFile(lock)
	if err != nil || string(b) != strconv.Itoa(pidMorto) {
		t.Errorf("trava alheia nao pode ser tocada: %q %v", b, err)
	}
}
