// doctor.go — o diagnóstico pré-despacho: confere o que um job precisaria
// no meio do laço e falha com a CAUSA em vez de deixar o job morrer lá
// (spec §14, "Proxy local indisponível"). `--probe` remede a sondagem
// vencida: re-mede os modelos e grava o resultado no arquivo do roster.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/roster"
)

// doctorDeps sao as costuras testaveis do doctor: ambiente, http e sondagem.
// Nil vira o default de producao — os testes injetam fakes por aqui.
type doctorDeps struct {
	getenv func(string) string
	http   *http.Client
	probe  func(ctx context.Context, id string) (roster.Probe, error)
	now    time.Time
}

func (d doctorDeps) comPadroes() doctorDeps {
	if d.getenv == nil {
		d.getenv = os.Getenv
	}
	if d.http == nil {
		d.http = &http.Client{Timeout: 5 * time.Second}
	}
	if d.now.IsZero() {
		d.now = time.Now()
	}
	if d.probe == nil {
		base := d.getenv("DELEGADOR_BASE_URL")
		if base == "" {
			base = executorBaseURL
		}
		c := llm.New(llm.Options{BaseURL: base, APIKey: d.getenv("DELEGADOR_API_KEY")})
		d.probe = func(ctx context.Context, id string) (roster.Probe, error) {
			return roster.ProbeModel(ctx, c, id)
		}
	}
	return d
}

// doctorReport e o diagnostico: cada check guarda a causa da falha, nao so
// o veredito — um "NAO pronto" sem motivo nao ajuda quem le.
type doctorReport struct {
	baseURL     string
	proxyStatus int
	proxyErr    error
	hasKey      bool
	rosterPath  string
	rosterErr   error
	models      []roster.Model
	elegiveis   []roster.Model
	motivos     []string
	vencidos    []roster.Model // habilitados com sondagem mais velha que o limite
}

// ok diz se o ambiente esta pronto para despachar: proxy respondendo, chave
// presente, roster lido e pelo menos um modelo elegivel.
func (r doctorReport) ok() bool {
	return r.proxyErr == nil && r.hasKey && r.rosterErr == nil && len(r.elegiveis) > 0
}

// checkDoctor roda os quatro checks. Nao para no primeiro erro: o relatorio
// acumula todas as causas de uma vez.
func checkDoctor(ctx context.Context, rosterFlag string, d doctorDeps) doctorReport {
	d = d.comPadroes()
	var rep doctorReport

	rep.baseURL = d.getenv("DELEGADOR_BASE_URL")
	if rep.baseURL == "" {
		rep.baseURL = executorBaseURL
	}
	rep.hasKey = d.getenv("TYPESAFE_API_KEY") != ""

	rep.rosterPath = rosterFlag
	if rep.rosterPath == "" {
		rep.rosterPath = d.getenv("DELEGADOR_ROSTER")
	}
	if rep.rosterPath == "" {
		rep.rosterPath = defaultRosterPath()
	}

	// A sonda mais barata que ainda e honesta: GET /models. Qualquer
	// resposta HTTP prova que ha um servidor na ponta (mesmo um 401); o
	// que falha e o transporte — conexao recusada, timeout, DNS.
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(rep.baseURL, "/")+"/models", nil); err != nil {
		rep.proxyErr = err
	} else {
		if k := d.getenv("DELEGADOR_API_KEY"); k != "" {
			req.Header.Set("Authorization", "Bearer "+k)
		}
		if resp, err := d.http.Do(req); err != nil {
			rep.proxyErr = err
		} else {
			rep.proxyStatus = resp.StatusCode
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}

	rep.models, rep.rosterErr = roster.Load(rep.rosterPath)
	if rep.rosterErr == nil {
		rep.elegiveis, rep.motivos = roster.Elegiveis(rep.models, sondagemMaxIdade, d.now)
		for _, m := range rep.models {
			if m.Habilitado && d.now.Sub(m.Sondado.Em) > sondagemMaxIdade {
				rep.vencidos = append(rep.vencidos, m)
			}
		}
	}
	return rep
}

