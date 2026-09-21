// runhelpers_test.go — o ambiente do run sem rede: repo git com um modulo
// Go quebrado de verdade, job persistido como o plan deixaria, roster com
// um barato e um forte elegiveis, e os dois servidores falsos dirigindo o
// laco por cenario.
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/heliowap/delegador/internal/job"
	"github.com/heliowap/delegador/internal/testsupport"
)

// runEnv e o que o teste precisa do ambiente montado.
type runEnv struct {
	JobID    string
	Worktree string
	Requests *[]testsupport.Request // o que a API fake recebeu, na ordem
}

// setupRunEnv monta o cenario inteiro: temporarios para estado e worktree,
// chaves falsas, Jev e proxy executivo de mentira, e o job salvo como o
// plan o deixaria (politica, papeis de comando e registro de rota).
func setupRunEnv(t *testing.T, cenario testsupport.Scenario) runEnv {
	t.Helper()

	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TYPESAFE_API_KEY", "k")
	// Os nouls que o run pergunta: o watchdog do laco, os dois da
	// compactacao e o do relatorio. Sem override o fake acusa pergunta
	// desconhecida; os valores mantem tudo no trace e nao vetam nada.
	t.Setenv("TYPESAFE_BASE_URL", testsupport.StartFakeJev(t, map[string]float64{
		"sem_progresso":                 0.1,
		"chamada_necessaria":            0.9,
		"resultado_necessario_verbatim": 0.9,
		"relatorio_afirma_verde":        0.9,
	}))
	baseURL, requests := testsupport.StartFakeAPI(t, cenario)
	t.Setenv("DELEGADOR_BASE_URL", baseURL)
	t.Setenv("DELEGADOR_ROSTER", escreveRosterDoisModelos(t))

	worktree := iniciaRepoRun(t)

	j, err := job.Create(worktree)
	if err != nil {
		t.Fatalf("job.Create: %v", err)
	}
	// Os campos que o run remonta, como o plan os grava.
	j.Model = "barato"
	j.WritePrefixes = []string{""}
	j.AllowCommands = []string{"go test ./..."}
	j.TestCmd = "go test ./..."
	j.TestGlobs = []string{"*_test.go"}
	j.Dimensao = "mecanica"
	j.Percentil = 0
	if err := j.Save(); err != nil {
		t.Fatalf("j.Save: %v", err)
	}
	briefing := "# Tarefa\n\nSoma(a, b) esta subtraindo. Escreva o teste que prova o " +
		"defeito, corrija soma.go e rode `go test ./...`. Nao commite.\n"
	if err := os.WriteFile(j.Path("briefing.md"), []byte(briefing), 0o644); err != nil {
		t.Fatal(err)
	}
	return runEnv{JobID: j.ID, Worktree: worktree, Requests: requests}
}

