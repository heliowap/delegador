package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/heliowap/delegador/internal/gate"
	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/job"
	"github.com/heliowap/delegador/internal/roster"
	"github.com/heliowap/delegador/internal/route"
)

// ExitRejected e o codigo de saida quando o gate reprova a tarefa.
const ExitRejected = 3

// sondagemMaxIdade e a validade da sondagem de viabilidade do roster; mesma
// convencao dos testes do roster. Quem remede e o `doctor --probe`.
const sondagemMaxIdade = 30 * 24 * time.Hour

// defaultTestGlobs e o conjunto que o v1 usava inline no subcomando result:
// cobre go, pytest e os padroes de teste de JS/TS.
var defaultTestGlobs = []string{"*_test.go", "test_*.py", "*.test.ts", "*.spec.ts"}

type planOutput struct {
	gate.Verdict
	JobID       string  `json:"job_id"`
	Briefing    string  `json:"briefing_path"`
	Model       string  `json:"modelo"`
	Dimensao    string  `json:"dimensao,omitempty"`
	NaoMedido   bool    `json:"modelo_nao_medido,omitempty"`
	JevUSD      float64 `json:"custo_jev_usd"`
	EvidenceIn  int     `json:"evidencias_recebidas"`
	EvidenceOut int     `json:"evidencias_mantidas"`
	Volume      float64 `json:"volume,omitempty"`
	MaxTurns    int     `json:"max_turns,omitempty"`
	CostCapUSD  float64 `json:"teto_usd,omitempty"`
}

// defaultRosterPath localiza o config/roster.yaml do modulo delegador pelo
// caminho fonte deste arquivo. O roster e configuracao do delegador, nao do
// repositorio alvo, entao nao se resolve por --worktree nem por cwd — o v1
// localizava recursos por flags e esta e a raiz estavel equivalente.
func defaultRosterPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "config", "roster.yaml")
}

