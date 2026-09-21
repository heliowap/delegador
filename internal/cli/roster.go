// roster.go — o subcomando do roster: sem flags, lista os modelos com o
// status de elegibilidade, o motivo de cada exclusão e a idade da sondagem;
// `--probe [id]` re-mede e grava o resultado no arquivo — com id, aquele
// modelo; sem id, todos os habilitados com sondagem vencida.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/heliowap/delegador/internal/roster"
)

// resolveRosterPath aplica a precedência de sempre: flag > DELEGADOR_ROSTER
// > o roster do módulo — mesma ordem do run e do plan.
func resolveRosterPath(flagValue string, getenv func(string) string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := getenv("DELEGADOR_ROSTER"); v != "" {
		return v
	}
	return defaultRosterPath()
}

func runRoster(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("roster", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		probe   = fs.Bool("probe", false, "re-sonda e grava no roster: o id posicional, ou todos os vencidos sem id")
		rosterF = fs.String("roster", "", "caminho do roster.yaml (padrao: DELEGADOR_ROSTER ou o do modulo delegador)")
	)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "roster: uso: roster [--roster <path>] | roster --probe [id] [--roster <path>]")
		return ExitUsage
	}
	var id string
	if fs.NArg() == 1 {
		id = fs.Arg(0)
	}
	if id != "" && !*probe {
		fmt.Fprintln(stderr, "roster: id posicional so faz sentido com --probe")
		return ExitUsage
	}
	return rosterCmd(ctx, *rosterF, *probe, id, doctorDeps{}, stdout, stderr)
}

func rosterCmd(ctx context.Context, rosterFlag string, doProbe bool, id string, deps doctorDeps, stdout, stderr io.Writer) int {
	deps = deps.comPadroes()
	path := resolveRosterPath(rosterFlag, deps.getenv)
	models, err := roster.Load(path)
	if err != nil {
		fmt.Fprintf(stderr, "roster: %v\n", err)
		return 1
	}

	if !doProbe {
		listaRoster(stdout, models, deps.now)
		return 0
	}

	// Quem sonda: o id pedido, ou todos os habilitados com sondagem vencida.
	var alvos []roster.Model
	if id != "" {
		var m *roster.Model
		for i := range models {
			if models[i].ID == id {
				m = &models[i]
				break
			}
		}
		if m == nil {
			fmt.Fprintf(stderr, "roster: modelo %q nao esta em %s\n", id, path)
			return 1
		}
		alvos = []roster.Model{*m}
	} else {
		for _, m := range models {
			if m.Habilitado && deps.now.Sub(m.Sondado.Em) > sondagemMaxIdade {
				alvos = append(alvos, m)
			}
		}
		if len(alvos) == 0 {
			fmt.Fprintln(stdout, "roster: nenhuma sondagem vencida — nada a remeder")
			return 0
		}
	}

	falhas := 0
	for _, m := range alvos {
		p, err := deps.probe(ctx, m.ID)
		if err != nil {
			fmt.Fprintf(stderr, "roster: %s — sondagem FALHOU: %v\n", m.ID, err)
			falhas++
			continue
		}
		if err := roster.WriteProbe(path, m.ID, p); err != nil {
			fmt.Fprintf(stderr, "roster: %s — gravando: %v\n", m.ID, err)
			falhas++
			continue
		}
		fmt.Fprintf(stdout, "roster: %s — gravada em %s (tool_call=%v, reasoning=%v, tokens_base=%d, %.1fs)\n",
			m.ID, p.Em.Format("2006-01-02"), p.ToolCall, p.ReasoningContent, p.TokensBase, p.LatenciaS)
	}
	if falhas > 0 {
		return 1
	}
	return 0
}

// listaRoster imprime um modelo por linha: id, elegibilidade com o motivo
// da exclusão, idade da sondagem e o que a sondagem mediu.
func listaRoster(stdout io.Writer, models []roster.Model, agora time.Time) {
	_, motivos := roster.Elegiveis(models, sondagemMaxIdade, agora)
	motivoDe := map[string]string{}
	for _, m := range motivos {
		id, r, _ := strings.Cut(m, ": ")
		motivoDe[id] = r
	}

	elegiveis := 0
	for _, m := range models {
		status := "elegivel"
		if r, ex := motivoDe[m.ID]; ex {
			status = "EXCLUIDO: " + r
		} else {
			elegiveis++
		}
		sondagem := "nunca sondado"
		if !m.Sondado.Em.IsZero() {
			dias := int(agora.Sub(m.Sondado.Em).Hours() / 24)
			sondagem = fmt.Sprintf("sondado em %s (ha %dd), tool_call=%v",
				m.Sondado.Em.Format("2006-01-02"), dias, m.Sondado.ToolCall)
		}
		fmt.Fprintf(stdout, "%-30s %-70s %s\n", m.ID, status, sondagem)
	}
	fmt.Fprintf(stdout, "%d de %d elegiveis\n", elegiveis, len(models))
}
