// Package render escreve o bloco de resultado que chega ao orquestrador:
// cancelamento, divergencias, veredito verificado, trace compactado e a
// linha de custo — nesta ordem, porque quem le precisa ver o que desmente
// antes do que afirma (spec §6.6).
package render

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/job"
	"github.com/heliowap/delegador/internal/route"
	"github.com/heliowap/delegador/internal/tools"
	"github.com/heliowap/delegador/internal/verify"
)

// afirmaVerdeMin e o corte do noul relatorio_afirma_verde: acima dele a
// afirmacao de verde conta como feita, e um verify vermelho vira
// divergencia.
const afirmaVerdeMin = 0.5

// Input e o que o resultado precisa para se escrever.
type Input struct {
	Job         *job.Job
	Verify      verify.Report
	Turns       []agent.Turn
	AfirmaVerde float64
	Escolha     route.Escolha
	Escalou     bool
	JevUSD      float64
	ExecutorUSD float64
}

// Result escreve o relatorio final.
func Result(w io.Writer, in Input) {
	// Cancelamento primeiro: se o watchdog cortou o laco, e o fato que
	// enquadra todo o resto.
	if in.Job != nil && in.Job.CancelReason != nil {
		cr := in.Job.CancelReason
		fmt.Fprintf(w, "CANCELADO pelo watchdog: %s (noul %.2f)\n", cr.Signal, cr.Probability)
		if cr.TurnExcerpt != "" {
			fmt.Fprintf(w, "  trecho do turno: %s\n", cr.TurnExcerpt)
		}
		if cr.ResumeCommand != "" {
			fmt.Fprintf(w, "  retomar com:     %s\n", cr.ResumeCommand)
		}
		fmt.Fprintln(w)
	}

	// Divergencias: o que desmente a entrega vem antes do veredito.
	if in.AfirmaVerde >= afirmaVerdeMin && !in.Verify.Green() {
		fmt.Fprintf(w, "DIVERGENCIA: o relatorio afirma verde (noul %.2f), mas a verificacao saiu vermelha.\n",
			in.AfirmaVerde)
		for _, s := range in.Verify.Steps {
			if s.Skipped || s.ExpectFail || s.ExitCode == 0 {
				continue
			}
			fmt.Fprintf(w, "  saida real de %s:\n", s.Name)
			fmt.Fprintln(w, indent(s.Stdout, "    "))
		}
	}
	if !in.Verify.MutationProved {
		if s, ok := in.Verify.Step("mutacao"); !ok || s.Skipped {
			fmt.Fprintln(w, "aviso: sonda de mutacao nao rodou; a entrega esta sem prova")
		} else {
			fmt.Fprintln(w, "aviso: mutacao nao provou nada — o teste ficou verde mesmo com a correcao desfeita")
		}
	}

	// O veredito verificado: os numeros de verify.json, fatos sem modelo.
	if in.Verify.Green() {
		fmt.Fprintln(w, "veredito: VERDE")
	} else {
		fmt.Fprintln(w, "veredito: VERMELHO")
	}
	fmt.Fprintf(w, "  diff: %d arquivos, +%d -%d\n", in.Verify.Files, in.Verify.Added, in.Verify.Removed)
	for _, s := range in.Verify.Steps {
		if s.Skipped {
			fmt.Fprintf(w, "  %s: pulado\n", s.Name)
			continue
		}
		fmt.Fprintf(w, "  %s: exit %d\n", s.Name, s.ExitCode)
	}

	// O trace compactado por delecao: o que ficou, ficou inteiro.
	fmt.Fprintf(w, "trace: %d turnos mantidos\n", len(in.Turns))
	for _, t := range in.Turns {
		for i, call := range t.Message.ToolCalls {
			fmt.Fprintf(w, "  turno %d: %s %s\n", t.Index, call.Name, callArgs(call))
			if i < len(t.Results) {
				fmt.Fprintln(w, indent(t.Results[i].Output, "    "))
			}
		}
		if len(t.Message.ToolCalls) == 0 && t.Message.Content != "" {
			fmt.Fprintf(w, "  turno %d: %s\n", t.Index, t.Message.Content)
		}
	}

	// Modelo e custo: Jev e executor separados porque as ordens de
	// grandeza sao diferentes (spec §9).
	if in.Escolha.Modelo.ID != "" {
		fmt.Fprintf(w, "modelo:   %s", in.Escolha.Modelo.ID)
		if in.Escolha.Dimensao != "" {
			fmt.Fprintf(w, " (dimensao %s)", in.Escolha.Dimensao)
		}
		fmt.Fprintln(w)
	}
	if in.Escolha.NaoMedido {
		fmt.Fprintln(w, "          modelo nao medido: sem benchmark de terceiro, entrou por viabilidade e custo")
	}
	if in.Escolha.Motivo != "" {
		fmt.Fprintf(w, "rota:     %s\n", in.Escolha.Motivo)
	}
	if in.Escalou {
		fmt.Fprintln(w, "escalada: sim")
	}
	fmt.Fprintf(w, "custo:    executor $%.4f, jev $%.5f\n", in.ExecutorUSD, in.JevUSD)
}

// indent desloca cada linha do texto para dentro do bloco.
func indent(s, prefix string) string {
	if s == "" {
		return prefix + "(sem saida)"
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

// callArgs devolve os argumentos da chamada como pares k=v ordenados, sem
// as chaves internas "_*".
func callArgs(c tools.Call) string {
	keys := make([]string, 0, len(c.Args))
	for k := range c.Args {
		if strings.HasPrefix(k, "_") {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%q", k, c.Args[k])
	}
	return strings.Join(parts, " ")
}
