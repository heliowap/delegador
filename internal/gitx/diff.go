package gitx

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func output(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
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
func RevertNonTest(ctx context.Context, dir string, testGlobs []string) error {
	out, err := output(ctx, dir, "diff", "--name-only")
	if err != nil {
		return err
	}
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if name == "" || isTestFile(name, testGlobs) {
			continue
		}
		if _, err := output(ctx, dir, "checkout", "--", name); err != nil {
			return fmt.Errorf("revertendo %s: %w", name, err)
		}
	}
	return nil
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
