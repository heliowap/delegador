// run.go — o subcomando que junta tudo (spec §6.3–§6.6): remonta a politica
// e a verificacao a partir do que o plan persistiu no job, roda o laco do
// executor, verifica em codigo, decide a cascata e, se escalar, re-roteia
// com o corte elevado e tenta de novo. O relatorio sai para stdout e fica
// gravado em result.txt para o `result`.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/cascade"
	"github.com/heliowap/delegador/internal/compact"
	"github.com/heliowap/delegador/internal/gitx"
	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/job"
	"github.com/heliowap/delegador/internal/ledger"
	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/render"
	"github.com/heliowap/delegador/internal/roster"
	"github.com/heliowap/delegador/internal/route"
	"github.com/heliowap/delegador/internal/tools"
	"github.com/heliowap/delegador/internal/verify"
)

// executorBaseURL e o proxy OpenAI-compativel padrao do executor; DELEGADOR_BASE_URL
// sobrepoe (teste e deploy alternativo).
const executorBaseURL = "http://127.0.0.1:8317/v1"

// runMaxTurns e o teto de turnos de fabrica por tentativa: folgado para uma
// correcao delegada, curto o bastante para o teto cortar um laco em fuga.
const runMaxTurns = 30

// askerContado embrulha o cliente Jev gravando o uso de cada Ask no ledger
// do job — a economia so fica auditavel se toda chamada registrar o que
// gastou. O kind distingue os tres consumidores: pre-condicao do laco,
// compactacao do trace e o noul do relatorio.
type askerContado struct {
	a    agent.Asker
	l    *jev.Ledger
	kind string
}

func (m askerContado) Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error) {
	res, err := m.a.Ask(ctx, state, qs)
	if err == nil {
		_ = m.l.Record(m.kind, res.Usage)
	}
	return res, err
}

// precosDo devolve o preco por MTok do modelo no roster, aplicado aos dois
// lados — o roster declara um custo unico. Modelo ausente ou custo null nao
// derruba o run: o ledger registra os tokens com preco zero e avisa no
// stderr, porque perder o token e pior que perder o dolar.
func precosDo(models []roster.Model, id string, stderr io.Writer) (in, out float64) {
	for _, m := range models {
		if m.ID == id {
			if m.CustoUSDPorMTok == nil {
				fmt.Fprintf(stderr, "run: %s tem custo_usd_por_mtok null; tokens contabilizados sem preco\n", id)
				return 0, 0
			}
			return *m.CustoUSDPorMTok, *m.CustoUSDPorMTok
		}
	}
	fmt.Fprintf(stderr, "run: %s nao esta no roster; tokens contabilizados sem preco\n", id)
	return 0, 0
}

// escolhaDoJob reconstroi a Escolha da rota persistida no job, para o
// relatorio explicar o modelo sem refazer a conta — mesma funcao do
// registro que o plan grava.
func escolhaDoJob(models []roster.Model, j *job.Job) route.Escolha {
	e := route.Escolha{Dimensao: route.Dimensao(j.Dimensao), Percentil: j.Percentil}
	for _, m := range models {
		if m.ID == j.Model {
			e.Modelo = m
			e.NaoMedido = m.Benchmark == nil
			return e
		}
	}
	e.Modelo = roster.Model{ID: j.Model}
	return e
}

// evidenciaCap e o teto por bloco de evidencia (saida de passo e diff) —
// mesma ordem do digestFieldCap: a tentativa reprovada nao pode estourar
// o contexto da tentativa seguinte, e o corte e declarado no texto.
const evidenciaCap = 4000

func cortaEvidencia(s string) string {
	if len(s) <= evidenciaCap {
		return s
	}
	return fmt.Sprintf("%s\n[%d bytes cortados]", s[:evidenciaCap], len(s)-evidenciaCap)
}

