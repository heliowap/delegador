package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoComArquivoNovo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-qm", "base")
	if err := os.WriteFile(filepath.Join(dir, "novo.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func staged(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// Medido em 2026-09-21: o add -AN sem desfazer deixava arquivo novo em
// intent-to-add. Nesse estado `git clean` nao remove e `git checkout -- .`
// TRUNCA para zero byte — foi assim que uma tarefa herdou dois arquivos
// vazios e falhou por um motivo que nao era dela.
func TestDiffNaoDeixaOIndiceSujo(t *testing.T) {
	dir := repoComArquivoNovo(t)
	d, err := Diff(context.Background(), dir)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(d, "novo.go") {
		t.Error("o arquivo novo precisa aparecer no diff")
	}
	if s := staged(t, dir); s != "" {
		t.Errorf("indice ficou sujo depois do Diff: %q", s)
	}
}

func TestDiffStatNaoDeixaOIndiceSujo(t *testing.T) {
	dir := repoComArquivoNovo(t)
	if _, _, files, err := DiffStat(context.Background(), dir); err != nil || files == 0 {
		t.Fatalf("DiffStat: files=%d err=%v", files, err)
	}
	if s := staged(t, dir); s != "" {
		t.Errorf("indice ficou sujo depois do DiffStat: %q", s)
	}
}

// Trabalho ja staged e de outra pessoa: nao se desfaz.
func TestDiffPreservaIndiceQueJaEstavaOcupado(t *testing.T) {
	dir := repoComArquivoNovo(t)
	cmd := exec.Command("git", "add", "novo.go")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if _, err := Diff(context.Background(), dir); err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if s := staged(t, dir); !strings.Contains(s, "novo.go") {
		t.Errorf("o staged de outra pessoa foi desfeito: %q", s)
	}
}
