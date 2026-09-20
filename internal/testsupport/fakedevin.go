package testsupport

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// InstallFakeDevin compila testdata/fakedevin, instala como "devin" num
// diretorio temporario a frente do PATH e devolve esse diretorio.
func InstallFakeDevin(t *testing.T, scenario string) string {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "devin")

	cmd := exec.Command("go", "build", "-o", bin, "./testdata/fakedevin")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build do fakedevin: %v\n%s", err, out)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKEDEVIN_SCENARIO", scenario)
	return dir
}

// repoRoot sobe a partir deste arquivo ate encontrar go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller falhou")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod nao encontrado acima de " + file)
		}
		dir = parent
	}
}

// waitForFile espera ate que path exista e contenha want, ou falha.
func waitForFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil && strings.Contains(string(raw), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout esperando %q em %s", want, path)
}