// evidenciaEscalada monta o contexto da tentativa seguinte: a saida dos
// passos e o diff deixado pela tentativa reprovada, verbatim, como a
// cascata manda (spec §6.5). revertido avisa se o revert rodou de fato —
// sem ele a frase final mentiria sobre o estado da worktree.
func evidenciaEscalada(rep verify.Report, revertido bool) string {
	var sb strings.Builder
	sb.WriteString("\n\n---\n\nA tentativa anterior reprovou na verificacao. Evidencia verbatim:\n\n")
	for _, s := range rep.Steps {
		if s.Skipped {
			continue
		}
		fmt.Fprintf(&sb, "### passo %s: exit %d\n%s\n", s.Name, s.ExitCode, cortaEvidencia(s.Stdout))
	}
	if rep.Diff != "" {
		fmt.Fprintf(&sb, "### diff deixado pela tentativa\n%s\n", cortaEvidencia(rep.Diff))
	}
	if revertido {
		sb.WriteString("As mudancas de nao-teste foram revertidas; o teste vermelho ficou como reproducao.\n")
	} else {
		sb.WriteString("O revert das mudancas de nao-teste FALHOU — a worktree pode ainda carregar codigo da tentativa reprovada.\n")
	}
	return sb.String()
}

// guardaVerify persiste o verify da tentativa: "os dois diffs" do spec —
// cada tentativa deixa o proprio Report e o proprio diff, para o humano
// que recebe o caso quando a cascata bate no teto. O indice vem dos
// verify-*.json ja existentes: na retomada a numeracao continua em vez
// de sobrescrever o artefato da tentativa anterior.
func guardaVerify(j *job.Job, rep verify.Report, stderr io.Writer) {
	n := 1
	if entries, err := os.ReadDir(j.Dir()); err == nil {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "verify-") || !strings.HasSuffix(name, ".json") {
				continue
			}
			k, err := strconv.Atoi(name[len("verify-") : len(name)-len(".json")])
			if err == nil && k >= n {
				n = k + 1
			}
		}
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "run: serializando verify-%d: %v\n", n, err)
	} else if err := os.WriteFile(j.Path(fmt.Sprintf("verify-%d.json", n)), raw, 0o644); err != nil {
		fmt.Fprintf(stderr, "run: gravando verify-%d.json: %v\n", n, err)
	}
	if rep.Diff != "" {
		if err := os.WriteFile(j.Path(fmt.Sprintf("verify-%d.diff", n)), []byte(rep.Diff), 0o644); err != nil {
			fmt.Fprintf(stderr, "run: gravando verify-%d.diff: %v\n", n, err)
		}
	}
}

// anotaFalha deixa em result.txt a causa de uma saida sem relatorio — o
// `result` tem o que mostrar em vez de arquivo ausente ou texto velho.
func anotaFalha(j *job.Job, format string, a ...any) {
	msg := fmt.Sprintf(format+"\n", a...)
	_ = os.WriteFile(j.Path("result.txt"), []byte(msg), 0o644)
}

