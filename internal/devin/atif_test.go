package devin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heliowap/devin-plugin-cc/internal/testsupport"
)

// O parser e exercitado contra a saida do fakedevin, nao contra um JSON
// escrito a mao: assim, se o formato do produtor mudar, o teste acusa.
func TestParseATIFReadsFakeDevinExport(t *testing.T) {
	testsupport.InstallFakeDevin(t, "healthy")

	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	prompt := filepath.Join(dir, "b.md")
	if err := os.WriteFile(prompt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("devin", "--prompt-file", prompt, "--export", export, "-p").CombinedOutput(); err != nil {
		t.Fatalf("fakedevin: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(export)
	if err != nil {
		t.Fatal(err)
	}
	turns, err := ParseATIF(raw)
	if err != nil {
		t.Fatalf("ParseATIF: %v", err)
	}
	if len(turns) != 5 {
		t.Fatalf("len(turns) = %d, quero 5", len(turns))
	}
	if turns[2].Tools[0].Output != "FAIL: TestRota" {
		t.Errorf("turno 2 nao trouxe o vermelho: %+v", turns[2].Tools[0])
	}
	if !strings.Contains(turns[2].Summary(), "go test ./pkg/svc/") {
		t.Errorf("Summary nao traz o comando: %q", turns[2].Summary())
	}
}

func TestParseATIFToleratesUnknownFields(t *testing.T) {
	raw := []byte(`{"schema":"atif/2","turns":[
		{"index":0,"text":"oi","novidade":{"a":1},"tools":[
			{"id":"t0","name":"read","input":"f.go","output":"...","status":"ok","extra":true}
		]}
	]}`)
	turns, err := ParseATIF(raw)
	if err != nil {
		t.Fatalf("ParseATIF: %v", err)
	}
	if len(turns) != 1 || turns[0].Tools[0].ID != "t0" {
		t.Errorf("campos desconhecidos quebraram o parse: %+v", turns)
	}
}

func TestParseATIFErrorsOnInvalidJSON(t *testing.T) {
	if _, err := ParseATIF([]byte("{{{")); err == nil {
		t.Fatal("quero erro em JSON invalido")
	}
}

// O export e escrito atomicamente pelo devin, mas um leitor pode pegar um
// arquivo vazio entre o create e o rename. Isso nao e erro: e "ainda nao".
func TestParseATIFEmptyReturnsNoTurns(t *testing.T) {
	turns, err := ParseATIF(nil)
	if err != nil {
		t.Fatalf("ParseATIF(nil): %v", err)
	}
	if len(turns) != 0 {
		t.Errorf("quero zero turnos, tenho %d", len(turns))
	}
}
