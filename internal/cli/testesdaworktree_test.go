package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func escreve(t *testing.T, dir string, caminhos ...string) {
	t.Helper()
	for _, c := range caminhos {
		p := filepath.Join(dir, c)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Medido em 2026-09-21 na issue expr-lang/expr#950: a worktree era um
// repositorio real com dezenas de arquivos de teste, e o briefing anunciou
// como "criterio de aceite" os seis primeiros em ordem alfabetica —
// ast/find_test.go, bench_test.go e afins — nenhum deles o teste da tarefa.
// Lista truncada e um palpite apresentado como fato: melhor nao listar.
func TestListaIncompletaNaoVaiProBriefing(t *testing.T) {
	dir := t.TempDir()
	escreve(t, dir, "a_test.go", "b_test.go", "c_test.go",
		"d_test.go", "e_test.go", "f_test.go", "g_test.go")
	if got := testesNaWorktree(dir, []string{"*_test.go"}); got != nil {
		t.Errorf("quero nenhum arquivo quando a lista nao cabe, tenho %v", got)
	}
}

// Lista completa e pequena continua valendo: e o caso da worktree de uma
// tarefa so, onde citar os arquivos ajuda o executor a achar o contrato.
func TestListaCompletaVaiProBriefing(t *testing.T) {
	dir := t.TempDir()
	escreve(t, dir, "pkg/moeda/arredondar_test.go", "pkg/moeda/arredondar.go")
	got := testesNaWorktree(dir, []string{"*_test.go"})
	if len(got) != 1 || got[0] != filepath.Join("pkg", "moeda", "arredondar_test.go") {
		t.Errorf("quero o unico teste da worktree, tenho %v", got)
	}
}
