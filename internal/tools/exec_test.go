package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunDeniedCallReturnsErrorResultNotGoError(t *testing.T) {
	p := policy(t)
	r := (&Registry{}).Run(context.Background(),
		Call{Name: "write_file", Args: map[string]string{"path": "infra/x.yaml", "content": "x"}}, p)

	if !r.IsError {
		t.Fatal("negacao deveria virar Result com IsError")
	}
	if r.Output == "" {
		t.Error("o motivo precisa chegar ao modelo, para ele tentar outro caminho")
	}
	if _, err := os.Stat(filepath.Join(p.Worktree, "infra/x.yaml")); err == nil {
		t.Error("o arquivo negado foi escrito mesmo assim")
	}
}

func TestWriteThenReadRoundTrip(t *testing.T) {
	p := policy(t)
	reg := &Registry{}
	ctx := context.Background()

	if r := reg.Run(ctx, Call{Name: "write_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "content": "package svc\n"}}, p); r.IsError {
		t.Fatalf("write falhou: %s", r.Output)
	}
	r := reg.Run(ctx, Call{Name: "read_file", Args: map[string]string{"path": "pkg/svc/rota.go"}}, p)
	if r.IsError || !strings.Contains(r.Output, "package svc") {
		t.Errorf("read: %+v", r)
	}
}

func TestEditReplacesExactlyOnce(t *testing.T) {
	p := policy(t)
	reg := &Registry{}
	ctx := context.Background()
	reg.Run(ctx, Call{Name: "write_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "content": "a := 1\nb := 1\n"}}, p)

	r := reg.Run(ctx, Call{Name: "edit_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "old": "a := 1", "new": "a := 2"}}, p)
	if r.IsError {
		t.Fatalf("edit falhou: %s", r.Output)
	}
	got := reg.Run(ctx, Call{Name: "read_file", Args: map[string]string{"path": "pkg/svc/rota.go"}}, p)
	if got.Output != "a := 2\nb := 1\n" {
		t.Errorf("conteudo = %q", got.Output)
	}
}

// Substituicao ambigua e erro: silenciosamente trocar a primeira ocorrencia
// e como um modelo corrompe arquivo sem ninguem perceber.
func TestEditRefusesAmbiguousMatch(t *testing.T) {
	p := policy(t)
	reg := &Registry{}
	ctx := context.Background()
	reg.Run(ctx, Call{Name: "write_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "content": "x := 1\nx := 1\n"}}, p)

	r := reg.Run(ctx, Call{Name: "edit_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "old": "x := 1", "new": "x := 2"}}, p)
	if !r.IsError {
		t.Error("duas ocorrencias deveriam ser erro, nao escolha da primeira")
	}
}

func TestEditRefusesNoMatch(t *testing.T) {
	p := policy(t)
	reg := &Registry{}
	ctx := context.Background()
	reg.Run(ctx, Call{Name: "write_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "content": "a := 1\n"}}, p)

	if r := reg.Run(ctx, Call{Name: "edit_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "old": "nao existe", "new": "y"}}, p); !r.IsError {
		t.Error("ausencia de match deveria ser erro")
	}
}

func TestExecCapturesOutputAndExitCode(t *testing.T) {
	p := policy(t)
	p.AllowCommands = append(p.AllowCommands, "go version")
	r := (&Registry{}).Run(context.Background(),
		Call{Name: "exec", Args: map[string]string{"command": "go version"}}, p)
	if r.IsError || !strings.Contains(r.Output, "go1.") {
		t.Errorf("exec: %+v", r)
	}
}

// Comando allowlistado travado nao pode segurar o laco para sempre: o
// teto por invocacao corta e devolve o timeout como erro de ferramenta.
func TestExecTimeoutReturnsErrorResult(t *testing.T) {
	p := policy(t)
	p.AllowCommands = append(p.AllowCommands, "sleep")
	reg := &Registry{ExecTimeout: 100 * time.Millisecond}
	r := reg.Run(context.Background(),
		Call{Name: "exec", Args: map[string]string{"command": "sleep 5"}}, p)
	if !r.IsError {
		t.Error("comando pendurado tinha que voltar erro")
	}
	if !strings.Contains(r.Output, "tempo limite") {
		t.Errorf("o modelo precisa saber que foi timeout: %q", r.Output)
	}
}

// O filho roda codigo escrito por modelo: o ambiente dele nao carrega as
// chaves que o pai usa para assinar requisicao.
func TestExecEnvDoesNotLeakKeys(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "segredo-de-teste")
	p := policy(t)
	p.AllowCommands = append(p.AllowCommands, "env")
	r := (&Registry{}).Run(context.Background(),
		Call{Name: "exec", Args: map[string]string{"command": "env"}}, p)
	if r.IsError {
		t.Fatalf("env: %s", r.Output)
	}
	if strings.Contains(r.Output, "TYPESAFE_API_KEY") || strings.Contains(r.Output, "segredo-de-teste") {
		t.Error("o ambiente do filho vazou a chave do pai")
	}
}

func TestSchemasAreOpenAIShaped(t *testing.T) {
	schemas := (&Registry{}).Schemas()
	if len(schemas) < 6 {
		t.Fatalf("quero ao menos 6 ferramentas, tenho %d", len(schemas))
	}
	for _, s := range schemas {
		if s["type"] != "function" {
			t.Errorf("type = %v, quero function", s["type"])
		}
		fn, ok := s["function"].(map[string]any)
		if !ok || fn["name"] == "" || fn["description"] == "" || fn["parameters"] == nil {
			t.Errorf("schema incompleto: %v", s)
		}
	}
}
