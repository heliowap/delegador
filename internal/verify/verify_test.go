package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// repoWithFix monta um repo git de verdade com uma correcao e um teste.
// provesTheFix decide se o teste realmente depende da correcao.
func repoWithFix(t *testing.T, provesTheFix bool) string {
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

	write("go.mod", "module exemplo\n\ngo 1.27\n")
	write("soma.go", "package exemplo\n\nfunc Soma(a, b int) int { return a - b }\n") // defeito
	git("init", "-q")
	git("add", ".")
	git("commit", "-qm", "base")

	// A correcao.
	write("soma.go", "package exemplo\n\nfunc Soma(a, b int) int { return a + b }\n")

	if provesTheFix {
		write("soma_test.go", "package exemplo\n\nimport \"testing\"\n\nfunc TestSoma(t *testing.T) {\n\tif Soma(2, 3) != 5 {\n\t\tt.Fatal(\"quero 5\")\n\t}\n}\n")
	} else {
		// Teste que passa com ou sem a correcao: nao prova nada.
		write("soma_test.go", "package exemplo\n\nimport \"testing\"\n\nfunc TestSoma(t *testing.T) {\n\tif Soma(0, 0) != 0 {\n\t\tt.Fatal(\"quero 0\")\n\t}\n}\n")
	}
	return dir
}

func cfg() Config {
	return Config{
		TestCmd:   "go test ./...",
		TestGlobs: []string{"*_test.go"},
		Timeout:   60 * time.Second,
	}
}

func TestRunProvesMutationWhenTestDependsOnFix(t *testing.T) {
	rep, err := Run(context.Background(), repoWithFix(t, true), cfg())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.MutationProved {
		t.Error("o teste depende da correcao; a mutacao deveria ficar vermelha")
	}
	if !rep.Green() {
		t.Errorf("a suite deveria estar verde: %+v", rep.Steps)
	}
}

// Este e o caso que o protocolo manda pegar: teste que passa mesmo sem a
// correcao. Foi assim que se descobriu um teste que fixava a funcao errada.
func TestRunFlagsTestThatProvesNothing(t *testing.T) {
	rep, err := Run(context.Background(), repoWithFix(t, false), cfg())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.MutationProved {
		t.Error("o teste passa sem a correcao; a mutacao NAO deveria provar nada")
	}
}

func TestRunSkipsStepsWithoutCommand(t *testing.T) {
	c := cfg()
	c.LintCmd = ""
	rep, err := Run(context.Background(), repoWithFix(t, true), c)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, s := range rep.Steps {
		if s.Name == "lint" && !s.Skipped {
			t.Error("lint sem comando deveria ficar marcado como pulado, nao inventado")
		}
	}
}

func TestRunCapturesDiff(t *testing.T) {
	rep, err := Run(context.Background(), repoWithFix(t, true), cfg())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Files == 0 {
		t.Error("a verificacao precisa capturar o diff")
	}
}