// escreveRosterDoisModelos grava um roster com dois elegiveis no formato
// que roster.Load parseia: o barato tem indice baixo e custo baixo, o
// forte indice alto e custo alto — o corte de 0.25 da escalada tira o
// barato do conjunto e so o forte passa.
func escreveRosterDoisModelos(t *testing.T) string {
	t.Helper()
	yaml := "as_of_sondagem: \"" + time.Now().Format("2006-01-02") + "\"\n" +
		"modelos:\n" +
		"  - id: barato\n" +
		"    papel: barato\n" +
		"    sondado:\n" +
		"      tool_call: true\n" +
		"    benchmark:\n" +
		"      coding_index: 50\n" +
		"      custo_por_tarefa_usd: 0.01\n" +
		"    humano:\n" +
		"      custo_usd_por_mtok: 0.10\n" +
		"      habilitado: true\n" +
		"  - id: forte\n" +
		"    papel: forte\n" +
		"    sondado:\n" +
		"      tool_call: true\n" +
		"    benchmark:\n" +
		"      coding_index: 90\n" +
		"      custo_por_tarefa_usd: 0.50\n" +
		"    humano:\n" +
		"      custo_usd_por_mtok: 5.00\n" +
		"      habilitado: true\n"
	p := filepath.Join(t.TempDir(), "roster.yaml")
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// escreveRosterUmModelo grava um roster com um unico elegivel: com a
// verificacao vermelha a cascata pede escalada e a rota esgota — o run
// sai pelo retorno cedo de route.Escolher, que e o caminho que o teste
// de CancelReason quer exercitar.
func escreveRosterUmModelo(t *testing.T) string {
	t.Helper()
	yaml := "as_of_sondagem: \"" + time.Now().Format("2006-01-02") + "\"\n" +
		"modelos:\n" +
		"  - id: barato\n" +
		"    papel: barato\n" +
		"    sondado:\n" +
		"      tool_call: true\n" +
		"    benchmark:\n" +
		"      coding_index: 50\n" +
		"      custo_por_tarefa_usd: 0.01\n" +
		"    humano:\n" +
		"      custo_usd_por_mtok: 0.10\n" +
		"      habilitado: true\n"
	p := filepath.Join(t.TempDir(), "roster.yaml")
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// iniciaRepoRun sobe um repo git minimo com um modulo Go real: Soma
// subtrai em vez de somar e o unico teste commitado e fraco — quem escreve
// o teste que prova o defeito e o executor, como manda o protocolo.
func iniciaRepoRun(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	grava := func(nome, conteudo string) {
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(conteudo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	grava("go.mod", "module exemplo\n\ngo 1.27\n")
	grava("soma.go", "package soma\n\n// Soma devolve a soma de a e b.\nfunc Soma(a, b int) int {\n\treturn a - b // defeito: subtrai\n}\n")
	grava("soma_test.go", "package soma\n\nimport \"testing\"\n\n// Teste fraco de fabrica: o teste que prova o defeito quem escreve e o executor.\nfunc TestSomaZero(t *testing.T) {\n\tif Soma(0, 0) != 0 {\n\t\tt.Fatal(\"Soma(0,0) deveria ser 0\")\n\t}\n}\n")

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "teste@delegador")
	run("config", "user.name", "teste")
	run("add", ".")
	run("commit", "-m", "inicial")
	return dir
}

// As respostas roteirizadas escrevem arquivos de verdade: o Arguments e
// JSON e o conteudo viaja escapado — \n e \" dentro do literal crudo sao
// os escapes que o JSON espera.
var (
	argsTesteReal = `{"path":"soma_test.go","content":"package soma\n\nimport \"testing\"\n\nfunc TestSomaSoma(t *testing.T) {\n\tif got := Soma(2, 3); got != 5 {\n\t\tt.Errorf(\"Soma(2,3) = %d, quero 5\", got)\n\t}\n}\n"}`
	argsCorrecao  = `{"path":"soma.go","content":"package soma\n\n// Soma devolve a soma de a e b.\nfunc Soma(a, b int) int {\n\treturn a + b\n}\n"}`
	// A "correcao" errada troca subtracao por multiplicacao: o teste real
	// continua vermelho (2*3=6, nao 5), e a cascata tem falha provada.
	argsCorrecaoErrada = `{"path":"soma.go","content":"package soma\n\n// Soma devolve a soma de a e b.\nfunc Soma(a, b int) int {\n\treturn a * b\n}\n"}`
	argsGoTest         = `{"command":"go test ./..."}`
)

// cenarioQueEscreveTesteECorrige e o caminho feliz: o barato escreve o
// teste que prova o defeito e a correcao certa, roda o teste e encerra.
// A sonda de mutacao prova — sem a correcao o teste fica vermelho.
func cenarioQueEscreveTesteECorrige() testsupport.Scenario {
	return testsupport.Scenario{
		{Content: "Escrevo o teste que prova o defeito e a correcao.",
			ToolCalls: []testsupport.ToolCall{
				{ID: "w1", Name: "write_file", Arguments: argsTesteReal},
				{ID: "w2", Name: "write_file", Arguments: argsCorrecao},
			}},
		{Content: "Rodo o teste para conferir.",
			ToolCalls: []testsupport.ToolCall{
				{ID: "e1", Name: "exec", Arguments: argsGoTest},
			}},
		{Content: "Teste escrito, correcao aplicada e go test ./... verde."},
	}
}

// cenarioQueFalhaDepoisPassa e a cascata: o barato entrega uma "correcao"
// que deixa o teste vermelho (verificacao reprova), o revert mantem o
// teste e desfaz so a correcao, e o forte escreve a soma de verdade.
func cenarioQueFalhaDepoisPassa() testsupport.Scenario {
	return testsupport.Scenario{
		// Tentativa 1 (barato): teste real + correcao errada.
		{Content: "Escrevo o teste e a correcao.",
			ToolCalls: []testsupport.ToolCall{
				{ID: "w1", Name: "write_file", Arguments: argsTesteReal},
				{ID: "w2", Name: "write_file", Arguments: argsCorrecaoErrada},
			}},
		{Content: "Pronto, corrigi o Soma."},
		// Tentativa 2 (forte): so a correcao certa — o teste do barato
		// ficou na worktree como reproducao.
		{Content: "A tentativa anterior multiplicava em vez de somar. Corrijo.",
			ToolCalls: []testsupport.ToolCall{
				{ID: "w3", Name: "write_file", Arguments: argsCorrecao},
			}},
		{Content: "Corrigido: Soma soma, e o teste escrito pelo barato prova."},
	}
}
