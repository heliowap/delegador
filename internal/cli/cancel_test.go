package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/job"
)

// Medido em 2026-09-21: um run morto por `pkill` nunca executou o Release
// diferido, e a worktree ficou travada para sempre. `plan` recusava, `run`
// so recupera com estado `running`, e nao havia comando que soltasse.
func TestCancelLiberaAWorktree(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	wt := t.TempDir()

	j, err := job.Create(wt)
	if err != nil {
		t.Fatal(err)
	}
	// Com a trava na mao, um segundo job na mesma worktree e recusado.
	if _, err := job.Create(wt); err == nil {
		t.Fatal("a trava precisa impedir dois jobs na mesma worktree")
	}

	var out, errb bytes.Buffer
	if code := runCancel(context.Background(), []string{"--job", j.ID}, &out, &errb); code != 0 {
		t.Fatalf("cancel = %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), j.ID) {
		t.Errorf("a saida precisa nomear o job: %q", out.String())
	}

	// Liberada: a worktree aceita um job novo.
	if _, err := job.Create(wt); err != nil {
		t.Errorf("depois do cancel a worktree deveria aceitar: %v", err)
	}
	recarregado, err := job.Load(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recarregado.State != job.StateCancelled {
		t.Errorf("estado = %s, quero cancelled", recarregado.State)
	}
}

func TestCancelPrecisaDeJob(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runCancel(context.Background(), nil, &out, &errb); code != ExitUsage {
		t.Errorf("sem --job quero ExitUsage, tenho %d", code)
	}
}
