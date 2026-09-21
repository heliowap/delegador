package job

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootHonorsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-teste")
	got, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/xdg-teste/delegador/jobs" {
		t.Errorf("Root() = %q", got)
	}
}

func TestCreateAndLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create exige diretorio real: a worktree e canonizada na entrada.
	j, err := Create(t.TempDir())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if j.State != StatePlanned {
		t.Errorf("State = %q, quero %q", j.State, StatePlanned)
	}
	j.Model = "swe-2-max"
	j.Permission = "smart"
	j.State = StateRunning
	if err := j.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(j.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Model != "swe-2-max" || got.Permission != "smart" || got.State != StateRunning {
		t.Errorf("round-trip perdeu campos: %+v", got)
	}
}

// Dois jobs na mesma worktree e exatamente o que o protocolo proibe:
// "nunca aponte dois agentes para a mesma pasta".
func TestCreateRefusesSecondJobOnSameWorktree(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	wt := t.TempDir()

	if _, err := Create(wt); err != nil {
		t.Fatalf("primeiro Create: %v", err)
	}
	if _, err := Create(wt); err == nil {
		t.Fatal("quero erro no segundo Create da mesma worktree")
	}
}

func TestReleaseAllowsReuse(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	wt := t.TempDir()

	j, err := Create(wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := Release(j.ID); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := Create(wt); err != nil {
		t.Fatalf("Create apos Release: %v", err)
	}
}

func TestPathIsInsideJobDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	j, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := j.Path("briefing.md")
	if filepath.Dir(p) != j.Dir() {
		t.Errorf("Path saiu do diretorio do job: %q", p)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Errorf("o diretorio do job nao existe: %v", err)
	}
}