func runRun(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		jobID    = fs.String("job", "", "id do job planejado")
		rosterF  = fs.String("roster", "", "caminho do roster.yaml (padrao: DELEGADOR_ROSTER ou o do modulo delegador)")
		maxTurns = fs.Int("max-turns", runMaxTurns, "teto de turnos do laco por tentativa")
	)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "run: argumento posicional inesperado — uso: run --job <id> [--roster <path>] [--max-turns N]")
		return ExitUsage
	}
	if *maxTurns <= 0 {
		fmt.Fprintln(stderr, "run: --max-turns precisa ser > 0")
		return ExitUsage
	}
	if *jobID == "" {
		fmt.Fprintln(stderr, "run: --job e obrigatorio")
		return ExitUsage
	}

	j, err := job.Load(*jobID)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return 1
	}
	// Rodavel: planned na primeira vez, failed na retomada. running pode
	// ser um executor vivo; completed/cancelled nao tem o que rodar.
	if j.State != job.StatePlanned && j.State != job.StateFailed {
		fmt.Fprintf(stderr, "run: job %s esta %s; run aceita planned ou failed\n", j.ID, j.State)
		return 1
	}
	// Na retomada a trava ja foi solta pelo Release terminal — readquire
	// antes de rodar: mesma worktree, mesmo dono, e se outro job tomou a
	// vaga o erro nomeia o dono em vez de executar destravado.
	if err := j.Reacquire(); err != nil {
		anotaFalha(j, "run: %v", err)
		fmt.Fprintf(stderr, "run: %v\n", err)
		return 1
	}
	defer func() {
		// Estado terminal solta a trava: o job encerrou e a worktree
		// volta a aceitar delegacao. planned/running mantem — a vaga
		// ainda e deste job. Release confere o dono: trava alheia fica.
		if j.State.Terminal() {
			_ = job.Release(j.ID)
		}
	}()

	// O roster entra antes do laco: o ledger do executor precisa do preco
	// do modelo e a cascata precisa dos elegiveis para re-rotear.
	rosterPath := *rosterF
	if rosterPath == "" {
		rosterPath = os.Getenv("DELEGADOR_ROSTER")
	}
	if rosterPath == "" {
		rosterPath = defaultRosterPath()
	}
	models, err := roster.Load(rosterPath)
	if err != nil {
		anotaFalha(j, "run: %v", err)
		fmt.Fprintf(stderr, "run: %v\n", err)
		return 1
	}

	briefing, err := os.ReadFile(j.Path("briefing.md"))
	if err != nil {
		anotaFalha(j, "run: lendo briefing: %v", err)
		fmt.Fprintf(stderr, "run: lendo briefing: %v\n", err)
		return 1
	}

	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		msg := "run: TYPESAFE_API_KEY ausente. Sem ela a pre-condicao " +
			"e a compactacao nao rodam, e executar sem watchdog e o que este plugin evita."
		anotaFalha(j, "%s", msg)
		fmt.Fprintln(stderr, msg)
		return 1
	}
	jevClient := jev.New(jev.Options{APIKey: key, BaseURL: os.Getenv("TYPESAFE_BASE_URL")})
	jevLedger := &jev.Ledger{Path: j.Path("jev.jsonl")}
	execLedger := &ledger.Ledger{Path: j.Path("executor.jsonl")}

	base := os.Getenv("DELEGADOR_BASE_URL")
	if base == "" {
		base = executorBaseURL
	}
	// A chave sai do ambiente e nao e gravada nem impressa; o proxy local
	// pode nem pedir uma.
	llmClient := llm.New(llm.Options{BaseURL: base, APIKey: os.Getenv("DELEGADOR_API_KEY")})

	// A politica e a verificacao sao remontadas dos campos que o plan
	// persistiu — nada se repede por flag.
	policy := tools.Policy{
		Worktree:      j.Worktree,
		WritePrefixes: j.WritePrefixes,
		AllowCommands: j.AllowCommands,
	}
	verifyCfg := verify.Config{
		TestCmd: j.TestCmd, SuiteCmd: j.SuiteCmd, LintCmd: j.LintCmd,
		TestGlobs: j.TestGlobs,
	}

	// CancelReason da tentativa anterior morre aqui: sem isso um retorno
	// cedo (verify de infra, rota esgotada) salvaria o veto velho de novo.
	j.CancelReason = nil
	j.State = job.StateRunning
	if err := j.Save(); err != nil {
		anotaFalha(j, "run: %v", err)
		fmt.Fprintf(stderr, "run: %v\n", err)
		return 1
	}

	var (
		reg               = &tools.Registry{}
		out               agent.Outcome
		rep               verify.Report
		escolha           = escolhaDoJob(models, j)
		precoIn, precoOut = precosDo(models, j.Model, stderr)
		escalou           bool
		dePara            string // "a -> b" da escalada, para o relatorio
		motivoEscalada    string
		motivoCascata     string // por que parou sem escalar, quando houve
		contexto          string // evidencia da falha anterior, na segunda tentativa
	)

	for {
		// O custo que o teto ve = ledger ja gravado + turnos corridos
		// desta tentativa, recomputado a cada pre-condicao — vetar no
		// meio do laco nao pode esperar o fim dele.
		var emCurso float64
		pre := agent.NewPrecondition(agent.DefaultPreConfig(),
			askerContado{jevClient, jevLedger, "precondicao"},
			func() float64 {
				total, _ := execLedger.Total()
				return total + emCurso
			})
		vigia := func(turns []agent.Turn) *agent.Veto {
			var inTok, outTok int
			for _, t := range turns {
				inTok += t.Usage.PromptTokens
				outTok += t.Usage.CompletionTokens
			}
			emCurso = float64(inTok)/1e6*precoIn + float64(outTok)/1e6*precoOut
			return pre(turns)
		}

		var runErr error
		out, runErr = agent.Run(ctx, llmClient, reg,
			agent.Config{Model: j.Model, MaxTurns: *maxTurns, Policy: policy},
			string(briefing)+contexto, vigia)
		// Um registro por turno com o preco vigente na hora — os tokens
		// sao os que a API reportou, nunca estimativa local (spec §9).
		for _, t := range out.Turns {
			if err := execLedger.Record("turno",
				t.Usage.PromptTokens, t.Usage.CompletionTokens, precoIn, precoOut); err != nil {
				fmt.Fprintf(stderr, "run: gravando executor.jsonl: %v\n", err)
			}
		}
		if runErr != nil {
			fmt.Fprintf(stderr, "run: laco do executor: %v\n", runErr)
		}

		rep, err = verify.Run(ctx, j.Worktree, verifyCfg)
		if err != nil {
			// Verificacao que nao completou e falha de infra: nao chega
			// na cascata, nao escala — reporta a causa e falha o job.
			fmt.Fprintf(stderr, "run: verificacao nao completou: %v\n", err)
			j.State = job.StateFailed
			_ = j.Save()
			anotaFalha(j, "verificacao nao completou por falha de infraestrutura: %v", err)
			fmt.Fprintf(stdout, "verificacao nao completou por falha de infraestrutura: %v\n", err)
			return 1
		}
		guardaVerify(j, rep, stderr)

		d := cascade.Avaliar(out, rep, j.Escaladas, cascade.DefaultConfig())
		if !d.Escala {
			motivoCascata = d.Motivo
			break
		}

		// Re-roteio ANTES de tocar o job: Escolher primeiro — o modelo que
		// falhou sai do conjunto (escalar e trocar de modelo, nao repetir
		// o que a verificacao reprovou) — e so depois sobem Escaladas,
		// Percentil e Model, no MESMO Save. Sem rota o job falha com o
		// estado inteiro: um restart nao reaplica degrau ja contado nem
		// ve Escaladas subir sem o modelo novo.
		novoPercentil := min(1.0, j.Percentil+d.NovoPercentil)
		elegiveis, motivos := roster.Elegiveis(models, sondagemMaxIdade, time.Now())
		for _, m := range motivos {
			fmt.Fprintf(stderr, "roster: %s\n", m)
		}
		var candidatos []roster.Model
		for _, m := range elegiveis {
			if m.ID != j.Model {
				candidatos = append(candidatos, m)
			}
		}
		nova, err := route.Escolher(candidatos, route.Dimensao(j.Dimensao), novoPercentil)
		if err != nil {
			fmt.Fprintf(stderr, "run: escalada sem re-roteio: %v\n", err)
			j.State = job.StateFailed
			_ = j.Save()
			anotaFalha(j, "escalada sem re-roteio: %v", err)
			return 1
		}
		dePara = j.Model + " -> " + nova.Modelo.ID
		motivoEscalada = d.Motivo
		j.Escaladas++
		j.Percentil = novoPercentil
		j.Model = nova.Modelo.ID
		escolha = nova
		escalou = true
		if err := j.Save(); err != nil {
			anotaFalha(j, "run: %v", err)
			fmt.Fprintf(stderr, "run: %v\n", err)
			return 1
		}
		precoIn, precoOut = precosDo(models, j.Model, stderr)

		// O revert desfaz so o que nao e teste — o teste vermelho fica
		// como reproducao — e a evidencia entra verbatim no contexto do
		// modelo mais forte (spec §6.5). Revert primeiro, evidencia
		// depois: a frase final so pode afirmar o revert que aconteceu.
		revertido := true
		if err := gitx.RevertNonTest(ctx, j.Worktree, j.TestGlobs); err != nil {
			fmt.Fprintf(stderr, "run: revertendo mudancas de nao-teste: %v\n", err)
			revertido = false
		}
		contexto = evidenciaEscalada(rep, revertido)
	}

	concluido := out.Stop == "final" && rep.Green() && rep.MutationProved
	// Veto da pre-condicao vira CancelReason persistida: a evidencia do
	// corte (sinal, probabilidade, trecho, comando de retomada) fica no
	// job.json — e no bloco CANCELADO do relatorio — em vez de evaporar
	// com o processo.
	if out.Veto != nil {
		j.CancelReason = &job.CancelReason{
			Signal:        out.Veto.Signal,
			Probability:   out.Veto.Probability,
			TurnExcerpt:   out.Veto.Excerpt,
			ResumeCommand: "delegador run --job " + j.ID,
			At:            time.Now(),
		}
	} else {
		// Retomada que terminou sem veto apaga o bloco CANCELADO anterior —
		// um motivo velho num job que completou leria como veto fresco.
		j.CancelReason = nil
	}
	if concluido {
		j.State = job.StateCompleted
	} else {
		j.State = job.StateFailed
	}
	if err := j.Save(); err != nil {
		anotaFalha(j, "run: %v", err)
		fmt.Fprintf(stderr, "run: %v\n", err)
		return 1
	}

	// O trace compactado e o noul do relatorio medem a ULTIMA tentativa —
	// e a entrega sob julgamento; a tentativa anterior fica registrada nos
	// verify-*.json e na linha de escalada. Jev fora degrada para o trace
	// integral: o relatorio nao pode morrer com a rede.
	turnos := out.Turns
	if kept, _, err := compact.Turns(ctx,
		askerContado{jevClient, jevLedger, "compactacao"}, string(briefing), out.Turns); err == nil {
		turnos = kept
	} else {
		fmt.Fprintf(stderr, "run: compactacao indisponivel, trace vai integral: %v\n", err)
	}
	afirma := 0.0
	if p, _, err := compact.AfirmaVerde(ctx,
		askerContado{jevClient, jevLedger, "relatorio"}, out.Final); err == nil {
		afirma = p
	} else {
		fmt.Fprintf(stderr, "run: noul do relatorio indisponivel: %v\n", err)
	}

	_, jevUSD, _ := jevLedger.Total()
	execUSD, _ := execLedger.Total()

	var buf bytes.Buffer
	render.Result(&buf, render.Input{
		Job: j, Verify: rep, Turns: turnos, AfirmaVerde: afirma,
		Escolha: escolha, Escalou: escalou,
		JevUSD: jevUSD, ExecutorUSD: execUSD,
	})
	if escalou {
		fmt.Fprintf(&buf, "escalou:  %s (%s)\n", dePara, motivoEscalada)
	}
	// O motivo da cascata so entra quando a decisao nao foi "verde e
	// pronto" — num verde pos-escalada o "vai para o humano" do teto
	// leria como falha que nao houve.
	if !concluido && motivoCascata != "" {
		fmt.Fprintf(&buf, "cascata:  %s\n", motivoCascata)
	}
	if err := os.WriteFile(j.Path("result.txt"), buf.Bytes(), 0o644); err != nil {
		fmt.Fprintf(stderr, "run: gravando result.txt: %v\n", err)
	}
	_, _ = stdout.Write(buf.Bytes())

	if !concluido {
		return 1
	}
	return 0
}