func runPlan(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		task          = fs.String("task", "", "o defeito em uma frase")
		evidence      = fs.String("evidence", "", "JSONL de evidencias verbatim")
		worktree      = fs.String("worktree", "", "worktree isolada desta tarefa")
		testCmd       = fs.String("test-cmd", "", "comando de teste, copiavel")
		suiteCmd      = fs.String("suite-cmd", "", "comando da suite do pacote tocado")
		lintCmd       = fs.String("lint-cmd", "", "comando de lint do repositorio")
		venv          = fs.String("venv", "", "caminho absoluto do venv a reusar")
		nodeMods      = fs.String("node-modules", "", "caminho de node_modules a reusar")
		branch        = fs.String("branch", "", "branch da worktree, para registro")
		rosterF       = fs.String("roster", "", "caminho do roster.yaml (padrao: o do modulo delegador)")
		testesProntos = fs.Bool("testes-prontos", false,
			"os testes ja estao no disco e sao o criterio; o briefing pede implementacao, nao teste")
		tetoUSD = fs.Float64("teto-usd", 0,
			"teto de custo por tentativa, em dolares; 0 usa o teto derivado do volume")
		asJSON = fs.Bool("json", false, "saida em JSON")
	)
	var testGlobs []string
	fs.Func("test-glob", "glob de arquivo de teste para a sonda de mutacao (repetivel)", func(s string) error {
		testGlobs = append(testGlobs, s)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if len(testGlobs) == 0 {
		testGlobs = defaultTestGlobs
	}
	if *task == "" || *worktree == "" {
		fmt.Fprintln(stderr, "plan: --task e --worktree sao obrigatorios")
		return ExitUsage
	}

	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		fmt.Fprintln(stderr, "plan: TYPESAFE_API_KEY ausente. Sem ela os gates nao rodam, "+
			"e despachar sem gate e o que este plugin existe para evitar.")
		return 1
	}

	client := jev.New(jev.Options{APIKey: key, BaseURL: os.Getenv("TYPESAFE_BASE_URL")})

	var items []gate.Evidence
	if *evidence != "" {
		f, err := os.Open(*evidence)
		if err != nil {
			fmt.Fprintf(stderr, "plan: abrindo evidencias: %v\n", err)
			return 1
		}
		items, err = gate.ParseEvidence(f)
		f.Close()
		if err != nil {
			fmt.Fprintf(stderr, "plan: %v\n", err)
			return 1
		}
	}

	j, err := job.Create(*worktree)
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return 1
	}
	ledger := &jev.Ledger{Path: j.Path("jev.jsonl")}

	kept, usage, err := gate.SelectEvidence(ctx, client, *task, items)
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		_ = job.Release(j.ID)
		return 1
	}
	// Gravacao de ledger falha sem parar o plan — mas avisa: auditoria que
	// falha em silencio mente por omissao.
	if err := ledger.Record("selecao_evidencia", usage); err != nil {
		fmt.Fprintf(stderr, "plan: gravando jev.jsonl (selecao_evidencia): %v\n", err)
	}

	briefing := gate.BuildBriefing(*task, kept, gate.BriefingLimits{
		TestCmd: *testCmd, SuiteCmd: *suiteCmd, LintCmd: *lintCmd,
		VenvPath: *venv, NodeModulesPath: *nodeMods,
		TestesJaEscritos: *testesProntos, ArquivosDeTeste: testesNaWorktree(*worktree, testGlobs),
	})
	if err := os.WriteFile(j.Path("briefing.md"), []byte(briefing), 0o644); err != nil {
		fmt.Fprintf(stderr, "plan: gravando briefing: %v\n", err)
		_ = job.Release(j.ID)
		return 1
	}

	verdict, usage, err := gate.Check(ctx, client, *task, briefing, gate.RepoFacts{})
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		_ = job.Release(j.ID)
		return 1
	}
	if err := ledger.Record("gates", usage); err != nil {
		fmt.Fprintf(stderr, "plan: gravando jev.jsonl (gates): %v\n", err)
	}

	// Guarda a evidencia recebida no job, com a marca do que sobreviveu
	// (spec §10): sem isso nao da para auditar uma selecao depois.
	if f, err := os.Create(j.Path("evidence.jsonl")); err == nil {
		enc := json.NewEncoder(f)
		for _, e := range items {
			_ = enc.Encode(e)
		}
		f.Close()
	}

	var (
		dimensao  route.Dimensao
		naoMedido bool
		volume    float64
	)
	if verdict.Delegable {
		path := *rosterF
		if path == "" {
			path = os.Getenv("DELEGADOR_ROSTER")
		}
		if path == "" {
			path = defaultRosterPath()
		}
		models, err := roster.Load(path)
		if err != nil {
			fmt.Fprintf(stderr, "plan: %v\n", err)
			_ = job.Release(j.ID)
			return 1
		}
		elegiveis, motivos := roster.Elegiveis(models, sondagemMaxIdade, time.Now())
		for _, m := range motivos {
			fmt.Fprintf(stderr, "roster: %s\n", m)
		}
		if len(elegiveis) == 0 {
			fmt.Fprintln(stderr, "plan: nenhum modelo elegivel no roster; os motivos estao acima")
			_ = job.Release(j.ID)
			return 1
		}

		cls, usage, err := route.Classificar(ctx, client, briefing)
		if err != nil {
			fmt.Fprintf(stderr, "plan: %v\n", err)
			_ = job.Release(j.ID)
			return 1
		}
		if err := ledger.Record("rota", usage); err != nil {
			fmt.Fprintf(stderr, "plan: gravando jev.jsonl (rota): %v\n", err)
		}
		// As respostas inteiras vao para o disco antes de qualquer decisao:
		// probabilidade, confianca e distribuicao de cada pergunta, mais os
		// atomos de complexidade que ainda nao decidem nada. E o que permite
		// mudar um peso da rota e refazer a conta sobre os jobs ja rodados,
		// em vez de reexecutar. Falhar ao gravar nao para o plano — o job
		// perde a auditoria, nao o trabalho.
		gravaClassificacao(j.Path("classificacao.json"), cls, stderr)

		// Dois ajustes que nao vem da dimensao: a autocontencao, porque
		// tarefa com decisao em aberto exige modelo que sustente o
		// enquadramento sozinho; e a CONFIANCA da propria escolha de
		// dimensao, porque cortar por um eixo escolhido no chute e pior que
		// nao cortar.
		escolha, err := route.EscolherCom(elegiveis, cls.Dimensao, cls.Percentil,
			route.Opcoes{Autocontida: verdict.Autocontida,
				ConfiancaDimensao: cls.ConfiancaDimensao})
		if err != nil {
			fmt.Fprintf(stderr, "plan: %v\n", err)
			_ = job.Release(j.ID)
			return 1
		}
		dimensao, naoMedido = escolha.Dimensao, escolha.NaoMedido
		if escolha.Motivo != "" {
			fmt.Fprintf(stderr, "rota: %s\n", escolha.Motivo)
		}

		j.Model, j.Branch = escolha.Modelo.ID, *branch
		// O tools.Policy do run sai daqui: escrita dentro da worktree (o
		// briefing limita por proibicao em prosa; a allowlist nao tem negacao)
		// e os comandos sao os declarados no briefing.
		j.WritePrefixes = []string{""}
		j.AllowCommands = nonEmpty(*testCmd, *suiteCmd, *lintCmd)
		// O verify.Config do run sai daqui tambem: os papeis dos comandos e
		// os globs de teste da sonda de mutacao.
		j.TestCmd, j.SuiteCmd, j.LintCmd = *testCmd, *suiteCmd, *lintCmd
		j.TestGlobs = testGlobs
		// O registro da rota: a cascata eleva o corte sem repreguntar ao Jev.
		j.Percentil, j.Dimensao = escolha.Percentil, string(escolha.Dimensao)
		j.Autocontida = verdict.Autocontida
		// O orcamento do laco sai do VOLUME, nao da complexidade: uma
		// migracao trivial em trinta arquivos e barata por sitio e cara no
		// total, e cravar 30 turnos para toda tarefa errava os dois extremos.
		orc := route.OrcamentoPara(cls.Volume, route.OrcamentoBase())
		j.MaxTurns, j.CostCapUSD = orc.Turnos, orc.TetoUSD
		j.IdleTurns = orc.TurnosOciosos
		// O teto medido pelo volume e um palpite sobre o tamanho da tarefa,
		// e quando ele erra o run morre com "CANCELADO: teto_de_custo" e a
		// instrucao de levantar o teto — que ate 2026-09-21 nao tinha como
		// ser seguida. Esta flag e a resposta a essa instrucao: o numero e
		// de quem opera, e o volume so decide quando ninguem disse nada.
		if *tetoUSD > 0 {
			j.CostCapUSD = *tetoUSD
		}
		volume = cls.Volume
		if err := j.Save(); err != nil {
			fmt.Fprintf(stderr, "plan: %v\n", err)
			_ = job.Release(j.ID)
			return 1
		}
	}

	_, usd, _ := ledger.Total()
	out := planOutput{
		Verdict: verdict, JobID: j.ID, Briefing: j.Path("briefing.md"),
		Model: j.Model, Dimensao: string(dimensao), NaoMedido: naoMedido,
		JevUSD: usd, EvidenceIn: len(items), EvidenceOut: len(kept),
		Volume: volume, MaxTurns: j.MaxTurns, CostCapUSD: j.CostCapUSD,
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
	} else {
		renderPlan(stdout, out)
	}

	if !verdict.Delegable {
		// O job fica gravado como trilha de auditoria da reprova — o
		// veredito, a evidencia e o briefing que o gate leu. cancelled,
		// nao failed: quem decidiu foi um veredito, nao uma falha de
		// execucao — e o estado terminal tira o job da fila do run.
		j.State = job.StateCancelled
		if err := j.Save(); err != nil {
			fmt.Fprintf(stderr, "plan: gravando estado cancelado: %v\n", err)
		}
		_ = job.Release(j.ID)
		return ExitRejected
	}
	return 0
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func renderPlan(w io.Writer, o planOutput) {
	fmt.Fprintf(w, "job:         %s\n", o.JobID)
	fmt.Fprintf(w, "tipo:        %s\n", o.Kind)
	fmt.Fprintf(w, "evidencia:   %d recebidas, %d mantidas\n", o.EvidenceIn, o.EvidenceOut)
	fmt.Fprintf(w, "autocontida: %.2f\n", o.Autocontida)
	if o.MaxTurns > 0 {
		fmt.Fprintf(w, "orcamento:   volume %.2f -> %d turnos, teto $%.2f\n",
			o.Volume, o.MaxTurns, o.CostCapUSD)
	}
	fmt.Fprintf(w, "jev:         $%.5f\n", o.JevUSD)
	if len(o.Warnings) > 0 {
		fmt.Fprintf(w, "avisos:      %v\n", o.Warnings)
	}
	if !o.Delegable {
		fmt.Fprintf(w, "\nREPROVADO. Corrija e rode de novo: %v\n", o.Missing)
		// tarefa_autocontida reprovando sozinha e o caso menos obvio da
		// lista: nao e um item faltando no briefing, e uma decisao que
		// ninguem tomou. Sem esta linha o operador procura o que escrever
		// a mais, quando o que falta e escolher.
		for _, m := range o.Missing {
			if m == "tarefa_autocontida" {
				fmt.Fprintf(w, "\n  autocontida %.2f: as decisoes da tarefa estao em aberto.\n", o.Autocontida)
				fmt.Fprintln(w, "  Nenhum modelo resolve isso — delegar aqui nao devolve uma resposta")
				fmt.Fprintln(w, "  pior, devolve a resposta de outra pergunta. Feche o que ainda e")
				fmt.Fprintln(w, "  escolha sua (qual comportamento e o certo, onde intervir) e delegue")
				fmt.Fprintln(w, "  a execucao.")
				break
			}
		}
		return
	}
	fmt.Fprintf(w, "modelo:      %s (dimensao %s)\n", o.Model, o.Dimensao)
	if o.NaoMedido {
		fmt.Fprintln(w, "             sem benchmark de terceiro: entrou por viabilidade e custo")
	}
	fmt.Fprintf(w, "briefing:    %s\n", o.Briefing)
}

// testesNaWorktree lista os arquivos de teste que casam com os globs, para o
// briefing poder nomear o oraculo em vez de falar dele no abstrato.
func testesNaWorktree(dir string, globs []string) []string {
	var achados []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if err == nil && info.IsDir() && info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		for _, g := range globs {
			if ok, _ := filepath.Match(g, info.Name()); ok {
				if rel, err := filepath.Rel(dir, p); err == nil {
					achados = append(achados, rel)
				}
				return nil
			}
		}
		return nil
	})
	sort.Strings(achados)
	// Ou a lista e COMPLETA, ou nao vai. Truncar em seis devolvia os seis
	// primeiros em ordem alfabetica, e o briefing os anunciava como criterio
	// de aceite: medido em 2026-09-21 num repositorio real, nenhum dos seis
	// tinha relacao com a tarefa. Sem lista, o briefing manda o executor
	// olhar a evidencia e o comando de teste, que apontam os certos.
	if len(achados) > 6 {
		return nil
	}
	return achados
}
