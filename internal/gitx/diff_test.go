package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoDirty monta um repo git real com um commit base e, por cima, uma
// mudanca em arquivo de producao mais um arquivo de teste novo.
func repoDirty(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	write("soma.go", "package exemplo\n\nfunc Soma(a, b int) int { return a - b }\n")
	git("init", "-q")
	git("add", ".")
	git("commit", "-qm", "base")

	write("soma.go", "package exemplo\n\nfunc Soma(a, b int) int { return a + b }\n")
	write("soma_test.go", "package exemplo\n")
	return dir
}

func TestDiffCapturesNewAndModifiedFiles(t *testing.T) {
	d, err := Diff(context.Background(), repoDirty(t))
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(d, "return a + b") {
		t.Error("o diff precisa conter a correcao")
	}
	if !strings.Contains(d, "soma_test.go") {
		t.Error("o diff precisa incluir o arquivo novo (add -AN)")
	}
}

func TestDiffStatCountsLinesAndFiles(t *testing.T) {
	added, removed, files, err := DiffStat(context.Background(), repoDirty(t))
	if err != nil {
		t.Fatalf("DiffStat: %v", err)
	}
	if added == 0 || removed == 0 {
		t.Errorf("a correcao muda uma linha: added=%d removed=%d", added, removed)
	}
	if files != 2 {
		t.Errorf("files = %d, quero 2 (soma.go e soma_test.go)", files)
	}
}

// A mutacao depende disto: desfazer a correcao sem tocar no teste.
func TestRevertNonTestKeepsTestChanges(t *testing.T) {
	dir := repoDirty(t)
	if err := RevertNonTest(context.Background(), dir, []string{"*_test.go"}); err != nil {
		t.Fatalf("RevertNonTest: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "soma.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "return a - b") {
		t.Error("a correcao deveria ter sido desfeita")
	}
	if _, err := os.Stat(filepath.Join(dir, "soma_test.go")); err != nil {
		t.Error("o arquivo de teste nao pode sumir na reversao")
	}
}

// Arquivo novo de nao-teste sai DE VERDADE: `checkout --` restaura o blob
// vazio do intent-to-add e deixaria um stub de 0 bytes — um .go vazio
// quebra o build da tentativa seguinte e infla a sonda de mutacao.
func TestRevertNonTestDeletesNewFiles(t *testing.T) {
	dir := repoDirty(t)
	ctx := context.Background()

	// Marca o intent-to-add como o verify faz (gitx.Diff roda add -AN):
	// novo.go vira ` A` no status; soltinho.go, criado depois, fica `??`.
	// Os dois caminhos de arquivo novo tem que remover de verdade.
	if _, err := Diff(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "novo.go"), []byte("package exemplo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := output(ctx, dir, "add", "-AN"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "soltinho.go"), []byte("package exemplo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RevertNonTest(ctx, dir, []string{"*_test.go"}); err != nil {
		t.Fatalf("RevertNonTest: %v", err)
	}

	for _, name := range []string{"novo.go", "soltinho.go"} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err == nil {
			t.Errorf("%s ficou (%d bytes) — era para sumir, nao esvaziar", name, fi.Size())
		} else if !os.IsNotExist(err) {
			t.Errorf("%s: %v", name, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "soma.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "return a - b") {
		t.Error("a correcao deveria ter sido desfeita")
	}
	if _, err := os.Stat(filepath.Join(dir, "soma_test.go")); err != nil {
		t.Error("o arquivo de teste nao pode sumir na reversao")
	}
}
