package tools

import "testing"

// Escrita em .git e negação dura: hook ou textconv plantado la executa
// durante o verify, e config alterada fica como superficie de ataque
// persistida. A regra e por segmento — cobre o .git da raiz, o arquivo
// .git de uma linked worktree e .git aninhado de submodulo.
func TestDeniesWritesInsideDotGit(t *testing.T) {
	p := policy(t)
	p.WritePrefixes = []string{""} // worktree inteira: isola a negacao de .git

	for _, path := range []string{
		".git/hooks/pre-commit",
		".git/config",
		".git",              // arquivo .git de linked worktree
		"pkg/.git/config",   // .git aninhado de submodulo
		"a/b/.git/hooks/x",  // idem, mais fundo
		"pkg/svc/../.git/x", // normaliza e ainda pega o segmento
	} {
		if d := Allow(Call{Name: "write_file", Args: map[string]string{"path": path}}, p); d.Allowed {
			t.Errorf("escrita em interno do git aceita: %q", path)
		}
		if d := Allow(Call{Name: "edit_file", Args: map[string]string{"path": path, "old": "a", "new": "b"}}, p); d.Allowed {
			t.Errorf("edicao em interno do git aceita: %q", path)
		}
	}
}

// O veto e cirurgico: nome parecido nao e .git, e leitura continua livre —
// ler config e objeto do git e contexto legitimo para o modelo.
func TestDotGitDenialDoesNotCatchLookalikes(t *testing.T) {
	p := policy(t)
	p.WritePrefixes = []string{""}

	for _, path := range []string{"pkg/svc/git.go", ".github/workflows/ci.yml", "x.gitignore", "dgit/.gitkeep"} {
		if d := Allow(Call{Name: "write_file", Args: map[string]string{"path": path}}, p); !d.Allowed {
			t.Errorf("escrita normal nao pode cair no veto de .git: %q — %s", path, d.Reason)
		}
	}
	for _, path := range []string{".git/config", ".git/HEAD"} {
		if d := Allow(Call{Name: "read_file", Args: map[string]string{"path": path}}, p); !d.Allowed {
			t.Errorf("leitura de .git deveria seguir livre: %q — %s", path, d.Reason)
		}
	}
}
