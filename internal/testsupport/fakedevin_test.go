package testsupport

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFakeDevinWritesExportIncrementally(t *testing.T) {
	InstallFakeDevin(t, "healthy")

	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	prompt := filepath.Join(dir, "briefing.md")
	if err := os.WriteFile(prompt, []byte("corrija o defeito"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("devin", "--prompt-file", prompt, "--export", export, "-p")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fakedevin falhou: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(export)
	if err != nil {
		t.Fatalf("export nao foi escrito: %v", err)
	}
	var doc struct {
		Turns []struct {
			Index int `json:"index"`
			Tools []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("export nao e JSON valido: %v", err)
	}
	if len(doc.Turns) < 3 {
		t.Fatalf("quero ao menos 3 turnos no cenario healthy, tenho %d", len(doc.Turns))
	}
}

func TestFakeDevinRespectsScenario(t *testing.T) {
	InstallFakeDevin(t, "permission-block")

	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	prompt := filepath.Join(dir, "b.md")
	if err := os.WriteFile(prompt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// No cenario permission-block o processo nao termina sozinho: ele trava.
	// O teste so confere que ele chega a escrever o turno de bloqueio.
	cmd := exec.Command("devin", "--prompt-file", prompt, "--export", export, "-p")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	waitForFile(t, export, "aguardando confirmacao")
}
