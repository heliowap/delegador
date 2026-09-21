package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
)

// ExitUsage e o codigo de saida para invocacao invalida.
const ExitUsage = 2

type handler func(ctx context.Context, args []string, stdout, stderr io.Writer) int

func handlers() map[string]handler {
	return map[string]handler{
		"doctor": runDoctor,
		"plan":   runPlan,
		"result": runResult,
		"roster": runRoster,
		"run":    runRun,
		"status": runStatus,
	}
}

// Run despacha o subcomando e devolve o codigo de saida do processo.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	hs := handlers()
	if len(args) == 0 {
		usage(stderr, hs, "")
		return ExitUsage
	}
	h, ok := hs[args[0]]
	if !ok {
		usage(stderr, hs, args[0])
		return ExitUsage
	}
	return h(ctx, args[1:], stdout, stderr)
}

func usage(w io.Writer, hs map[string]handler, unknown string) {
	if unknown != "" {
		fmt.Fprintf(w, "delegador: subcomando desconhecido %q\n\n", unknown)
	}
	names := make([]string, 0, len(hs))
	for n := range hs {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintf(w, "uso: delegador <subcomando> [flags]\n\nsubcomandos: %v\n", names)
}
