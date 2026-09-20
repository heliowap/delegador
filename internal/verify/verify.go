package verify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/heliowap/delegador/internal/gitx"
)

// Config sao os comandos declarados no briefing.
type Config struct {
	TestCmd   string
	SuiteCmd  string
	LintCmd   string
	TestGlobs []string
	Timeout   time.Duration
}

// Step e a execucao de um comando, com a saida capturada.
type Step struct {
	Name       string `json:"nome"`
	Command    string `json:"comando"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Skipped    bool   `json:"pulado"`
	ExpectFail bool   `json:"esperava_falhar"`
}

// Report e o resultado da verificacao. Sao fatos: nenhum modelo participou.
type Report struct {
	Diff           string `json:"-"`
	Added          int    `json:"linhas_adicionadas"`
	Removed        int    `json:"linhas_removidas"`
	Files          int    `json:"arquivos"`
	Steps          []Step `json:"passos"`
	MutationProved bool   `json:"mutacao_provou"`
}

// Green indica que todo passo executado terminou como esperado: exit zero
// para os comandos do briefing, exit nao-zero para a sonda de mutacao —
// cujo "fracasso" e justamente a prova de que o teste pega o defeito.
func (r Report) Green() bool {
	for _, s := range r.Steps {
		if s.Skipped {
			continue
		}
		if s.ExpectFail {
			if s.ExitCode == 0 {
				return false
			}
			continue
		}
		if s.ExitCode != 0 {
			return false
		}
	}
	return true
}

// Step busca um passo pelo nome.
func (r Report) Step(name string) (Step, bool) {
	for _, s := range r.Steps {
		if s.Name == name {
			return s, true
		}
	}
	return Step{}, false
}

func runCmd(ctx context.Context, dir, name, command string, timeout time.Duration) Step {
	if command == "" {
		return Step{Name: name, Skipped: true}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	code := 0
	if err != nil {
		code = 1
		var ee *exec.ExitError
		if ok := asExitError(err, &ee); ok {
			code = ee.ExitCode()
		}
	}
	return Step{Name: name, Command: command, ExitCode: code, Stdout: string(out)}
}

func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

// Run executa a verificacao inteira do protocolo: diff, teste, mutacao,
// suite e lint. A mutacao roda numa copia descartavel da worktree, para que
// desfazer a correcao nunca toque o trabalho real.
func Run(ctx context.Context, dir string, cfg Config) (Report, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Minute
	}

	var rep Report

	diff, err := gitx.Diff(ctx, dir)
	if err != nil {
		return rep, fmt.Errorf("verify: diff: %w", err)
	}
	rep.Diff = diff
	rep.Added, rep.Removed, rep.Files, _ = gitx.DiffStat(ctx, dir)

	rep.Steps = append(rep.Steps, runCmd(ctx, dir, "teste", cfg.TestCmd, cfg.Timeout))

	if mut, err := mutation(ctx, dir, cfg); err == nil {
		rep.Steps = append(rep.Steps, mut)
		// A mutacao prova algo quando o teste FALHA sem a correcao.
		// Limitacao herdada do v1: uma falha de infra (timeout, ambiente
		// quebrado) tambem sai ExitCode!=0 e conta como "provou" — nao da
		// para distinguir teste-vermelho de nao-conseguiu-rodar.
		rep.MutationProved = !mut.Skipped && mut.ExitCode != 0
	} else {
		rep.Steps = append(rep.Steps, Step{Name: "mutacao", Skipped: true,
			Stdout: fmt.Sprintf("nao foi possivel executar: %v", err)})
	}

	rep.Steps = append(rep.Steps, runCmd(ctx, dir, "suite", cfg.SuiteCmd, cfg.Timeout))
	rep.Steps = append(rep.Steps, runCmd(ctx, dir, "lint", cfg.LintCmd, cfg.Timeout))

	return rep, nil
}

// mutation copia a worktree, desfaz a correcao e roda o teste. Teste que
// continua verde sem a correcao nao prova nada.
func mutation(ctx context.Context, dir string, cfg Config) (Step, error) {
	if cfg.TestCmd == "" {
		return Step{Name: "mutacao", Skipped: true}, nil
	}
	copyDir, err := os.MkdirTemp("", "delegador-mutacao-*")
	if err != nil {
		return Step{}, err
	}
	defer os.RemoveAll(copyDir)

	target := filepath.Join(copyDir, "wt")
	// Numa linked worktree o `.git` copiado e um arquivo que resolve a copia
	// como raiz da propria worktree: o revert fica na copia (verificado),
	// mas o gitdir/index e compartilhado com o original — efeito cosmetico.
	if out, err := exec.CommandContext(ctx, "cp", "-R", dir, target).CombinedOutput(); err != nil {
		return Step{}, fmt.Errorf("copiando worktree: %w: %s", err, out)
	}
	if err := gitx.RevertNonTest(ctx, target, cfg.TestGlobs); err != nil {
		return Step{}, err
	}

	s := runCmd(ctx, target, "mutacao", cfg.TestCmd, cfg.Timeout)
	s.Command = cfg.TestCmd + "   (com a correcao desfeita)"
	s.ExpectFail = true
	return s, nil
}
