package job

import (
	"os"
	"path/filepath"
	"testing"
)

// A trava e da worktree, nao da grafia: duas escritas do mesmo diretorio
// tem que cair no mesmo lock, ou dois jobs dividem a pasta — o erro que o
// protocolo proibe. E o caminho gravado no job e o canonico, para a
// politica e o cmd.Dir do run falarem da mesma pasta que a trava.
func TestLockPathMesmaWorktreeDuasGrafias(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()

	base := lockPath(root, wt)

	// Symlink para a mesma pasta e a grafia que o EvalSymlinks resolve.
	link := filepath.Join(t.TempDir(), "atalho")
	if err := os.Symlink(wt, link); err != nil {
		t.Skip("symlink indisponivel neste sistema")
	}
	if viaLink := lockPath(root, link); viaLink != base {
		t.Errorf("mesma worktree via symlink deu outra trava: %s vs %s", viaLink, base)
	}

	irmao := t.TempDir()
	if outra := lockPath(root, irmao); outra == base {
		t.Error("worktrees diferentes nao podem dividir a mesma trava")
	}
}

// Create guarda a forma canonica: e o que a trava, o cmd.Dir das
// ferramentas e a politica de escrita comparam.
func TestCreateGuardaWorktreeCanonico(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	wt := t.TempDir()

	link := filepath.Join(t.TempDir(), "atalho")
	if err := os.Symlink(wt, link); err != nil {
		t.Skip("symlink indisponivel neste sistema")
	}
	j, err := Create(link)
	if err != nil {
		t.Fatalf("Create via symlink: %v", err)
	}
	canon, err := filepath.EvalSymlinks(wt)
	if err != nil {
		t.Fatal(err)
	}
	if j.Worktree != canon {
		t.Errorf("Worktree = %q, quero o canonico %q", j.Worktree, canon)
	}
	// A segunda grafia da mesma pasta colide na mesma trava.
	if _, err := Create(wt); err == nil {
		t.Error("segunda grafia da mesma worktree tinha que colidir na trava")
	}
}

// Worktree que nao existe nao vira job: a canonizacao falha antes da trava.
func TestCreateRecusaWorktreeInexistente(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if _, err := Create(filepath.Join(t.TempDir(), "fantasma")); err == nil {
		t.Error("diretorio inexistente tinha que falhar")
	}
	arquivo := filepath.Join(t.TempDir(), "arquivo")
	if err := os.WriteFile(arquivo, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(arquivo); err == nil {
		t.Error("arquivo nao e worktree; tinha que falhar")
	}
}
