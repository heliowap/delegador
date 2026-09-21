package job

import (
	"os"
	"strings"
	"testing"
)

// A trava e do job, nao da worktree: Reacquire na propria trava e
// idempotente, depois de solta recria, e com trava alheia falha
// nomeando o dono.
func TestReacquireIdempotentRecriaERecusaAlheia(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	wt := t.TempDir() // Create exige diretorio existente

	j1, err := Create(wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := j1.Reacquire(); err != nil {
		t.Fatalf("Reacquire na propria trava deveria ser idempotente: %v", err)
	}
	if err := Release(j1.ID); err != nil {
		t.Fatal(err)
	}
	if err := j1.Reacquire(); err != nil {
		t.Fatalf("Reacquire devia recriar a trava solta: %v", err)
	}

	// Outro job toma a worktree: Reacquire do j1 falha nomeando o dono.
	if err := Release(j1.ID); err != nil {
		t.Fatal(err)
	}
	j2, err := Create(wt)
	if err != nil {
		t.Fatal(err)
	}
	err = j1.Reacquire()
	if err == nil {
		t.Fatal("Reacquire com trava alheia tinha que falhar")
	}
	if !strings.Contains(err.Error(), j2.ID) {
		t.Errorf("o erro nao nomeia o dono da trava: %v", err)
	}
}

// Release so derruba a propria trava: com a trava de outro job no lugar,
// soltar e no-op — a trava alheia fica intacta.
func TestReleaseNaoDerrubaTravaAlheia(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	wt := t.TempDir() // Create exige diretorio existente

	j1, err := Create(wt)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	if err := Release(j1.ID); err != nil {
		t.Fatal(err)
	}
	j2, err := Create(wt)
	if err != nil {
		t.Fatal(err)
	}

	if err := Release(j1.ID); err != nil {
		t.Fatalf("Release sem trava propria e no-op, nao erro: %v", err)
	}
	owner, err := os.ReadFile(lockPath(root, wt))
	if err != nil {
		t.Fatalf("a trava do j2 nao podia ter sumido: %v", err)
	}
	if string(owner) != j2.ID {
		t.Errorf("dono da trava = %q, quero %q", owner, j2.ID)
	}
}
