package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/heliowap/delegador/internal/job"
	"github.com/heliowap/delegador/internal/testsupport"
)

func writeEvidence(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "evidence.jsonl")
	body := `{"kind":"trecho","ref":"pkg/svc/rota.go:42","text":"return nil"}` + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Sem TYPESAFE_API_KEY o plan nao pode fingir que aprovou: ele recusa,
// porque despachar sem gate e exatamente o que este plugin existe para evitar.
func TestPlanWithoutAPIKeyFailsLoudly(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	dir := t.TempDir()
	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{
		"plan", "--task", "corrigir x", "--evidence", writeEvidence(t, dir), "--worktree", dir,
	}, &out, &errBuf)

	if code != 1 {
		t.Fatalf("exit = %d, quero 1", code)
	}
	if !bytes.Contains(errBuf.Bytes(), []byte("TYPESAFE_API_KEY")) {
		t.Errorf("stderr nao explica a causa: %q", errBuf.String())
	}
}

func TestPlanRejectedTaskExits3WithJSON(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	srv := testsupport.StartFakeJev(t, map[string]float64{"desenho_em_aberto": 0.95})
	t.Setenv("TYPESAFE_API_KEY", "k")
	t.Setenv("TYPESAFE_BASE_URL", srv)

	dir := t.TempDir()
	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{
		"plan", "--task", "refatorar tudo", "--evidence", writeEvidence(t, dir), "--worktree", dir, "--json",
	}, &out, &errBuf)

	if code != 3 {
		t.Fatalf("exit = %d, quero 3 (reprovado)", code)
	}
	var v struct {
		Delegable bool     `json:"delegavel"`
		Missing   []string `json:"faltando"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("stdout nao e JSON: %v\n%s", err, out.String())
	}
	if v.Delegable {
		t.Error("delegavel deveria ser false")
	}
	if len(v.Missing) == 0 {
		t.Error("faltando deveria nomear o item reprovado")
	}
}

// Cobre a fiação nova do v2: roster.Load + Elegiveis + Classificar + Escolher
// escolhem o modelo, e o job persiste o que o run precisa para remontar o
// tools.Policy. O roster de teste tem um modelo sem benchmark: entra por
// viabilidade e custo, marcado nao_medido.
func TestPlanDelegableRoutesAndPersistsPolicy(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	srv := testsupport.StartFakeJev(t, nil)
	t.Setenv("TYPESAFE_API_KEY", "k")
	t.Setenv("TYPESAFE_BASE_URL", srv)

	dir := t.TempDir()
	rosterPath := filepath.Join(dir, "roster.yaml")
	rosterYAML := "as_of_sondagem: \"" + time.Now().Format("2006-01-02") + "\"\n" +
		"modelos:\n" +
		"  - id: modelo-x\n" +
		"    papel: barato\n" +
		"    sondado:\n" +
		"      tool_call: true\n" +
		"    humano:\n" +
		"      custo_usd_por_mtok: 0.5\n" +
		"      habilitado: true\n"
	if err := os.WriteFile(rosterPath, []byte(rosterYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{
		"plan", "--task", "corrigir Soma", "--evidence", writeEvidence(t, dir),
		"--worktree", dir, "--test-cmd", "go test ./...", "--roster", rosterPath, "--json",
	}, &out, &errBuf)

	if code != 0 {
		t.Fatalf("exit = %d, quero 0 (delegavel): %s", code, errBuf.String())
	}
	var v struct {
		Model     string `json:"modelo"`
		JobID     string `json:"job_id"`
		NaoMedido bool   `json:"modelo_nao_medido"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("stdout nao e JSON: %v\n%s", err, out.String())
	}
	if v.Model != "modelo-x" {
		t.Errorf("modelo = %q, quero modelo-x (unico elegivel)", v.Model)
	}
	if !v.NaoMedido {
		t.Error("modelo sem benchmark deveria sair marcado nao_medido")
	}

	j, err := job.Load(v.JobID)
	if err != nil {
		t.Fatalf("job.Load: %v", err)
	}
	if j.Model != "modelo-x" {
		t.Errorf("j.Model = %q", j.Model)
	}
	if len(j.AllowCommands) != 1 || j.AllowCommands[0] != "go test ./..." {
		t.Errorf("AllowCommands = %v, quero o comando declarado no briefing", j.AllowCommands)
	}
	if len(j.WritePrefixes) != 1 || j.WritePrefixes[0] != "" {
		t.Errorf("WritePrefixes = %v, quero a worktree inteira", j.WritePrefixes)
	}

	// O verify.Config do run sai do job, nao de flags repetidas.
	if j.TestCmd != "go test ./..." || j.SuiteCmd != "" || j.LintCmd != "" {
		t.Errorf("comandos persistidos = %q %q %q", j.TestCmd, j.SuiteCmd, j.LintCmd)
	}
	if !slices.Equal(j.TestGlobs, defaultTestGlobs) {
		t.Errorf("TestGlobs = %v, quero o default %v", j.TestGlobs, defaultTestGlobs)
	}

	// O registro da rota: a cascata re-roteia sem repreguntar ao Jev.
	score, niveis := 1.2, 3.0 // complexidade tem 4 criterios; o fake devolve 1.2
	if want := score / niveis; j.Percentil != want {
		t.Errorf("Percentil = %v, quero %v", j.Percentil, want)
	}
	if j.Dimensao != "mecanica" {
		t.Errorf("Dimensao = %q, quero mecanica (default do fake)", j.Dimensao)
	}
	if j.Autocontida != 0.9 {
		t.Errorf("Autocontida = %v, quero 0.9 (default do fake)", j.Autocontida)
	}
}
