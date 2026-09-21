package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Numa linked worktree o `.git` e um ARQUIVO (ponteiro para o gitdir), nao
// um diretorio — e a sonda de mutacao copia a pasta com `cp -R`, ponteiro e
// tudo. Este teste trava a observacao empirica que sustenta a sonda: o git
// resolve a copia como raiz da propria worktree, entao o revert fica na
// copia e a correcao do original sobrevive intacta.
func TestRunProvesMutationOnLinkedWorktree(t *testing.T) {
	main := t.TempDir()
	git := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(dir, name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(main, "go.mod", "module exemplo\n\ngo 1.27\n")
	write(main, "soma.go", "package exemplo\n\nfunc Soma(a, b int) int { return a - b }\n") // defeito
	git(main, "init", "-q")
	git(main, "add", ".")
	git(main, "commit", "-qm", "base")

	linked := filepath.Join(t.TempDir(), "linked")
	git(main, "worktree", "add", "--detach", linked)

	// Na LINKED worktree: a correcao e o teste que depende dela.
	write(linked, "soma.go", "package exemplo\n\nfunc Soma(a, b int) int { return a + b }\n")
	write(linked, "soma_test.go", "package exemplo\n\nimport \"testing\"\n\nfunc TestSoma(t *testing.T) {\n\tif Soma(2, 3) != 5 {\n\t\tt.Fatal(\"quero 5\")\n\t}\n}\n")

	rep, err := Run(context.Background(), linked, cfg())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.MutationProved {
		t.Errorf("a mutacao tinha que provar numa linked worktree: %+v", rep.Steps)
	}
	if s, ok := rep.Step("teste"); !ok || s.ExitCode != 0 {
		t.Errorf("o passo de teste tinha que estar verde: %+v", rep.Steps)
	}

	// O revert ficou na copia: a correcao do original nao foi tocada.
	b, err := os.ReadFile(filepath.Join(linked, "soma.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "a + b") {
		t.Error("o revert vazou para a worktree original — a correcao sumiu")
	}
	if _, err := os.Stat(filepath.Join(linked, "soma_test.go")); err != nil {
		t.Error("o teste do original nao podia ter sumido")
	}
}