func runDoctor(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		probe   = fs.Bool("probe", false, "re-sonda os modelos com sondagem vencida e grava o resultado no roster")
		rosterF = fs.String("roster", "", "caminho do roster.yaml (padrao: DELEGADOR_ROSTER ou o do modulo delegador)")
	)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "doctor: argumento posicional inesperado — uso: doctor [--probe] [--roster <path>]")
		return ExitUsage
	}
	return doctor(ctx, *rosterF, *probe, doctorDeps{}, stdout)
}

func doctor(ctx context.Context, rosterFlag string, doProbe bool, deps doctorDeps, stdout io.Writer) int {
	deps = deps.comPadroes()
	rep := checkDoctor(ctx, rosterFlag, deps)

	if rep.proxyErr != nil {
		fmt.Fprintf(stdout, "proxy:     %s — FALHOU: %v\n", rep.baseURL, rep.proxyErr)
	} else {
		fmt.Fprintf(stdout, "proxy:     %s — respondeu (GET /models: %d)\n", rep.baseURL, rep.proxyStatus)
	}
	if rep.hasKey {
		fmt.Fprintln(stdout, "chave:     TYPESAFE_API_KEY presente")
	} else {
		fmt.Fprintln(stdout, "chave:     TYPESAFE_API_KEY AUSENTE — gates, watchdog e compactacao nao rodam")
	}
	if rep.rosterErr != nil {
		fmt.Fprintf(stdout, "roster:    %s — FALHOU: %v\n", rep.rosterPath, rep.rosterErr)
	} else {
		fmt.Fprintf(stdout, "roster:    %s — %d modelos\n", rep.rosterPath, len(rep.models))
		fmt.Fprintf(stdout, "elegiveis: %d de %d\n", len(rep.elegiveis), len(rep.models))
		for _, m := range rep.motivos {
			fmt.Fprintf(stdout, "  excluido: %s\n", m)
		}
		if len(rep.vencidos) > 0 {
			ids := make([]string, len(rep.vencidos))
			for i, m := range rep.vencidos {
				ids[i] = m.ID
			}
			fmt.Fprintf(stdout, "sondagem:  %d vencida(s): %s — remede com: delegador doctor --probe\n",
				len(ids), strings.Join(ids, ", "))
		}
	}

	if doProbe && rep.rosterErr == nil {
		if len(rep.vencidos) == 0 {
			fmt.Fprintln(stdout, "sondagem:  todas em dia, nada a remeder")
		}
		for _, m := range rep.vencidos {
			p, err := deps.probe(ctx, m.ID)
			if err != nil {
				fmt.Fprintf(stdout, "sondagem:  %s — FALHOU: %v\n", m.ID, err)
				continue
			}
			if err := roster.WriteProbe(rep.rosterPath, m.ID, p); err != nil {
				fmt.Fprintf(stdout, "sondagem:  %s — gravando roster: %v\n", m.ID, err)
				continue
			}
			fmt.Fprintf(stdout, "sondagem:  %s — gravada (tool_call=%v, reasoning=%v, tokens_base=%d, %.1fs)\n",
				m.ID, p.ToolCall, p.ReasoningContent, p.TokensBase, p.LatenciaS)
		}
		// Re-avalia com o roster atualizado: o veredito final e sobre o
		// que o arquivo diz agora, nao sobre o que dizia antes da remedicao.
		if len(rep.vencidos) > 0 {
			rep = checkDoctor(ctx, rosterFlag, deps)
			fmt.Fprintf(stdout, "elegiveis: %d de %d apos a sondagem\n", len(rep.elegiveis), len(rep.models))
		}
	}

	if rep.ok() {
		fmt.Fprintln(stdout, "ambiente pronto")
		return 0
	}
	var causas []string
	if rep.proxyErr != nil {
		causas = append(causas, "proxy inalcançavel")
	}
	if !rep.hasKey {
		causas = append(causas, "TYPESAFE_API_KEY ausente")
	}
	if rep.rosterErr != nil {
		causas = append(causas, "roster nao carrega")
	} else if len(rep.elegiveis) == 0 {
		causas = append(causas, "nenhum modelo elegivel")
	}
	fmt.Fprintf(stdout, "ambiente NAO pronto: %s\n", strings.Join(causas, "; "))
	return 1
}
