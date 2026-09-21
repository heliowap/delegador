package gitx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/heliowap/delegador/internal/safeenv"
)

func output(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// git roda hooks e textconv do repo: mesmo filho, mesmo ambiente limpo.
	cmd.Env = safeenv.List()
	out, err := cmd.Output()
	return string(out), err
}

// Diff devolve o diff completo da worktree, incluindo arquivos novos.
func Diff(ctx context.Context, dir string) (string, error) {
	if _, err := output(ctx, dir, "add", "-AN"); err != nil {
		return "", fmt.Errorf("git add -AN: %w", err)
	}
	return output(ctx, dir, "diff")
}

// DiffStat resume o diff em numeros.
func DiffStat(ctx context.Context, dir string) (int, int, int, error) {
	if _, err := output(ctx, dir, "add", "-AN"); err != nil {
		return 0, 0, 0, err
	}
	out, err := output(ctx, dir, "diff", "--numstat")
	if err != nil {
		return 0, 0, 0, err
	}
	var added, removed, files int
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		a, _ := strconv.Atoi(f[0])
		r, _ := strconv.Atoi(f[1])
		added, removed, files = added+a, removed+r, files+1
	}
	return added, removed, files, nil
}

// RevertNonTest desfaz, em dir, as mudancas de arquivos que nao casam com
// testGlobs. Usado no teste de mutacao: o teste tem que ficar vermelho.
//
// Arquivo novo (intent-to-add do `git add -AN`, ou untracked) e REMOVIDO —
// `checkout --` restauraria o blob vazio do indice e deixaria um stub de
// 0 bytes que quebra o build da tentativa seguinte e infla a sonda de
// mutacao. A enumeracao sai do status --porcelain, nao do `git diff`: a
// delecao estagiada pelo add -AN e invisivel ao diff e precisa de
// `checkout HEAD --` para voltar.
func RevertNonTest(ctx context.Context, dir string, testGlobs []string) error {
	out, err := output(ctx, dir, "status", "--porcelain", "-z")
	if err != nil {
		return err
	}
	for _, c := range mudancas(out) {
		if isTestFile(c.path, testGlobs) {
			continue
		}
		switch c.kind {
		case '?', 'A', 'C':
			// Arquivo novo ou copia: remove de verdade. RemoveAll cobre o
			// caso de um diretorio novo inteiro (?? dir/).
			if err := os.RemoveAll(filepath.Join(dir, c.path)); err != nil {
				return fmt.Errorf("removendo %s: %w", c.path, err)
			}
		case 'R':
			// Renomeado: some o nome novo e restaura a origem.
			if err := os.RemoveAll(filepath.Join(dir, c.path)); err != nil {
				return fmt.Errorf("removendo %s: %w", c.path, err)
			}
			if _, err := output(ctx, dir, "checkout", "HEAD", "--", c.orig); err != nil {
				return fmt.Errorf("revertendo %s: %w", c.orig, err)
			}
		default:
			// M, D, T e afins: restaura de HEAD — cobre a delecao
			// estagiada que `checkout --` (do indice) nao alcanca.
			if _, err := output(ctx, dir, "checkout", "HEAD", "--", c.path); err != nil {
				return fmt.Errorf("revertendo %s: %w", c.path, err)
			}
		}
	}
	return nil
}

// mudanca e uma entrada do status porcelain: a letra do status, o caminho
// atual e, para R/C, a origem.
type mudanca struct {
	kind byte
	path string
	orig string
}

// mudancas parseia `git status --porcelain -z`: cada entrada e "XY path";
// R/C carrega a origem no token seguinte. O -z evita a quotacao que a
// saida normal aplica a nomes com espaco ou acento.
func mudancas(z string) []mudanca {
	var ms []mudanca
	toks := strings.Split(z, "\x00")
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if len(t) < 4 {
			continue
		}
		m := mudanca{kind: t[0], path: t[3:]}
		if m.kind == ' ' {
			m.kind = t[1]
		}
		if t[0] == 'R' || t[0] == 'C' {
			if i+1 < len(toks) {
				i++
				m.orig = toks[i]
			}
		}
		ms = append(ms, m)
	}
	return ms
}

func isTestFile(name string, globs []string) bool {
	base := filepath.Base(name)
	for _, g := range globs {
		if ok, _ := filepath.Match(g, base); ok {
			return true
		}
	}
	return false
}
