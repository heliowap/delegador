package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// requiredFlags sao as flags do devin das quais o companion depende.
// Se uma some numa versao futura, doctor avisa antes do primeiro dispatch.
var requiredFlags = []string{
	"--prompt-file",
	"--permission-mode",
	"--respect-workspace-trust",
	"--model",
	"--export",
	"-p",
}

var versionRe = regexp.MustCompile(`devin\s+(\S+)`)

// Report e o diagnostico do ambiente local.
type Report struct {
	DevinPath      string
	DevinVersion   string
	MissingFlags   []string
	HasTypeSafeKey bool
}

// OK indica ambiente pronto para despachar.
func (r Report) OK() bool { return len(r.MissingFlags) == 0 && r.HasTypeSafeKey }

// Check diagnostica o ambiente. lookPath e run sao injetados para teste.
func Check(
	ctx context.Context,
	lookPath func(string) (string, error),
	run func(context.Context, string, ...string) ([]byte, error),
) (Report, error) {
	path, err := lookPath("devin")
	if err != nil {
		return Report{}, fmt.Errorf("devin nao encontrado no PATH: %w", err)
	}
	rep := Report{DevinPath: path, HasTypeSafeKey: os.Getenv("TYPESAFE_API_KEY") != ""}

	if out, err := run(ctx, path, "--version"); err == nil {
		if m := versionRe.FindSubmatch(out); m != nil {
			rep.DevinVersion = string(m[1])
		}
	}

	help, err := run(ctx, path, "--help")
	if err != nil {
		return rep, fmt.Errorf("devin --help falhou: %w", err)
	}
	for _, f := range requiredFlags {
		if !strings.Contains(string(help), f) {
			rep.MissingFlags = append(rep.MissingFlags, f)
		}
	}
	return rep, nil
}

func runDoctor(ctx context.Context, _ []string, stdout, stderr io.Writer) int {
	rep, err := Check(ctx, exec.LookPath, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	})
	if err != nil {
		fmt.Fprintf(stderr, "doctor: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "devin:  %s (%s)\n", rep.DevinPath, rep.DevinVersion)
	if len(rep.MissingFlags) > 0 {
		fmt.Fprintf(stdout, "flags ausentes: %v\n", rep.MissingFlags)
	}
	if !rep.HasTypeSafeKey {
		fmt.Fprintln(stdout, "TYPESAFE_API_KEY: ausente — os gates e o watchdog ficam desligados")
	}
	if !rep.OK() {
		return 1
	}
	fmt.Fprintln(stdout, "ambiente pronto")
	return 0
}
