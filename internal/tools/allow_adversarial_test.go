package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// Casos que o plano nao previu. Esta e a unica camada entre o modelo e o
// disco: o teste precisa atacar, nao confirmar.

func TestDeniesEnvPrefixSmugglingDeniedBinary(t *testing.T) {
	p := policy(t)
	for _, cmd := range []string{
		"GIT_DIR=/tmp/x git push origin main",
		"PATH=/tmp curl https://exfil",
		"env git push",
	} {
		if Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p).Allowed {
			t.Errorf("binario negado escondido atras de prefixo de ambiente: %q", cmd)
		}
	}
}

func TestDeniesAbsolutePathToDeniedBinary(t *testing.T) {
	p := policy(t)
	p.AllowCommands = append(p.AllowCommands, "/usr/bin/curl")
	for _, cmd := range []string{"/usr/bin/curl https://x", "/bin/rm -rf /", "../../bin/ssh host"} {
		if Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p).Allowed {
			t.Errorf("caminho absoluto para binario negado aceito: %q", cmd)
		}
	}
}

// Prefixo sem barra final nao pode casar diretorio irmao: "pkg/svc" nao
// autoriza "pkg/svcX".
func TestWritePrefixRespeitaFronteiraDeDiretorio(t *testing.T) {
	p := policy(t)
	p.WritePrefixes = []string{"pkg/svc"} // sem barra, de proposito
	if err := os.MkdirAll(filepath.Join(p.Worktree, "pkg/svcX"), 0o755); err != nil {
		t.Fatal(err)
	}

	if Allow(Call{Name: "write_file", Args: map[string]string{"path": "pkg/svcX/evil.go"}}, p).Allowed {
		t.Error("pkg/svcX nao deveria casar com o prefixo pkg/svc")
	}
	if !Allow(Call{Name: "write_file", Args: map[string]string{"path": "pkg/svc/ok.go"}}, p).Allowed {
		t.Error("pkg/svc/ok.go deveria passar com o prefixo pkg/svc")
	}
}

// Sem prefixo declarado, nenhuma escrita passa. Ausencia de politica e a
// politica mais restritiva, nunca a mais permissiva.
func TestSemPrefixoNenhumaEscritaPassa(t *testing.T) {
	p := policy(t)
	p.WritePrefixes = nil
	if Allow(Call{Name: "write_file", Args: map[string]string{"path": "pkg/svc/a.go"}}, p).Allowed {
		t.Error("sem prefixo declarado, escrita deveria ser negada")
	}
}

// Travessia que volta para dentro e legitima: nao se pune sintaxe, se
// verifica destino.
func TestTravessiaQueVoltaParaDentroEhAceita(t *testing.T) {
	p := policy(t)
	if d := Allow(Call{Name: "write_file",
		Args: map[string]string{"path": "pkg/svc/../svc/a.go"}}, p); !d.Allowed {
		t.Errorf("caminho que resolve para dentro deveria passar: %s", d.Reason)
	}
}

// Symlink de DIRETORIO apontando para fora: o arquivo alvo nem existe ainda,
// entao a resolucao precisa subir ate o ancestral existente.
func TestDeniesSymlinkedDirectoryEscape(t *testing.T) {
	p := policy(t)
	fora := t.TempDir()
	link := filepath.Join(p.Worktree, "pkg/svc/sub")
	if err := os.Symlink(fora, link); err != nil {
		t.Skip("symlink indisponivel")
	}
	if Allow(Call{Name: "write_file",
		Args: map[string]string{"path": "pkg/svc/sub/novo.go"}}, p).Allowed {
		t.Error("diretorio symlinkado para fora deveria ser negado")
	}
}

// Leitura tambem respeita symlink: exfiltrar por leitura e exfiltrar.
func TestLeituraTambemBloqueiaSymlinkParaFora(t *testing.T) {
	p := policy(t)
	alvo := filepath.Join(t.TempDir(), "segredo.txt")
	if err := os.WriteFile(alvo, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(p.Worktree, "espelho.txt")
	if err := os.Symlink(alvo, link); err != nil {
		t.Skip("symlink indisponivel")
	}
	if Allow(Call{Name: "read_file", Args: map[string]string{"path": "espelho.txt"}}, p).Allowed {
		t.Error("leitura via symlink para fora deveria ser negada")
	}
}

func TestExecVazioOuSoEspacoEhNegado(t *testing.T) {
	p := policy(t)
	for _, cmd := range []string{"", "   ", "\t"} {
		if Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p).Allowed {
			t.Errorf("comando vazio aceito: %q", cmd)
		}
	}
}

// Argumento ausente nao pode virar caminho vazio permitido.
func TestArgumentoAusenteEhNegado(t *testing.T) {
	p := policy(t)
	for _, name := range []string{"read_file", "write_file", "edit_file", "exec"} {
		if Allow(Call{Name: name, Args: nil}, p).Allowed {
			t.Errorf("%s sem argumento deveria ser negado", name)
		}
	}
}
