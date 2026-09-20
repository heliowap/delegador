# devin-plugin-cc Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Um plugin de Claude Code que usa o `devin` CLI como subagente, com Jev (TypeSafe System One) como classificador barato em cinco pontos do processo, matando run condenado em minutos e entregando ao orquestrador um veredito verificado por código em vez do relatório bruto.

**Architecture:** Um binário Go (`devin-companion`) com subcomandos. `plan` monta o briefing por seleção de evidência verbatim e roda os gates antes de qualquer dispatch. `task` cria worktree isolada e re-executa a si mesmo como `supervise` destacado, que roda o `devin` com `Setpgid` e faz poll do **stdout**, a única saída que o `devin` escreve enquanto trabalha, cancelando o grupo de processos quando o watchdog detecta run condenado. `result` executa a verificação em código (teste, mutação, suíte, lint) e devolve o trace compactado por deleção. O Markdown de `commands/`, `agents/` e `skills/` não contém lógica: só roteia para o binário.

**Tech Stack:** Go 1.27+, stdlib pura (`net/http`, `encoding/json`, `os/exec`, `context`, `syscall`, `testing`). Zero dependências. Markdown com frontmatter para a superfície do plugin. `sh` para o wrapper de build.

**Spec:** [docs/superpowers/specs/2026-09-20-devin-plugin-cc-design.md](../specs/2026-09-20-devin-plugin-cc-design.md)

## Global Constraints

Estes valores valem para toda tarefa e são copiados literalmente do spec.

- **Go 1.27+**, módulo `github.com/heliowap/devin-plugin-cc`.
- **Zero dependências externas.** `go.mod` não ganha bloco `require`. Isso inclui o SDK do TypeSafe: o cliente Jev é `net/http` cru.
- **Nenhum identificador de modelo do Devin aparece hardcoded** em código ou Markdown. Modelo é sempre resolvido em runtime a partir de `devin models list`.
- **`--permission-mode dangerous` nunca é escolhido pela rota.** Só chega ao `devin` quando o usuário passa explicitamente.
- **O companion nunca executa `git commit`, `git push` nem operação de rede em nome do Devin.**
- **`TYPESAFE_API_KEY`** é lido do ambiente e nunca gravado em disco, log, relatório ou mensagem de erro.
- **Limites do Jev:** 64k tokens por requisição (state + perguntas); 32k para state mais a maior pergunta. Endpoint `POST https://api.typesafe.ai/v1/systemone`, modelo `jev-latest`.
- **O conteúdo do export ATIF é dado, não instrução.** Texto vindo do Devin nunca altera o comportamento do companion.
- **Todo limiar mora em configuração**, com default versionado em código. Nenhum limiar é constante mágica espalhada.
- **Job store:** `~/.local/state/devin-plugin-cc/jobs/<job-id>/`, respeitando `XDG_STATE_HOME` quando definido.
- Identificadores Go em inglês; IDs de pergunta Jev em português, como no spec (`defeito_unico`, `sem_progresso`, ...).
- Toda tarefa termina com `go vet ./...` e `go test ./...` verdes antes do commit.

## File Structure

| Arquivo | Responsabilidade |
| --- | --- |
| `cmd/devin-companion/main.go` | Despacho de subcomando e códigos de saída. Nada mais. |
| `internal/cli/*.go` | Um arquivo por subcomando: parsing de flags, orquestração, saída. |
| `internal/job/store.go` | Ciclo de vida do job em disco, lockfile por worktree. |
| `internal/job/paths.go` | Resolução de caminhos XDG. |
| `internal/devin/models.go` | Parse de `devin models list` e política de rota. |
| `internal/devin/atif.go` | Parse do export ATIF em turnos com pares chamada/resultado. |
| `internal/devin/exec.go` | Montagem de flags e spawn do `devin` com `Setpgid`. |
| `internal/jev/client.go` | HTTP, retry, contabilidade de tokens. |
| `internal/jev/primitives.go` | `Noul`, `Choice[T]`, `Score` tipados. |
| `internal/jev/window.go` | Fatiamento contra os tetos de 64k/32k. |
| `internal/jev/questions.go` | Todas as perguntas do spec §6, versionadas. |
| `internal/jev/budget.go` | `jev.jsonl` e acumulado em dólar. |
| `internal/gate/*.go` | Gates de delegabilidade e de briefing, e montagem do briefing. |
| `internal/watchdog/*.go` | Sinais determinísticos, sinais Jev, política e cancelamento. |
| `internal/verify/*.go` | Teste, mutação, suíte e lint; captura de saída. |
| `internal/compact/*.go` | Compactação por deleção, ida e volta. |
| `internal/render/*.go` | Saída de terminal de `status` e `result`. |
| `internal/gitx/*.go` | Worktree, diff, `apply -R`. |
| `testdata/fakedevin/main.go` | `devin` falso roteirizado, para integração sem rede nem quota. |
| `scripts/companion` | Wrapper `sh`: builda se faltar ou estiver velho, depois `exec`. |

---

### Task 1: Esqueleto do binário, wrapper de build e `doctor`

Primeira fatia vertical: algo executável no fim da tarefa.

**Files:**
- Create: `go.mod`
- Create: `cmd/devin-companion/main.go`
- Create: `internal/cli/cli.go`
- Create: `internal/cli/doctor.go`
- Create: `scripts/companion`
- Test: `internal/cli/cli_test.go`, `internal/cli/doctor_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `cli.Run(ctx context.Context, args []string, stdout, stderr io.Writer) int` — ponto de entrada único, devolve código de saída. `cli.ErrUsage` para subcomando desconhecido. `doctor.Check(ctx, lookPath func(string) (string, error), run func(context.Context, string, ...string) ([]byte, error)) (Report, error)` com `Report{DevinPath, DevinVersion string; MissingFlags []string; HasTypeSafeKey bool}`.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/cli/cli_test.go
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunUnknownSubcommandExits2(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{"frobnicate"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("exit code = %d, quero 2", code)
	}
	if !strings.Contains(errBuf.String(), "frobnicate") {
		t.Errorf("stderr nao nomeia o subcomando desconhecido: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "doctor") {
		t.Errorf("stderr nao lista os subcomandos disponiveis: %q", errBuf.String())
	}
}

func TestRunNoArgsPrintsUsage(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := Run(context.Background(), nil, &out, &errBuf); code != 2 {
		t.Fatalf("exit code = %d, quero 2", code)
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar o vermelho**

Run: `go test ./internal/cli/ -run TestRun -v`
Expected: FAIL — `undefined: Run`.

- [ ] **Step 3: Implementação mínima**

```go
// go.mod
module github.com/heliowap/devin-plugin-cc

go 1.27
```

```go
// internal/cli/cli.go
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
		fmt.Fprintf(w, "devin-companion: subcomando desconhecido %q\n\n", unknown)
	}
	names := make([]string, 0, len(hs))
	for n := range hs {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintf(w, "uso: devin-companion <subcomando> [flags]\n\nsubcomandos: %v\n", names)
}
```

```go
// cmd/devin-companion/main.go
package main

import (
	"context"
	"os"

	"github.com/heliowap/devin-plugin-cc/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/cli/ -run TestRun -v`
Expected: PASS.

- [ ] **Step 5: Escrever o teste do `doctor`**

`doctor` recebe `lookPath` e `run` como parâmetros para ser testável sem tocar o `devin` real.

```go
// internal/cli/doctor_test.go
package cli

import (
	"context"
	"errors"
	"testing"
)

func TestCheckReportsMissingDevin(t *testing.T) {
	lookPath := func(string) (string, error) { return "", errors.New("not found") }
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, nil }

	_, err := Check(context.Background(), lookPath, run)
	if err == nil {
		t.Fatal("quero erro quando o devin nao esta no PATH")
	}
}

func TestCheckDetectsMissingFlags(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/local/bin/devin", nil }
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("devin 3000.10.31 (b98cc431)\n"), nil
		}
		// help sem --export, para provar que a deteccao funciona
		return []byte("--prompt-file --permission-mode --respect-workspace-trust --model -p"), nil
	}

	rep, err := Check(context.Background(), lookPath, run)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.DevinVersion != "3000.10.31" {
		t.Errorf("DevinVersion = %q, quero 3000.10.31", rep.DevinVersion)
	}
	if len(rep.MissingFlags) != 1 || rep.MissingFlags[0] != "--export" {
		t.Errorf("MissingFlags = %v, quero [--export]", rep.MissingFlags)
	}
}

func TestCheckAllFlagsPresent(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/local/bin/devin", nil }
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("devin 3000.10.31 (b98cc431)\n"), nil
		}
		return []byte("--prompt-file --permission-mode --respect-workspace-trust --model --export -p -c"), nil
	}

	rep, err := Check(context.Background(), lookPath, run)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(rep.MissingFlags) != 0 {
		t.Errorf("MissingFlags = %v, quero vazio", rep.MissingFlags)
	}
}
```

- [ ] **Step 6: Rodar e confirmar o vermelho**

Run: `go test ./internal/cli/ -run TestCheck -v`
Expected: FAIL — `undefined: Check`.

- [ ] **Step 7: Implementar o `doctor`**

```go
// internal/cli/doctor.go
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
```

- [ ] **Step 8: Rodar e confirmar o verde**

Run: `go test ./internal/cli/ -v && go vet ./...`
Expected: PASS.

- [ ] **Step 9: Escrever o wrapper `scripts/companion`**

```sh
#!/bin/sh
# Builda o companion quando o binario falta ou esta mais velho que o fonte,
# depois executa. Mantem "clonou, funcionou" enquanto houver Go instalado.
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
bin="$root/bin/devin-companion"

needs_build=0
[ -x "$bin" ] || needs_build=1
if [ "$needs_build" -eq 0 ]; then
    newer="$(find "$root/cmd" "$root/internal" -name '*.go' -newer "$bin" -print -quit 2>/dev/null || true)"
    [ -n "$newer" ] && needs_build=1
fi

if [ "$needs_build" -eq 1 ]; then
    if ! command -v go >/dev/null 2>&1; then
        echo "devin-plugin-cc: Go nao encontrado no PATH e o binario nao esta compilado." >&2
        echo "Instale Go 1.27+ e rode /devin:setup." >&2
        exit 127
    fi
    mkdir -p "$root/bin"
    ( cd "$root" && go build -o "$bin" ./cmd/devin-companion ) >&2
fi

exec "$bin" "$@"
```

```bash
chmod +x scripts/companion
```

- [ ] **Step 10: Verificar o wrapper de ponta a ponta**

Run: `rm -rf bin && ./scripts/companion doctor; echo "exit=$?"`
Expected: compila, e imprime o diagnóstico. Com `devin` instalado e `TYPESAFE_API_KEY` no ambiente, `exit=0`.

Run: `./scripts/companion frobnicate; echo "exit=$?"`
Expected: `exit=2`, com a lista de subcomandos.

- [ ] **Step 11: Commit**

```bash
printf 'bin/\n' > .gitignore
git add go.mod cmd internal scripts .gitignore
git commit -m "feat: esqueleto do companion, wrapper de build e doctor"
```

---

### Task 2: `fakedevin` e o harness de integração

Sem um `devin` falso no `PATH`, testar watchdog e cancelamento significa queimar quota a cada execução de `go test`. Esta tarefa vem antes de tudo que depende disso.

**Files:**
- Create: `testdata/fakedevin/main.go`
- Create: `internal/testsupport/fakedevin.go`
- Test: `internal/testsupport/fakedevin_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `testsupport.InstallFakeDevin(t *testing.T, scenario string) string` — compila o `fakedevin`, instala num diretório temporário à frente do `PATH` com o nome `devin` e devolve esse diretório. Cenários: `"healthy"`, `"repeat-fail"`, `"permission-block"`, `"out-of-scope"`, `"no-progress"`.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/testsupport/fakedevin_test.go
package testsupport

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFakeDevinWritesExportIncrementally(t *testing.T) {
	InstallFakeDevin(t, "healthy")

	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	prompt := filepath.Join(dir, "briefing.md")
	if err := os.WriteFile(prompt, []byte("corrija o defeito"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("devin", "--prompt-file", prompt, "--export", export, "-p")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fakedevin falhou: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(export)
	if err != nil {
		t.Fatalf("export nao foi escrito: %v", err)
	}
	var doc struct {
		Turns []struct {
			Index int `json:"index"`
			Tools []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("export nao e JSON valido: %v", err)
	}
	if len(doc.Turns) < 3 {
		t.Fatalf("quero ao menos 3 turnos no cenario healthy, tenho %d", len(doc.Turns))
	}
}

func TestFakeDevinRespectsScenario(t *testing.T) {
	InstallFakeDevin(t, "permission-block")

	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	prompt := filepath.Join(dir, "b.md")
	if err := os.WriteFile(prompt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// No cenario permission-block o processo nao termina sozinho: ele trava.
	// O teste so confere que ele chega a escrever o turno de bloqueio.
	cmd := exec.Command("devin", "--prompt-file", prompt, "--export", export, "-p")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	waitForFile(t, export, "aguardando confirmacao")
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/testsupport/ -v`
Expected: FAIL — `undefined: InstallFakeDevin`.

- [ ] **Step 3: Implementar o `fakedevin`**

```go
// testdata/fakedevin/main.go
// Um "devin" falso e roteirizado. Escreve o export ATIF turno a turno,
// como o devin real faz, para que o watchdog possa ser testado sem rede.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

type tool struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Status string `json:"status"`
}

type turn struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
	Tools []tool `json:"tools"`
}

type doc struct {
	Turns []turn `json:"turns"`
}

func main() {
	var promptFile, export, model, permMode, trust string
	var print bool
	flag.StringVar(&promptFile, "prompt-file", "", "")
	flag.StringVar(&export, "export", "", "")
	flag.StringVar(&model, "model", "", "")
	flag.StringVar(&permMode, "permission-mode", "", "")
	flag.StringVar(&trust, "respect-workspace-trust", "", "")
	flag.BoolVar(&print, "p", false, "")
	flag.Parse()

	if os.Getenv("FAKEDEVIN_ECHO_ARGS") != "" {
		fmt.Println(os.Args[1:])
	}

	scenario := os.Getenv("FAKEDEVIN_SCENARIO")
	if scenario == "" {
		scenario = "healthy"
	}
	step := 50 * time.Millisecond
	if s := os.Getenv("FAKEDEVIN_STEP_MS"); s != "" {
		var ms int
		fmt.Sscanf(s, "%d", &ms)
		step = time.Duration(ms) * time.Millisecond
	}

	d := doc{}
	emit := func(tr turn) {
		d.Turns = append(d.Turns, tr)
		if export != "" {
			raw, _ := json.MarshalIndent(d, "", "  ")
			tmp := export + ".tmp"
			_ = os.WriteFile(tmp, raw, 0o644)
			_ = os.Rename(tmp, export) // escrita atomica, como o devin real
		}
		time.Sleep(step)
	}

	switch scenario {
	case "healthy":
		emit(turn{0, "lendo o arquivo", []tool{{"t0", "read", "pkg/svc/rota.go", "func Rota() {...}", "ok"}}})
		emit(turn{1, "escrevendo o teste", []tool{{"t1", "write", "pkg/svc/rota_test.go", "escrito", "ok"}}})
		emit(turn{2, "confirmando o vermelho", []tool{{"t2", "exec", "go test ./pkg/svc/", "FAIL: TestRota", "ok"}}})
		emit(turn{3, "corrigindo", []tool{{"t3", "edit", "pkg/svc/rota.go", "1 hunk", "ok"}}})
		emit(turn{4, "confirmando o verde", []tool{{"t4", "exec", "go test ./pkg/svc/", "ok  pkg/svc  0.4s", "ok"}}})
		fmt.Println("Relatorio: teste escrito, vermelho confirmado, correcao aplicada, verde confirmado.")
	case "repeat-fail":
		emit(turn{0, "rodando o teste", []tool{{"t0", "exec", "go test ./pkg/svc/", "FAIL: cannot find module", "erro"}}})
		for i := 1; i <= 6; i++ {
			emit(turn{i, "tentando de novo", []tool{{fmt.Sprintf("t%d", i), "exec", "go test ./pkg/svc/", "FAIL: cannot find module", "erro"}}})
		}
	case "permission-block":
		fmt.Println("Vou inspecionar o repositorio antes de comecar.")
		emit(turn{0, "lendo", []tool{{"t0", "read", "pkg/svc/rota.go", "func Rota() {...}", "ok"}}})
		// A linha literal que o devin real emite ao esbarrar numa confirmacao
		// que o modo -p nao consegue exibir. Medido contra 3000.10.31.
		fmt.Println("warning: rejected a tool call that requires confirmation. " +
			"Running in non-interactive mode. Use --permission-mode dangerous to auto-approve all tools.")
		emit(turn{1, "aguardando confirmacao para rodar o teste", []tool{{"t1", "exec", "go test ./...", "aguardando confirmacao do usuario", "pendente"}}})
		select {} // trava de proposito
	case "out-of-scope":
		emit(turn{0, "lendo", []tool{{"t0", "read", "pkg/svc/rota.go", "...", "ok"}}})
		emit(turn{1, "editando fora do escopo", []tool{{"t1", "edit", "infra/deploy.yaml", "1 hunk", "ok"}}})
		emit(turn{2, "editando fora do escopo", []tool{{"t2", "edit", "infra/secrets.tf", "1 hunk", "ok"}}})
	case "no-progress":
		for i := 0; i < 8; i++ {
			// Volume no stdout para o watchdog ter janela que valha uma pergunta.
			fmt.Printf("turno %d: explorando o repositorio de novo; rodei ls -la e nada mudou. %s\n",
				i, strings.Repeat("contexto irrelevante repetido. ", 80))
			emit(turn{i, "explorando o repositorio", []tool{{fmt.Sprintf("t%d", i), "exec", "ls -la", "total 48", "ok"}}})
		}
	default:
		fmt.Fprintf(os.Stderr, "fakedevin: cenario desconhecido %q\n", scenario)
		os.Exit(64)
	}
}
```

```go
// internal/testsupport/fakedevin.go
package testsupport

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// InstallFakeDevin compila testdata/fakedevin, instala como "devin" num
// diretorio temporario a frente do PATH e devolve esse diretorio.
func InstallFakeDevin(t *testing.T, scenario string) string {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "devin")

	cmd := exec.Command("go", "build", "-o", bin, "./testdata/fakedevin")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build do fakedevin: %v\n%s", err, out)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKEDEVIN_SCENARIO", scenario)
	return dir
}

// repoRoot sobe a partir deste arquivo ate encontrar go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller falhou")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod nao encontrado acima de " + file)
		}
		dir = parent
	}
}

// waitForFile espera ate que path exista e contenha want, ou falha.
func waitForFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil && strings.Contains(string(raw), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout esperando %q em %s", want, path)
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/testsupport/ -v`
Expected: PASS nos dois testes.

- [ ] **Step 5: Commit**

```bash
git add testdata internal/testsupport
git commit -m "test: devin falso roteirizado e harness de integracao"
```

---

### Task 3: Parse de `devin models list` e rota de modelo

**Files:**
- Create: `internal/devin/models.go`
- Create: `testdata/models-list.txt`
- Test: `internal/devin/models_test.go`

**Interfaces:**
- Consumes: nada.
- Produces:
  - `devin.Model{UID, Label, Family string; ContextTokens int; Free bool; InputUSDPerM float64}`
  - `devin.ParseModelList(r io.Reader) ([]Model, error)` — tolerante: linha que não casa é ignorada.
  - `devin.Effort` com `EffortMedium`, `EffortHigh`, `EffortMax`.
  - `devin.SelectModel(models []Model, want Effort) (Model, error)` — prefere gratuito; entre pagos, o menor `InputUSDPerM`; empata pelo maior `ContextTokens`.

- [ ] **Step 1: Criar a fixture**

```bash
mkdir -p testdata
cat > testdata/models-list.txt <<'EOF'
Available models (48 families)

Claude Opus 5 (claude-opus-5)
  aliases: opus
  claude-opus-5-medium                    Claude Opus 5 Medium  [1M context, $5 / 1M Input · $0.5 / 1M Cached input · $25 / 1M Output]
  claude-opus-5-max                       Claude Opus 5 Max  [1M context, $5 / 1M Input · $0.5 / 1M Cached input · $25 / 1M Output]

SWE-2 (swe-2)
  aliases: swe
  swe-2-medium                            SWE-2 Medium  [262K context, Free]
  swe-2-high                              SWE-2 High  [262K context, Free]
  swe-2-max                               SWE-2 Max  [262K context, Free]

GPT-5.6 Luna (gpt-5.6-luna)
  gpt-5-6-luna-medium                     GPT-5.6 Luna Medium Thinking  [1M context, $0.2 / 1M Input · $0.02 / 1M Cached input · $1.2 / 1M Output]

Quantum Flux 9 (quantum-flux-9)
  qf9-warp                                Quantum Flux 9 Warp  [formato que ainda nao existe]

Pass a family slug, alias, or model UID to `--model` (e.g. `--model opus`)
EOF
```

- [ ] **Step 2: Escrever o teste que falha**

```go
// internal/devin/models_test.go
package devin

import (
	"os"
	"testing"
)

func fixture(t *testing.T) []Model {
	t.Helper()
	f, err := os.Open("../../testdata/models-list.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	models, err := ParseModelList(f)
	if err != nil {
		t.Fatalf("ParseModelList: %v", err)
	}
	return models
}

func TestParseModelListReadsFreeAndPaid(t *testing.T) {
	models := fixture(t)

	byUID := map[string]Model{}
	for _, m := range models {
		byUID[m.UID] = m
	}

	swe, ok := byUID["swe-2-max"]
	if !ok {
		t.Fatal("swe-2-max ausente")
	}
	if !swe.Free {
		t.Error("swe-2-max deveria ser Free")
	}
	if swe.ContextTokens != 262_000 {
		t.Errorf("ContextTokens = %d, quero 262000", swe.ContextTokens)
	}
	if swe.Family != "swe-2" {
		t.Errorf("Family = %q, quero swe-2", swe.Family)
	}

	opus, ok := byUID["claude-opus-5-max"]
	if !ok {
		t.Fatal("claude-opus-5-max ausente")
	}
	if opus.Free {
		t.Error("claude-opus-5-max nao e gratuito")
	}
	if opus.InputUSDPerM != 5 {
		t.Errorf("InputUSDPerM = %v, quero 5", opus.InputUSDPerM)
	}
	if opus.ContextTokens != 1_000_000 {
		t.Errorf("ContextTokens = %d, quero 1000000", opus.ContextTokens)
	}
}

// Linha em formato desconhecido nao pode derrubar o parser: o devin ganha
// familias novas com frequencia e o plugin precisa sobreviver a isso.
func TestParseModelListIgnoresUnknownFormat(t *testing.T) {
	for _, m := range fixture(t) {
		if m.UID == "qf9-warp" {
			t.Error("linha em formato desconhecido nao deveria virar Model")
		}
	}
}

func TestSelectModelPrefersFree(t *testing.T) {
	got, err := SelectModel(fixture(t), EffortMax)
	if err != nil {
		t.Fatalf("SelectModel: %v", err)
	}
	if got.UID != "swe-2-max" {
		t.Errorf("UID = %q, quero swe-2-max (gratuito vence)", got.UID)
	}
}

func TestSelectModelFallsBackToCheapestPaid(t *testing.T) {
	var paid []Model
	for _, m := range fixture(t) {
		if !m.Free {
			paid = append(paid, m)
		}
	}
	got, err := SelectModel(paid, EffortMedium)
	if err != nil {
		t.Fatalf("SelectModel: %v", err)
	}
	if got.UID != "gpt-5-6-luna-medium" {
		t.Errorf("UID = %q, quero gpt-5-6-luna-medium (mais barato)", got.UID)
	}
}

func TestSelectModelErrorsOnEmpty(t *testing.T) {
	if _, err := SelectModel(nil, EffortMax); err == nil {
		t.Fatal("quero erro com lista vazia")
	}
}
```

- [ ] **Step 3: Rodar e confirmar o vermelho**

Run: `go test ./internal/devin/ -v`
Expected: FAIL — `undefined: ParseModelList`.

- [ ] **Step 4: Implementar**

```go
// internal/devin/models.go
package devin

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Effort e o nivel de esforco pedido, traduzido em sufixo de modelo.
type Effort string

const (
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
	EffortMax    Effort = "max"
)

// Model e uma entrada de `devin models list`.
type Model struct {
	UID           string
	Label         string
	Family        string
	ContextTokens int
	Free          bool
	InputUSDPerM  float64
}

var (
	familyRe = regexp.MustCompile(`^(\S.*)\s+\(([a-z0-9][a-z0-9.\-]*)\)\s*$`)
	modelRe  = regexp.MustCompile(`^\s{2,}(\S+)\s{2,}(.+?)\s*\[([^\]]*)\]\s*$`)
	ctxRe    = regexp.MustCompile(`([\d,]+)\s*(K|M)?\s*context`)
	priceRe  = regexp.MustCompile(`\$([\d.]+)\s*/\s*1M\s+Input`)
)

// ParseModelList le a saida de `devin models list`. Linhas que nao casam com
// o formato esperado sao ignoradas em silencio: familias novas nao podem
// derrubar o plugin.
func ParseModelList(r io.Reader) ([]Model, error) {
	var models []Model
	var family string

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "Available models") || strings.HasPrefix(line, "Pass a family") {
			continue
		}
		if m := familyRe.FindStringSubmatch(line); m != nil {
			family = m[2]
			continue
		}
		m := modelRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		attrs := m[3]
		ctx := ctxRe.FindStringSubmatch(attrs)
		if ctx == nil {
			continue // formato desconhecido: ignora
		}
		tokens, err := parseContext(ctx[1], ctx[2])
		if err != nil {
			continue
		}
		mod := Model{
			UID:           m[1],
			Label:         strings.TrimSpace(m[2]),
			Family:        family,
			ContextTokens: tokens,
			Free:          strings.Contains(attrs, "Free"),
		}
		if p := priceRe.FindStringSubmatch(attrs); p != nil {
			mod.InputUSDPerM, _ = strconv.ParseFloat(p[1], 64)
		}
		models = append(models, mod)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("lendo models list: %w", err)
	}
	return models, nil
}

func parseContext(num, unit string) (int, error) {
	n, err := strconv.Atoi(strings.ReplaceAll(num, ",", ""))
	if err != nil {
		return 0, err
	}
	switch unit {
	case "K":
		return n * 1000, nil
	case "M":
		return n * 1_000_000, nil
	default:
		return n, nil
	}
}

// ErrNoModel indica que nenhum modelo atende ao esforco pedido.
var ErrNoModel = errors.New("nenhum modelo disponivel para o esforco pedido")

// SelectModel escolhe o modelo para o esforco pedido: gratuito primeiro,
// depois o mais barato por token de entrada, desempatando pelo maior contexto.
// Nenhum UID aparece hardcoded — a escolha vem sempre da lista viva.
func SelectModel(models []Model, want Effort) (Model, error) {
	var cands []Model
	for _, m := range models {
		if strings.HasSuffix(m.UID, "-"+string(want)) {
			cands = append(cands, m)
		}
	}
	if len(cands) == 0 {
		// Familia sem sufixo de esforco: aceita qualquer uma, para nao travar.
		cands = models
	}
	if len(cands) == 0 {
		return Model{}, ErrNoModel
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.Free != b.Free {
			return a.Free
		}
		if a.InputUSDPerM != b.InputUSDPerM {
			return a.InputUSDPerM < b.InputUSDPerM
		}
		return a.ContextTokens > b.ContextTokens
	})
	return cands[0], nil
}
```

- [ ] **Step 5: Rodar e confirmar o verde**

Run: `go test ./internal/devin/ -v && go vet ./...`
Expected: PASS nos cinco testes.

- [ ] **Step 6: Commit**

```bash
git add internal/devin testdata/models-list.txt
git commit -m "feat: parse de models list e rota de modelo sem UID hardcoded"
```

---

### Task 4: Cliente Jev e primitivas tipadas

**Files:**
- Create: `internal/jev/primitives.go`
- Create: `internal/jev/client.go`
- Test: `internal/jev/client_test.go`

**Interfaces:**
- Consumes: nada.
- Produces:
  - `jev.Question` (interface selada, `isQuestion()`), com `jev.Noul{Instructions string; Criteria *NoulCriteria}`, `jev.Choice{Instructions string; Criteria map[string]string}`, `jev.Score{Instructions string; Criteria []string}`.
  - `jev.Answers` com `NoulOf(id string) (float64, bool)`, `ChoiceOf(id string) (ChoiceAnswer, bool)`, `ScoreOf(id string) (ScoreAnswer, bool)`.
  - `jev.ChoiceAnswer{Choice string; Confidence float64; Probabilities map[string]float64}`, `jev.ScoreAnswer{Score, Confidence float64; Probabilities []float64; Legend map[string]string}`.
  - `jev.Usage{InputTokens, OutputTokens int}`; `jev.Result{Answers Answers; Usage Usage}`.
  - `jev.New(opts Options) *Client` com `Options{APIKey, BaseURL string; HTTP *http.Client; MaxRetries int}`.
  - `(*Client).Ask(ctx context.Context, state any, qs map[string]Question) (Result, error)`.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/jev/client_test.go
package jev

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAskSendsCorrectPayload(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer k-123" {
			t.Errorf("Authorization = %q", auth)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, `{"model":"jev-1.13.0","answers":{"urgente":{"type":"noul","noul":0.91}},"usage":{"input_tokens":120,"output_tokens":0}}`)
	}))
	defer srv.Close()

	c := New(Options{APIKey: "k-123", BaseURL: srv.URL})
	res, err := c.Ask(context.Background(), "pagamentos falhando ha 3 dias", map[string]Question{
		"urgente": Noul{Instructions: "O texto transmite urgencia?"},
	})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}

	if got["model"] != "jev-latest" {
		t.Errorf("model = %v, quero jev-latest", got["model"])
	}
	qs := got["questions"].(map[string]any)["urgente"].(map[string]any)
	if qs["type"] != "noul" {
		t.Errorf("type = %v, quero noul", qs["type"])
	}

	p, ok := res.Answers.NoulOf("urgente")
	if !ok {
		t.Fatal("resposta urgente ausente")
	}
	if p != 0.91 {
		t.Errorf("noul = %v, quero 0.91", p)
	}
	if res.Usage.InputTokens != 120 {
		t.Errorf("InputTokens = %d, quero 120", res.Usage.InputTokens)
	}
}

func TestAskParsesChoiceAndScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"model":"jev-1.13.0","answers":{
			"rota":{"type":"choice","choice":"smart","confidence":0.8,"probabilities":{"smart":0.8,"auto":0.2}},
			"complexidade":{"type":"score","score":1.43,"confidence":0.6,"probabilities":[0.0,0.57,0.43],"legend":{"0":"trivial","1":"media","2":"alta"}}
		},"usage":{"input_tokens":10,"output_tokens":0}}`)
	}))
	defer srv.Close()

	c := New(Options{APIKey: "k", BaseURL: srv.URL})
	res, err := c.Ask(context.Background(), "x", map[string]Question{
		"rota":         Choice{Instructions: "Qual modo?", Criteria: map[string]string{"smart": "roda teste", "auto": "so leitura"}},
		"complexidade": Score{Instructions: "Quao complexa?", Criteria: []string{"trivial", "media", "alta"}},
	})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}

	ch, ok := res.Answers.ChoiceOf("rota")
	if !ok || ch.Choice != "smart" || ch.Confidence != 0.8 {
		t.Errorf("ChoiceOf = %+v, ok=%v", ch, ok)
	}
	sc, ok := res.Answers.ScoreOf("complexidade")
	if !ok || sc.Score != 1.43 {
		t.Errorf("ScoreOf = %+v, ok=%v", sc, ok)
	}
}

func TestAskRetriesOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{"model":"jev","answers":{"a":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1}}`)
	}))
	defer srv.Close()

	c := New(Options{APIKey: "k", BaseURL: srv.URL, MaxRetries: 5, backoffBase: time.Nanosecond})
	if _, err := c.Ask(context.Background(), "x", map[string]Question{"a": Noul{Instructions: "?"}}); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("chamadas = %d, quero 3", n)
	}
}

// 401 nao e transitorio: falhar rapido, e sem vazar a chave na mensagem.
func TestAskDoesNotRetryOn401AndHidesKey(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(Options{APIKey: "sk-supersecreta", BaseURL: srv.URL, MaxRetries: 5, backoffBase: time.Nanosecond})
	_, err := c.Ask(context.Background(), "x", map[string]Question{"a": Noul{Instructions: "?"}})
	if err == nil {
		t.Fatal("quero erro em 401")
	}
	if calls.Load() != 1 {
		t.Errorf("chamadas = %d, quero 1 (sem retry)", calls.Load())
	}
	if strings.Contains(err.Error(), "sk-supersecreta") {
		t.Error("a mensagem de erro vazou a chave")
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/jev/ -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Implementar as primitivas**

```go
// internal/jev/primitives.go
package jev

// Question e uma pergunta do System One. Interface selada: so os tres
// primitivos do TypeSafe a implementam.
type Question interface{ isQuestion() }

// NoulCriteria descreve o que conta como sim e como nao.
type NoulCriteria struct {
	True  string `json:"true"`
	False string `json:"false"`
}

// Noul pergunta sim ou nao e devolve probabilidade de sim.
type Noul struct {
	Instructions string        `json:"instructions"`
	Criteria     *NoulCriteria `json:"criteria,omitempty"`
}

// Choice escolhe uma entre opcoes nomeadas.
type Choice struct {
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// Score posiciona numa escala ordenada de 2 a 10 niveis, do menor ao maior.
type Score struct {
	Instructions string   `json:"instructions"`
	Criteria     []string `json:"criteria"`
}

func (Noul) isQuestion()   {}
func (Choice) isQuestion() {}
func (Score) isQuestion()  {}

// ChoiceAnswer e a resposta de uma Choice.
type ChoiceAnswer struct {
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// ScoreAnswer e a resposta de um Score.
type ScoreAnswer struct {
	Score         float64           `json:"score"`
	Confidence    float64           `json:"confidence"`
	Probabilities []float64         `json:"probabilities"`
	Legend        map[string]string `json:"legend"`
}

type rawAnswer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul"`
	Choice        string             `json:"choice"`
	Score         float64            `json:"score"`
	Confidence    float64            `json:"confidence"`
	Probabilities any                `json:"probabilities"`
	Legend        map[string]string  `json:"legend"`
}

// Answers sao as respostas indexadas pelos IDs das perguntas.
type Answers map[string]rawAnswer

// NoulOf devolve a probabilidade de sim do noul com este id.
func (a Answers) NoulOf(id string) (float64, bool) {
	r, ok := a[id]
	if !ok || r.Type != "noul" {
		return 0, false
	}
	return r.Noul, true
}

// ChoiceOf devolve a resposta da choice com este id.
func (a Answers) ChoiceOf(id string) (ChoiceAnswer, bool) {
	r, ok := a[id]
	if !ok || r.Type != "choice" {
		return ChoiceAnswer{}, false
	}
	probs := map[string]float64{}
	if m, ok := r.Probabilities.(map[string]any); ok {
		for k, v := range m {
			if f, ok := v.(float64); ok {
				probs[k] = f
			}
		}
	}
	return ChoiceAnswer{Choice: r.Choice, Confidence: r.Confidence, Probabilities: probs}, true
}

// ScoreOf devolve a resposta do score com este id.
func (a Answers) ScoreOf(id string) (ScoreAnswer, bool) {
	r, ok := a[id]
	if !ok || r.Type != "score" {
		return ScoreAnswer{}, false
	}
	var probs []float64
	if s, ok := r.Probabilities.([]any); ok {
		for _, v := range s {
			if f, ok := v.(float64); ok {
				probs = append(probs, f)
			}
		}
	}
	return ScoreAnswer{Score: r.Score, Confidence: r.Confidence, Probabilities: probs, Legend: r.Legend}, true
}
```

- [ ] **Step 4: Implementar o cliente**

```go
// internal/jev/client.go
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL e o endpoint do System One.
const DefaultBaseURL = "https://api.typesafe.ai/v1/systemone"

// ModelAlias e o alias estavel do Jev. Nao ha escolha de modelo aqui:
// o TypeSafe usa os mesmos pesos para todas as contas.
const ModelAlias = "jev-latest"

// Usage e o consumo de tokens de uma requisicao. Saida e gratuita.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Result e a resposta completa de uma requisicao.
type Result struct {
	Answers Answers
	Usage   Usage
}

// Options configura o cliente.
type Options struct {
	APIKey     string
	BaseURL    string
	HTTP       *http.Client
	MaxRetries int

	// backoffBase e zerado nos testes para nao dormir.
	backoffBase time.Duration
}

// Client fala com a API do System One.
type Client struct {
	apiKey      string
	baseURL     string
	http        *http.Client
	maxRetries  int
	backoffBase time.Duration
}

// New cria o cliente. A chave nunca e gravada nem impressa.
func New(o Options) *Client {
	c := &Client{
		apiKey:      o.APIKey,
		baseURL:     o.BaseURL,
		http:        o.HTTP,
		maxRetries:  o.MaxRetries,
		backoffBase: o.backoffBase,
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 60 * time.Second}
	}
	if c.maxRetries == 0 {
		c.maxRetries = 3
	}
	if c.backoffBase == 0 {
		c.backoffBase = 500 * time.Millisecond
	}
	return c
}

type request struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

type response struct {
	Model   string  `json:"model"`
	Answers Answers `json:"answers"`
	Usage   Usage   `json:"usage"`
}

// Ask faz uma requisicao com todas as perguntas juntas. Perguntas de uma
// mesma requisicao sao avaliadas em paralelo e nao veem as respostas umas
// das outras — e por isso que fan-out especulativo funciona.
func (c *Client) Ask(ctx context.Context, state any, qs map[string]Question) (Result, error) {
	if len(qs) == 0 {
		return Result{}, fmt.Errorf("jev: nenhuma pergunta")
	}
	body, err := json.Marshal(request{State: state, Model: ModelAlias, Questions: qs})
	if err != nil {
		return Result{}, fmt.Errorf("jev: serializando requisicao: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			d := c.backoffBase * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			case <-time.After(d):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
		if err != nil {
			return Result{}, fmt.Errorf("jev: montando requisicao: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("jev: conexao: %w", err)
			continue
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusOK:
			if readErr != nil {
				return Result{}, fmt.Errorf("jev: lendo resposta: %w", readErr)
			}
			var out response
			if err := json.Unmarshal(raw, &out); err != nil {
				return Result{}, fmt.Errorf("jev: resposta nao e JSON valido: %w", err)
			}
			return Result{Answers: out.Answers, Usage: out.Usage}, nil

		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			// transitorio: tenta de novo
			lastErr = fmt.Errorf("jev: status %d", resp.StatusCode)

		default:
			// 401, 422 e afins nao melhoram com retry. A chave nunca entra
			// na mensagem; so o status e o corpo da API.
			return Result{}, fmt.Errorf("jev: status %d: %s", resp.StatusCode, truncate(string(raw), 300))
		}
	}
	return Result{}, fmt.Errorf("jev: esgotadas %d tentativas: %w", c.maxRetries, lastErr)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
```

- [ ] **Step 5: Rodar e confirmar o verde**

Run: `go test ./internal/jev/ -v && go vet ./...`
Expected: PASS nos quatro testes.

- [ ] **Step 6: Commit**

```bash
git add internal/jev
git commit -m "feat: cliente Jev com primitivas tipadas, retry e chave protegida"
```

---

### Task 5: Fatiamento contra os tetos do Jev

Um export ATIF de run longo não cabe numa requisição. Esta tarefa isola a única regra de tamanho do projeto, para que nenhuma outra precise pensar nisso.

**Files:**
- Create: `internal/jev/window.go`
- Test: `internal/jev/window_test.go`

**Interfaces:**
- Consumes: `jev.Question` (Task 4).
- Produces:
  - `jev.EstimateTokens(s string) int` — estimativa por caracteres, declaradamente aproximada.
  - `jev.Limits{Total, StatePlusLongest int}` e `jev.DefaultLimits()` devolvendo `{64_000, 32_000}`.
  - `jev.Chunk[T any]{Items []T}` e `jev.SplitByBudget[T any](items []T, size func(T) int, budget int) ([]Chunk[T], error)` — erro quando um item sozinho estoura o orçamento.
  - `jev.StateBudget(qs map[string]Question, l Limits) (int, error)` — quanto sobra para o state dadas as perguntas.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/jev/window_test.go
package jev

import (
	"strings"
	"testing"
)

func TestEstimateTokensIsRoughlyCharsOverFour(t *testing.T) {
	got := EstimateTokens(strings.Repeat("a", 400))
	if got < 80 || got > 120 {
		t.Errorf("EstimateTokens(400 chars) = %d, quero entre 80 e 120", got)
	}
}

func TestStateBudgetSubtractsLongestQuestion(t *testing.T) {
	qs := map[string]Question{
		"curta":  Noul{Instructions: "ok?"},
		"longa":  Noul{Instructions: strings.Repeat("x", 4000)}, // ~1000 tokens
	}
	got, err := StateBudget(qs, DefaultLimits())
	if err != nil {
		t.Fatalf("StateBudget: %v", err)
	}
	// 32000 menos a maior pergunta (~1000), com folga para o envelope JSON.
	if got > 31_100 || got < 29_000 {
		t.Errorf("StateBudget = %d, quero perto de 31000", got)
	}
}

func TestStateBudgetErrorsWhenQuestionsAloneExceed(t *testing.T) {
	qs := map[string]Question{"gigante": Noul{Instructions: strings.Repeat("x", 200_000)}}
	if _, err := StateBudget(qs, DefaultLimits()); err == nil {
		t.Fatal("quero erro quando a pergunta sozinha estoura o teto")
	}
}

func TestSplitByBudgetGroupsItems(t *testing.T) {
	items := []string{"aaaa", "bbbb", "cccc", "dddd"} // 1 token cada
	chunks, err := SplitByBudget(items, func(s string) int { return EstimateTokens(s) }, 2)
	if err != nil {
		t.Fatalf("SplitByBudget: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, quero 2", len(chunks))
	}
	if len(chunks[0].Items) != 2 || len(chunks[1].Items) != 2 {
		t.Errorf("distribuicao errada: %+v", chunks)
	}
}

func TestSplitByBudgetErrorsOnOversizedItem(t *testing.T) {
	items := []string{strings.Repeat("x", 4000)}
	if _, err := SplitByBudget(items, func(s string) int { return EstimateTokens(s) }, 10); err == nil {
		t.Fatal("quero erro quando um item sozinho nao cabe")
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/jev/ -run TestEstimate -v`
Expected: FAIL — `undefined: EstimateTokens`.

- [ ] **Step 3: Implementar**

```go
// internal/jev/window.go
package jev

import (
	"encoding/json"
	"fmt"
)

// Limits sao os tetos do Jev por requisicao, em tokens.
type Limits struct {
	Total            int // state + todas as perguntas
	StatePlusLongest int // state + a maior pergunta
}

// DefaultLimits traz os valores publicados para jev-latest.
func DefaultLimits() Limits { return Limits{Total: 64_000, StatePlusLongest: 32_000} }

// envelopeSlack reserva espaco para o JSON em volta do conteudo.
const envelopeSlack = 512

// EstimateTokens estima tokens contando caracteres. E aproximacao, nao
// tokenizacao: erra para mais em texto denso e para menos em ASCII simples.
// Por isso todo orcamento aqui deixa folga.
func EstimateTokens(s string) int { return (len(s) + 3) / 4 }

func questionTokens(q Question) int {
	raw, err := json.Marshal(q)
	if err != nil {
		return 0
	}
	return EstimateTokens(string(raw))
}

// StateBudget devolve quantos tokens sobram para o state, respeitando os dois
// tetos ao mesmo tempo: o total e o de state mais a maior pergunta.
func StateBudget(qs map[string]Question, l Limits) (int, error) {
	var sum, longest int
	for _, q := range qs {
		n := questionTokens(q)
		sum += n
		if n > longest {
			longest = n
		}
	}
	byTotal := l.Total - sum - envelopeSlack
	byLongest := l.StatePlusLongest - longest - envelopeSlack

	budget := byTotal
	if byLongest < budget {
		budget = byLongest
	}
	if budget <= 0 {
		return 0, fmt.Errorf("jev: perguntas ocupam %d tokens e nao deixam espaco para o state (tetos %d/%d)",
			sum, l.Total, l.StatePlusLongest)
	}
	return budget, nil
}

// Chunk e um grupo de itens que cabe num orcamento.
type Chunk[T any] struct{ Items []T }

// SplitByBudget agrupa itens em chunks que cabem no orcamento, preservando a
// ordem. Item que sozinho nao cabe e erro: truncar aqui esconderia perda.
func SplitByBudget[T any](items []T, size func(T) int, budget int) ([]Chunk[T], error) {
	var chunks []Chunk[T]
	var cur Chunk[T]
	var used int

	for i, it := range items {
		n := size(it)
		if n > budget {
			return nil, fmt.Errorf("jev: item %d ocupa %d tokens e o orcamento e %d", i, n, budget)
		}
		if used+n > budget && len(cur.Items) > 0 {
			chunks = append(chunks, cur)
			cur, used = Chunk[T]{}, 0
		}
		cur.Items = append(cur.Items, it)
		used += n
	}
	if len(cur.Items) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks, nil
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/jev/ -v && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jev/window.go internal/jev/window_test.go
git commit -m "feat: fatiamento contra os tetos de 64k/32k do Jev"
```

---

### Task 6: Parser do export ATIF

> **Consumidor:** o `result` e a compactação (Task 14), **não** o watchdog. Mediu-se em 2026-09-20 que `--export` só é escrito no encerramento do run — com um run em andamento e arquivos já no disco, `export.json` não existia. Para o watchdog, a fonte é o stdout (Task 12).

**Files:**
- Create: `internal/devin/atif.go`
- Test: `internal/devin/atif_test.go`

**Interfaces:**
- Consumes: nada.
- Produces:
  - `devin.ToolInteraction{ID, Name, Input, Output, Status string}`
  - `devin.Turn{Index int; Text string; Tools []ToolInteraction}`
  - `devin.ParseATIF(raw []byte) ([]Turn, error)` — tolerante a campos desconhecidos; erro só quando o JSON é inválido.
  - `(Turn) Summary() string` — rendição compacta de um turno, usada como state do watchdog.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/devin/atif_test.go
package devin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heliowap/devin-plugin-cc/internal/testsupport"
)

// O parser e exercitado contra a saida do fakedevin, nao contra um JSON
// escrito a mao: assim, se o formato do produtor mudar, o teste acusa.
func TestParseATIFReadsFakeDevinExport(t *testing.T) {
	testsupport.InstallFakeDevin(t, "healthy")

	dir := t.TempDir()
	export := filepath.Join(dir, "export.json")
	prompt := filepath.Join(dir, "b.md")
	if err := os.WriteFile(prompt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("devin", "--prompt-file", prompt, "--export", export, "-p").CombinedOutput(); err != nil {
		t.Fatalf("fakedevin: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(export)
	if err != nil {
		t.Fatal(err)
	}
	turns, err := ParseATIF(raw)
	if err != nil {
		t.Fatalf("ParseATIF: %v", err)
	}
	if len(turns) != 5 {
		t.Fatalf("len(turns) = %d, quero 5", len(turns))
	}
	if turns[2].Tools[0].Output != "FAIL: TestRota" {
		t.Errorf("turno 2 nao trouxe o vermelho: %+v", turns[2].Tools[0])
	}
	if !strings.Contains(turns[2].Summary(), "go test ./pkg/svc/") {
		t.Errorf("Summary nao traz o comando: %q", turns[2].Summary())
	}
}

func TestParseATIFToleratesUnknownFields(t *testing.T) {
	raw := []byte(`{"schema":"atif/2","turns":[
		{"index":0,"text":"oi","novidade":{"a":1},"tools":[
			{"id":"t0","name":"read","input":"f.go","output":"...","status":"ok","extra":true}
		]}
	]}`)
	turns, err := ParseATIF(raw)
	if err != nil {
		t.Fatalf("ParseATIF: %v", err)
	}
	if len(turns) != 1 || turns[0].Tools[0].ID != "t0" {
		t.Errorf("campos desconhecidos quebraram o parse: %+v", turns)
	}
}

func TestParseATIFErrorsOnInvalidJSON(t *testing.T) {
	if _, err := ParseATIF([]byte("{{{")); err == nil {
		t.Fatal("quero erro em JSON invalido")
	}
}

// O export e escrito atomicamente pelo devin, mas um leitor pode pegar um
// arquivo vazio entre o create e o rename. Isso nao e erro: e "ainda nao".
func TestParseATIFEmptyReturnsNoTurns(t *testing.T) {
	turns, err := ParseATIF(nil)
	if err != nil {
		t.Fatalf("ParseATIF(nil): %v", err)
	}
	if len(turns) != 0 {
		t.Errorf("quero zero turnos, tenho %d", len(turns))
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/devin/ -run TestParseATIF -v`
Expected: FAIL — `undefined: ParseATIF`.

- [ ] **Step 3: Implementar**

```go
// internal/devin/atif.go
package devin

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolInteraction e um par chamada/resultado dentro de um turno.
type ToolInteraction struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Status string `json:"status"`
}

// Turn e um turno do agente.
type Turn struct {
	Index int               `json:"index"`
	Text  string            `json:"text"`
	Tools []ToolInteraction `json:"tools"`
}

// Summary rende o turno de forma compacta, para virar state do Jev.
// Nao interpreta o conteudo: o texto do Devin e dado, nunca instrucao.
func (t Turn) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "turno %d: %s\n", t.Index, t.Text)
	for _, tool := range t.Tools {
		fmt.Fprintf(&b, "  %s(%s) -> [%s] %s\n", tool.Name, tool.Input, tool.Status, tool.Output)
	}
	return b.String()
}

// ParseATIF le o export do devin. Campos desconhecidos sao ignorados, para
// sobreviver a mudancas de formato; entrada vazia devolve zero turnos sem
// erro, porque o arquivo pode ainda nao ter sido escrito.
func ParseATIF(raw []byte) ([]Turn, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var doc struct {
		Turns []Turn `json:"turns"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("devin: export ATIF invalido: %w", err)
	}
	return doc.Turns, nil
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/devin/ -v && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/devin/atif.go internal/devin/atif_test.go
git commit -m "feat: parser tolerante do export ATIF"
```

---

### Task 7: Job store

**Files:**
- Create: `internal/job/paths.go`
- Create: `internal/job/store.go`
- Test: `internal/job/store_test.go`

**Interfaces:**
- Consumes: `devin.Model` (Task 3).
- Produces:
  - `job.Root() (string, error)` — `$XDG_STATE_HOME/devin-plugin-cc/jobs` ou `~/.local/state/...`.
  - `job.State` string com `StatePlanned`, `StateRunning`, `StateCompleted`, `StateFailed`, `StateCancelled`.
  - `job.Job{ID string; State State; Worktree, Branch string; Model, Permission string; PID, PGID int; CreatedAt, UpdatedAt time.Time; CancelReason *CancelReason}`
  - `job.CancelReason{Signal string; Probability float64; TurnExcerpt string; ResumeCommand string; At time.Time}`
  - `job.Create(worktree string) (*Job, error)` — falha quando já existe job vivo para a mesma worktree (lockfile).
  - `(*Job) Dir() string`, `(*Job) Path(name string) string`, `(*Job) Save() error`, `job.Load(id string) (*Job, error)`, `job.Release(id string) error`.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/job/store_test.go
package job

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootHonorsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-teste")
	got, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/xdg-teste/devin-plugin-cc/jobs" {
		t.Errorf("Root() = %q", got)
	}
}

func TestCreateAndLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	j, err := Create("/tmp/wt-a")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if j.State != StatePlanned {
		t.Errorf("State = %q, quero %q", j.State, StatePlanned)
	}
	j.Model = "swe-2-max"
	j.Permission = "smart"
	j.State = StateRunning
	if err := j.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(j.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Model != "swe-2-max" || got.Permission != "smart" || got.State != StateRunning {
		t.Errorf("round-trip perdeu campos: %+v", got)
	}
}

// Dois jobs na mesma worktree e exatamente o que o protocolo proibe:
// "nunca aponte dois agentes para a mesma pasta".
func TestCreateRefusesSecondJobOnSameWorktree(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if _, err := Create("/tmp/wt-b"); err != nil {
		t.Fatalf("primeiro Create: %v", err)
	}
	if _, err := Create("/tmp/wt-b"); err == nil {
		t.Fatal("quero erro no segundo Create da mesma worktree")
	}
}

func TestReleaseAllowsReuse(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	j, err := Create("/tmp/wt-c")
	if err != nil {
		t.Fatal(err)
	}
	if err := Release(j.ID); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := Create("/tmp/wt-c"); err != nil {
		t.Fatalf("Create apos Release: %v", err)
	}
}

func TestPathIsInsideJobDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	j, err := Create("/tmp/wt-d")
	if err != nil {
		t.Fatal(err)
	}
	p := j.Path("briefing.md")
	if filepath.Dir(p) != j.Dir() {
		t.Errorf("Path saiu do diretorio do job: %q", p)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Errorf("o diretorio do job nao existe: %v", err)
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/job/ -v`
Expected: FAIL — `undefined: Root`.

- [ ] **Step 3: Implementar**

```go
// internal/job/paths.go
package job

import (
	"os"
	"path/filepath"
)

// Root e a raiz do job store, respeitando XDG_STATE_HOME.
func Root() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "devin-plugin-cc", "jobs"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "devin-plugin-cc", "jobs"), nil
}
```

```go
// internal/job/store.go
package job

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State e o estado do job.
type State string

const (
	StatePlanned   State = "planned"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Terminal indica estado do qual nao se sai.
func (s State) Terminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateCancelled
}

// CancelReason e o motivo legivel de um cancelamento pelo watchdog.
type CancelReason struct {
	Signal        string    `json:"signal"`
	Probability   float64   `json:"probability"`
	TurnExcerpt   string    `json:"turn_excerpt"`
	ResumeCommand string    `json:"resume_command"`
	At            time.Time `json:"at"`
}

// Job e o estado persistido de uma delegacao.
type Job struct {
	ID           string        `json:"id"`
	State        State         `json:"state"`
	Worktree     string        `json:"worktree"`
	Branch       string        `json:"branch"`
	Model        string        `json:"model"`
	Permission   string        `json:"permission"`
	PID          int           `json:"pid"`
	PGID         int           `json:"pgid"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	CancelReason *CancelReason `json:"cancel_reason,omitempty"`

	dir string
}

// Dir e o diretorio do job.
func (j *Job) Dir() string { return j.dir }

// Path devolve um caminho dentro do diretorio do job.
func (j *Job) Path(name string) string { return filepath.Join(j.dir, name) }

// Save grava job.json atomicamente.
func (j *Job) Save() error {
	j.UpdatedAt = time.Now()
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := j.Path("job.json.tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, j.Path("job.json"))
}

func newID() string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return "job-" + hex.EncodeToString(b)
}

func lockPath(root, worktree string) string {
	sum := hex.EncodeToString([]byte(filepath.Clean(worktree)))
	if len(sum) > 40 {
		sum = sum[:40]
	}
	return filepath.Join(root, ".locks", sum+".lock")
}

// Create cria um job novo e trava a worktree. Duas delegacoes na mesma pasta
// e o erro que o protocolo proibe explicitamente.
func Create(worktree string) (*Job, error) {
	root, err := Root()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, ".locks"), 0o755); err != nil {
		return nil, err
	}

	j := &Job{
		ID:        newID(),
		State:     StatePlanned,
		Worktree:  worktree,
		CreatedAt: time.Now(),
	}
	j.dir = filepath.Join(root, j.ID)

	lock := lockPath(root, worktree)
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			owner, _ := os.ReadFile(lock)
			return nil, fmt.Errorf("worktree %s ja esta em uso pelo job %s", worktree, string(owner))
		}
		return nil, err
	}
	_, _ = f.WriteString(j.ID)
	f.Close()

	if err := os.MkdirAll(j.dir, 0o755); err != nil {
		_ = os.Remove(lock)
		return nil, err
	}
	if err := j.Save(); err != nil {
		_ = os.Remove(lock)
		return nil, err
	}
	return j, nil
}

// Load le um job pelo id.
func Load(id string) (*Job, error) {
	root, err := Root()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, id)
	raw, err := os.ReadFile(filepath.Join(dir, "job.json"))
	if err != nil {
		return nil, fmt.Errorf("job %s: %w", id, err)
	}
	var j Job
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, fmt.Errorf("job %s: job.json invalido: %w", id, err)
	}
	j.dir = dir
	return &j, nil
}

// Release solta a trava da worktree do job.
func Release(id string) error {
	j, err := Load(id)
	if err != nil {
		return err
	}
	root, err := Root()
	if err != nil {
		return err
	}
	if err := os.Remove(lockPath(root, j.Worktree)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/job/ -v && go vet ./...`
Expected: PASS nos cinco testes.

- [ ] **Step 5: Commit**

```bash
git add internal/job
git commit -m "feat: job store com lockfile por worktree"
```

---

### Task 8: Perguntas do spec §6 e contabilidade de orçamento

**Files:**
- Create: `internal/jev/questions.go`
- Create: `internal/jev/budget.go`
- Test: `internal/jev/questions_test.go`, `internal/jev/budget_test.go`

**Interfaces:**
- Consumes: `jev.Question`, `jev.Usage` (Task 4).
- Produces:
  - `jev.DelegabilityQuestions() map[string]Question` — ids `tipo_de_tarefa`, `defeito_unico`, `desenho_em_aberto`, `cruza_pacotes`, `toca_sensivel`, `criterio_de_pronto`.
  - `jev.BriefingQuestions() map[string]Question` — ids `aponta_arquivo_linha`, `cita_fonte_do_contrato`, `pede_teste_antes_da_correcao`, `comandos_copiaveis`, `limites_explicitos`, `pede_relatorio`.
  - `jev.RouteQuestions() map[string]Question` — ids `permissao`, `complexidade`.
  - `jev.WatchdogQuestions() map[string]Question` — ids `sem_progresso`, `bloqueio_de_permissao`.
  - `jev.EvidenceQuestion() map[string]Question` — id `evidencia_necessaria`.
  - `jev.CompactionQuestions() map[string]Question` — ids `chamada_necessaria`, `resultado_necessario_verbatim`.
  - `jev.ReportQuestion() map[string]Question` — id `relatorio_afirma_verde`.
  - `jev.FindingQuestions() map[string]Question` — ids `tem_cenario_reproduzivel`, `severidade`.
  - `jev.InputUSDPerMillion = 0.042`; `jev.Ledger{Path string}` com `(*Ledger) Record(kind string, u Usage) error` e `(*Ledger) Total() (tokens int, usd float64, err error)`.

- [ ] **Step 1: Escrever o teste das perguntas**

```go
// internal/jev/questions_test.go
package jev

import (
	"encoding/json"
	"strings"
	"testing"
)

// Toda pergunta precisa carregar significado completa: o id nao e enviado ao
// modelo, entao "defeito_unico" sozinho nao diz nada a ele.
func TestAllQuestionsHaveSubstantiveInstructions(t *testing.T) {
	sets := map[string]map[string]Question{
		"delegability": DelegabilityQuestions(),
		"briefing":     BriefingQuestions(),
		"route":        RouteQuestions(),
		"watchdog":     WatchdogQuestions(),
		"evidence":     EvidenceQuestion(),
		"compaction":   CompactionQuestions(),
		"report":       ReportQuestion(),
		"finding":      FindingQuestions(),
	}
	for set, qs := range sets {
		if len(qs) == 0 {
			t.Errorf("conjunto %s esta vazio", set)
		}
		for id, q := range qs {
			raw, err := json.Marshal(q)
			if err != nil {
				t.Fatalf("%s/%s: %v", set, id, err)
			}
			var probe struct {
				Instructions string `json:"instructions"`
			}
			if err := json.Unmarshal(raw, &probe); err != nil {
				t.Fatalf("%s/%s: %v", set, id, err)
			}
			if len(probe.Instructions) < 40 {
				t.Errorf("%s/%s: instructions curtas demais (%d chars): %q",
					set, id, len(probe.Instructions), probe.Instructions)
			}
			if strings.Contains(probe.Instructions, "_") && !strings.Contains(probe.Instructions, " ") {
				t.Errorf("%s/%s: instructions parecem um id, nao uma pergunta", set, id)
			}
		}
	}
}

func TestDelegabilityHasExpectedIDs(t *testing.T) {
	qs := DelegabilityQuestions()
	for _, id := range []string{
		"tipo_de_tarefa", "defeito_unico", "desenho_em_aberto",
		"cruza_pacotes", "toca_sensivel", "criterio_de_pronto",
	} {
		if _, ok := qs[id]; !ok {
			t.Errorf("pergunta %q ausente", id)
		}
	}
	ch, ok := qs["tipo_de_tarefa"].(Choice)
	if !ok {
		t.Fatal("tipo_de_tarefa deveria ser Choice")
	}
	for _, opt := range []string{"correcao_com_teste", "revisao_somente_leitura", "investigacao", "nao_delegavel"} {
		if _, ok := ch.Criteria[opt]; !ok {
			t.Errorf("opcao %q ausente em tipo_de_tarefa", opt)
		}
	}
}

// dangerous nunca pode ser escolhido pela rota: e regra global do projeto.
func TestRouteNeverOffersDangerous(t *testing.T) {
	ch, ok := RouteQuestions()["permissao"].(Choice)
	if !ok {
		t.Fatal("permissao deveria ser Choice")
	}
	if _, found := ch.Criteria["dangerous"]; found {
		t.Error("dangerous nao pode ser opcao da rota")
	}
	for _, want := range []string{"auto", "accept-edits", "smart"} {
		if _, ok := ch.Criteria[want]; !ok {
			t.Errorf("modo %q ausente", want)
		}
	}
}

// Score precisa de niveis descritos por situacao concreta, do menor ao maior.
func TestComplexityScoreHasConcreteLevels(t *testing.T) {
	sc, ok := RouteQuestions()["complexidade"].(Score)
	if !ok {
		t.Fatal("complexidade deveria ser Score")
	}
	if len(sc.Criteria) < 2 || len(sc.Criteria) > 10 {
		t.Fatalf("len(Criteria) = %d, o Jev aceita de 2 a 10", len(sc.Criteria))
	}
	for i, level := range sc.Criteria {
		if len(level) < 30 {
			t.Errorf("nivel %d descrito de forma vaga: %q", i, level)
		}
	}
}

// Todo conjunto precisa caber no orcamento junto com um state util.
func TestEverySetLeavesRoomForState(t *testing.T) {
	sets := map[string]map[string]Question{
		"delegability": DelegabilityQuestions(),
		"briefing":     BriefingQuestions(),
		"route":        RouteQuestions(),
		"watchdog":     WatchdogQuestions(),
		"compaction":   CompactionQuestions(),
		"finding":      FindingQuestions(),
	}
	for name, qs := range sets {
		budget, err := StateBudget(qs, DefaultLimits())
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if budget < 20_000 {
			t.Errorf("%s: sobram so %d tokens para o state", name, budget)
		}
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/jev/ -run TestAllQuestions -v`
Expected: FAIL — `undefined: DelegabilityQuestions`.

- [ ] **Step 3: Implementar as perguntas**

```go
// internal/jev/questions.go
package jev

// As perguntas do spec §6. IDs em portugues, como especificado; o ID nao e
// enviado ao modelo, entao cada pergunta carrega o significado inteiro no
// texto. Cada conjunto tem fixture rotulada correspondente em evals/.

// QuestionsVersion muda sempre que o texto de uma pergunta muda, para que a
// auditoria em jev.jsonl continue interpretavel depois de uma recalibragem.
const QuestionsVersion = "1"

// DelegabilityQuestions julga se a tarefa deve ser delegada (spec §6.1).
func DelegabilityQuestions() map[string]Question {
	return map[string]Question{
		"tipo_de_tarefa": Choice{
			Instructions: "Classifique o trabalho descrito em `tarefa.texto` pelo tipo de entrega que ele pede a um agente de codigo que trabalha sozinho num repositorio.",
			Criteria: map[string]string{
				"correcao_com_teste":      "Corrigir um comportamento errado num codigo que ja existe, com um teste que prove a correcao.",
				"revisao_somente_leitura": "Ler codigo e apontar problemas, sem alterar arquivo nenhum.",
				"investigacao":            "Descobrir a causa de um sintoma, ou levantar como algo funciona, sem que se saiba de antemao qual sera a correcao.",
				"nao_delegavel":           "Exige decidir o desenho, negociar requisito, tocar credencial, segredo, dado pessoal ou deploy, ou depende de contexto que nao esta escrito.",
			},
		},
		"defeito_unico": Noul{
			Instructions: "A tarefa em `tarefa.texto` trata de um unico defeito ou comportamento a corrigir.",
			Criteria: &NoulCriteria{
				True:  "Um defeito, ou dois defeitos que sao a mesma causa vista de dois lugares.",
				False: "Dois ou mais temas independentes, que poderiam virar correcoes separadas sem prejuizo.",
			},
		},
		"desenho_em_aberto": Noul{
			Instructions: "Executar a tarefa em `tarefa.texto` exige escolher entre desenhos possiveis, sem que a tarefa diga qual escolher.",
			Criteria: &NoulCriteria{
				True:  "Quem for executar precisa decidir arquitetura, contrato de API, formato de dado ou politica que a tarefa deixou em aberto.",
				False: "O que fazer esta determinado; restam apenas decisoes locais de implementacao.",
			},
		},
		"cruza_pacotes": Noul{
			Instructions: "A correcao descrita em `tarefa.texto` exige mudancas coordenadas em mais de um pacote ou modulo, considerando `repo.pacotes_atingidos`.",
			Criteria: &NoulCriteria{
				True:  "Alterar um pacote sozinho deixaria o sistema inconsistente ou quebrado.",
				False: "A mudanca cabe num pacote, mesmo que outros o consumam sem alteracao.",
			},
		},
		"toca_sensivel": Noul{
			Instructions: "A tarefa em `tarefa.texto` envolve credencial, segredo, dado pessoal, configuracao de deploy ou infraestrutura de producao.",
			Criteria: &NoulCriteria{
				True:  "Executa-la implica ler, escrever ou mover esse tipo de material, ou alterar o que vai para producao.",
				False: "Fica restrita a codigo de aplicacao, teste ou documentacao.",
			},
		},
		"criterio_de_pronto": Noul{
			Instructions: "A tarefa em `tarefa.texto` deixa claro como saber que terminou, citando teste, comando ou saida esperada.",
			Criteria: &NoulCriteria{
				True:  "Ha um criterio objetivo e verificavel: um teste que precisa passar, um comando com saida esperada, um comportamento observavel.",
				False: "O criterio e subjetivo, implicito, ou simplesmente ausente.",
			},
		},
	}
}

// BriefingQuestions confere o briefing montado (spec §6.2). Cada pergunta
// corresponde a um item do protocolo medido em uso real.
func BriefingQuestions() map[string]Question {
	return map[string]Question{
		"aponta_arquivo_linha": Noul{
			Instructions: "O briefing localiza o defeito citando arquivo e linha, e nao apenas descrevendo o sintoma.",
			Criteria: &NoulCriteria{
				True:  "Ha ao menos uma referencia no formato caminho/arquivo:linha apontando onde esta o problema.",
				False: "So ha descricao do sintoma, nome de funcao solto, ou nenhuma localizacao.",
			},
		},
		"cita_fonte_do_contrato": Noul{
			Instructions: "O briefing diz qual contrato foi violado e nomeia a fonte desse contrato.",
			Criteria: &NoulCriteria{
				True:  "Cita um ADR, especificacao, documento, issue ou comentario normativo como a fonte da regra que o codigo desrespeita.",
				False: "Afirma que algo esta errado sem dizer com base em que regra, ou apenas apela ao bom senso.",
			},
		},
		"pede_teste_antes_da_correcao": Noul{
			Instructions: "O briefing manda escrever o teste antes de corrigir e manda confirmar que ele falha antes da correcao.",
			Criteria: &NoulCriteria{
				True:  "As duas coisas estao pedidas: escrever o teste primeiro, e confirmar o vermelho antes de mexer no codigo de producao.",
				False: "Pede teste mas nao pede o vermelho, pede so a correcao, ou deixa a ordem em aberto.",
			},
		},
		"comandos_copiaveis": Noul{
			Instructions: "O briefing traz os comandos exatos de teste e de lint, prontos para copiar e colar, incluindo o interpretador ou caminho de ambiente quando necessario.",
			Criteria: &NoulCriteria{
				True:  "Os comandos estao escritos por extenso e podem ser colados num shell sem adivinhacao.",
				False: "Diz apenas 'rode os testes', ou descreve o comando sem escreve-lo, ou omite como acessar o ambiente.",
			},
		},
		"limites_explicitos": Noul{
			Instructions: "O briefing lista o que nao pode ser feito: commit, push, acesso a rede, pastas proibidas, arquivos que nao podem mudar.",
			Criteria: &NoulCriteria{
				True:  "Ha uma lista explicita de proibicoes ou de escopo permitido.",
				False: "Nao ha restricao escrita, ou ela fica subentendida.",
			},
		},
		"pede_relatorio": Noul{
			Instructions: "O briefing pede um relatorio final com o teste escrito, a mudanca por arquivo e a saida dos comandos executados.",
			Criteria: &NoulCriteria{
				True:  "Pede explicitamente esses tres elementos, ou equivalentes que permitam conferir o trabalho sem reler o codigo.",
				False: "Nao pede relatorio, ou pede so um resumo em prosa.",
			},
		},
	}
}

// RouteQuestions escolhem modo de permissao e esforco (spec §6.4).
// dangerous nao aparece: nunca e escolhido automaticamente.
func RouteQuestions() map[string]Question {
	return map[string]Question{
		"permissao": Choice{
			Instructions: "Escolha o nivel minimo de autonomia que permite concluir a tarefa descrita em `tarefa.texto` num agente de codigo rodando sem ninguem para responder perguntas.",
			Criteria: map[string]string{
				"auto":         "Basta ler arquivos e responder. Nenhuma escrita em disco e nenhum comando que altere estado.",
				"accept-edits": "Precisa editar arquivos do repositorio, mas nao precisa executar comando nenhum para concluir.",
				"smart":        "Precisa executar comandos, tipicamente rodar teste ou lint, alem de editar arquivos.",
			},
		},
		"complexidade": Score{
			Instructions: "Avalie quanto raciocinio a tarefa em `tarefa.texto` exige de quem for executa-la, considerando o que precisa ser entendido antes de escrever a primeira linha.",
			Criteria: []string{
				"A mudanca e local e mecanica: corrigir um comparador, um nome errado, um off-by-one, um valor de constante. Quem le a tarefa ja sabe o que digitar.",
				"A mudanca exige ler uma funcao e seus chamadores para entender o contrato antes de alterar, mas o defeito e visivel quando se olha o lugar certo.",
				"A mudanca exige reconstruir o comportamento a partir de varios arquivos, entender um fluxo de dados ou um estado que atravessa camadas, e decidir onde exatamente intervir.",
				"A mudanca exige entender uma interacao nao obvia entre partes do sistema, como concorrencia, cache, ordem de eventos ou compatibilidade retroativa, onde a correcao ingenua introduz outro defeito.",
			},
		},
	}
}

// WatchdogQuestions julgam um turno em andamento (spec §6.3). O que e
// detectavel por codigo (comando repetido, arquivo fora do escopo) nao esta
// aqui de proposito.
func WatchdogQuestions() map[string]Question {
	return map[string]Question{
		"sem_progresso": Noul{
			Instructions: "Comparando o turno mais recente em `janela.turno_atual` com os anteriores em `janela.turnos_previos`, o agente deixou de progredir na tarefa.",
			Criteria: &NoulCriteria{
				True:  "O turno repete o que ja foi feito, reexplora o que ja foi visto, ou tenta a mesma coisa de novo sem mudar nada relevante na tentativa.",
				False: "O turno acrescenta informacao nova, avanca para uma etapa seguinte, ou muda de abordagem depois de um resultado.",
			},
		},
		"bloqueio_de_permissao": Noul{
			Instructions: "O turno em `janela.turno_atual` mostra o agente parado a espera de uma confirmacao humana que nao vai chegar, porque ele roda em modo nao interativo.",
			Criteria: &NoulCriteria{
				True:  "Ha pedido de aprovacao, aviso de permissao negada, ou uma ferramenta em estado pendente aguardando resposta.",
				False: "O agente segue trabalhando, mesmo que um comando tenha falhado por outro motivo.",
			},
		},
	}
}

// EvidenceQuestion seleciona evidencia para o briefing (spec §6.5, ida).
func EvidenceQuestion() map[string]Question {
	return map[string]Question{
		"evidencia_necessaria": Noul{
			Instructions: "Quem for executar a tarefa descrita em `tarefa.texto`, sem acesso a conversa que a originou, precisa do conteudo em `item.texto` para fazer o trabalho certo.",
			Criteria: &NoulCriteria{
				True:  "Sem este conteudo, quem executa teria que adivinhar: ele localiza o defeito, define o contrato violado, mostra o erro exato, ou diz como rodar o teste.",
				False: "E contexto de fundo, repeticao do que ja esta dito em outro item, ou detalhe que nao muda nada na execucao.",
			},
		},
	}
}

// CompactionQuestions compactam o trace por delecao (spec §6.5, volta).
// Mecanismo emprestado de fast-jev-compaction: nunca reescrever, so apagar.
func CompactionQuestions() map[string]Question {
	return map[string]Question{
		"chamada_necessaria": Noul{
			Instructions: "Quem for conferir se o trabalho descrito em `tarefa.texto` foi bem feito precisa saber que a acao em `interacao.chamada` aconteceu.",
			Criteria: &NoulCriteria{
				True:  "A acao faz parte da prova: escreveu o teste, rodou o teste, alterou o codigo, ou mostrou o estado que justificou a decisao seguinte.",
				False: "E exploracao, navegacao ou tentativa abandonada, cuja ausencia nao muda a conferencia.",
			},
		},
		"resultado_necessario_verbatim": Noul{
			Instructions: "O conteudo em `interacao.resultado` precisa ser preservado palavra por palavra para que a conferencia do trabalho continue possivel.",
			Criteria: &NoulCriteria{
				True:  "Contem o dado exato que sera conferido: saida de teste, mensagem de erro, diff, valor retornado.",
				False: "E confirmacao generica, listagem de diretorio, ou saida volumosa cujo unico conteudo util e ter dado certo.",
			},
		},
	}
}

// ReportQuestion confere o que o relatorio afirma (spec §6.6). O outro lado
// da comparacao e o exit code capturado, que e codigo, nao modelo.
func ReportQuestion() map[string]Question {
	return map[string]Question{
		"relatorio_afirma_verde": Noul{
			Instructions: "O relatorio em `relatorio.texto` afirma que os testes passaram depois da correcao.",
			Criteria: &NoulCriteria{
				True:  "Afirma que a suite ficou verde, que os testes passam, ou que a correcao foi verificada com sucesso.",
				False: "Relata falha, relata que nao conseguiu rodar, ou nao diz nada sobre o resultado dos testes.",
			},
		},
	}
}

// FindingQuestions triam achados de revisao (spec §6.7). A presenca de
// arquivo:linha e conferida por regex no codigo, nao aqui.
func FindingQuestions() map[string]Question {
	return map[string]Question{
		"tem_cenario_reproduzivel": Noul{
			Instructions: "O achado em `achado.texto` descreve uma situacao concreta em que o defeito aparece, e nao apenas uma opiniao sobre como o codigo deveria ser.",
			Criteria: &NoulCriteria{
				True:  "Descreve entrada, estado ou sequencia de acoes sob a qual o comportamento errado acontece, de forma que alguem poderia tentar reproduzir.",
				False: "Aponta estilo, nomenclatura, preferencia de estrutura, ou afirma que algo e arriscado sem dizer quando quebra.",
			},
		},
		"severidade": Score{
			Instructions: "Avalie a consequencia do problema descrito em `achado.texto` caso ele seja real e chegue a producao.",
			Criteria: []string{
				"Nao muda o comportamento do sistema: e legibilidade, nomenclatura ou preferencia de estilo.",
				"Degrada manutencao ou desempenho de forma perceptivel, mas o sistema continua correto para o usuario.",
				"Produz resultado errado, perda de dado ou falha visivel em algum caminho de execucao que usuarios percorrem.",
				"Abre brecha de seguranca, corrompe dado de forma silenciosa, ou derruba o sistema para todos os usuarios.",
			},
		},
	}
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/jev/ -run "TestAllQuestions|TestDelegability|TestRoute|TestComplexity|TestEverySet" -v`
Expected: PASS.

- [ ] **Step 5: Escrever o teste do ledger**

```go
// internal/jev/budget_test.go
package jev

import (
	"math"
	"path/filepath"
	"testing"
)

func TestLedgerAccumulatesTokensAndCost(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "jev.jsonl")}

	if err := l.Record("gate_delegabilidade", Usage{InputTokens: 1_000_000}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := l.Record("watchdog", Usage{InputTokens: 500_000}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	tokens, usd, err := l.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	if tokens != 1_500_000 {
		t.Errorf("tokens = %d, quero 1500000", tokens)
	}
	// 1,5M x $0,042/M = $0,063
	if math.Abs(usd-0.063) > 1e-9 {
		t.Errorf("usd = %v, quero 0.063", usd)
	}
}

func TestLedgerTotalOnMissingFileIsZero(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "nao-existe.jsonl")}
	tokens, usd, err := l.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	if tokens != 0 || usd != 0 {
		t.Errorf("quero zero, tenho %d/%v", tokens, usd)
	}
}
```

- [ ] **Step 6: Rodar e confirmar o vermelho**

Run: `go test ./internal/jev/ -run TestLedger -v`
Expected: FAIL — `undefined: Ledger`.

- [ ] **Step 7: Implementar o ledger**

```go
// internal/jev/budget.go
package jev

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"time"
)

// InputUSDPerMillion e o preco publicado do jev-latest. Saida e gratuita.
const InputUSDPerMillion = 0.042

// entry e uma linha de jev.jsonl.
type entry struct {
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Version  string    `json:"questions_version"`
	InTokens int       `json:"input_tokens"`
}

// Ledger contabiliza o gasto com Jev de um job. A economia precisa ser
// auditavel, nao prometida.
type Ledger struct{ Path string }

// Record acrescenta uma chamada ao ledger.
func (l *Ledger) Record(kind string, u Usage) error {
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	raw, err := json.Marshal(entry{
		At:       time.Now(),
		Kind:     kind,
		Version:  QuestionsVersion,
		InTokens: u.InputTokens,
	})
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	return err
}

// Total soma tokens e custo. Arquivo ausente significa zero, nao erro.
func (l *Ledger) Total() (int, float64, error) {
	f, err := os.Open(l.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	defer f.Close()

	var tokens int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue // linha corrompida nao invalida a conta inteira
		}
		tokens += e.InTokens
	}
	if err := sc.Err(); err != nil {
		return 0, 0, err
	}
	return tokens, float64(tokens) / 1_000_000 * InputUSDPerMillion, nil
}
```

- [ ] **Step 8: Rodar e confirmar o verde**

Run: `go test ./internal/jev/ -v && go vet ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/jev/questions.go internal/jev/questions_test.go internal/jev/budget.go internal/jev/budget_test.go
git commit -m "feat: perguntas do System One e contabilidade de orcamento"
```

---

### Task 9: Montagem do briefing e subcomando `plan`

A primeira fatia que gera valor de verdade: nenhuma tarefa mal formada passa daqui.

**Files:**
- Create: `internal/gate/evidence.go`
- Create: `internal/gate/briefing.go`
- Create: `internal/gate/gate.go`
- Create: `internal/cli/plan.go`
- Test: `internal/gate/briefing_test.go`, `internal/gate/gate_test.go`, `internal/cli/plan_test.go`

**Interfaces:**
- Consumes: `jev.Client`, `jev.Ledger`, todas as `*Questions()` (Tasks 4 e 8), `job.Create` (Task 7), `devin.SelectModel` (Task 3).
- Produces:
  - `gate.Evidence{Kind, Ref, Text string; Kept bool}` e `gate.ParseEvidence(r io.Reader) ([]Evidence, error)` (JSONL).
  - `gate.Asker` interface `Ask(ctx, state any, qs map[string]jev.Question) (jev.Result, error)` — permite fake nos testes sem HTTP.
  - `gate.SelectEvidence(ctx, a Asker, task string, items []Evidence) ([]Evidence, jev.Usage, error)`.
  - `gate.BuildBriefing(task string, kept []Evidence, limits BriefingLimits) string` com `BriefingLimits{TestCmd, LintCmd, SuiteCmd, VenvPath, NodeModulesPath string; Forbidden []string}`.
  - `gate.Verdict{Delegable bool; Kind string; Missing []string; Warnings []string; Effort devin.Effort; Permission string}`.
  - `gate.Check(ctx, a Asker, task, briefing string, facts RepoFacts) (Verdict, jev.Usage, error)` com `RepoFacts{BranchBase string; CitedFiles, Packages []string}`.
  - `gate.Thresholds` com defaults e `gate.DefaultThresholds()`.
  - `devin.ListModels(ctx) ([]Model, error)` — executa `devin models list` e parseia com o `ParseModelList` da Task 3. Vive aqui, e nao na Task 10, porque quem a consome e o `plan`.

- [ ] **Step 1: Escrever o teste da montagem do briefing**

`BuildBriefing` é pura: sem rede, sem Jev. Ela é o coração do handoff, então tem teste próprio.

```go
// internal/gate/briefing_test.go
package gate

import (
	"strings"
	"testing"
)

func TestBuildBriefingKeepsEvidenceVerbatim(t *testing.T) {
	kept := []Evidence{
		{Kind: "trecho", Ref: "pkg/svc/rota.go:42", Text: "if user.ID == \"\" { return nil }"},
		{Kind: "erro", Ref: "", Text: "panic: runtime error: index out of range [3] with length 3"},
		{Kind: "fonte", Ref: "docs/adr/0007.md", Text: "Toda rota autenticada devolve 401 quando o token expira."},
	}
	out := BuildBriefing("Rota devolve 200 com token expirado.", kept, BriefingLimits{
		TestCmd:   "go test ./pkg/svc/ -run TestRota",
		SuiteCmd:  "go test ./pkg/svc/",
		LintCmd:   "golangci-lint run ./pkg/svc/",
		Forbidden: []string{"infra/", "deploy/"},
	})

	for _, want := range []string{
		"pkg/svc/rota.go:42",
		"if user.ID == \"\" { return nil }",
		"panic: runtime error: index out of range [3] with length 3",
		"Toda rota autenticada devolve 401 quando o token expira.",
		"go test ./pkg/svc/ -run TestRota",
		"golangci-lint run ./pkg/svc/",
		"infra/",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("briefing nao contem %q", want)
		}
	}
}

// Os seis itens do protocolo precisam estar no texto montado, senao o gate
// da Task 9 reprova o proprio template — e o teste precisa pegar isso antes.
func TestBuildBriefingCoversProtocolItems(t *testing.T) {
	out := BuildBriefing("x", []Evidence{{Kind: "trecho", Ref: "a.go:1", Text: "y"}}, BriefingLimits{
		TestCmd: "go test ./...", LintCmd: "go vet ./...",
	})
	for _, want := range []string{
		"confirme o vermelho", // teste antes da correcao
		"nao faca commit",     // limites
		"Relatorio final",     // relatorio
	} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("briefing nao cobre %q", want)
		}
	}
}

func TestBuildBriefingIncludesEnvPathsWhenGiven(t *testing.T) {
	out := BuildBriefing("x", nil, BriefingLimits{
		TestCmd:  "pytest tests/",
		VenvPath: "/Users/h/wt-main/.venv",
	})
	if !strings.Contains(out, "/Users/h/wt-main/.venv") {
		t.Error("o caminho do venv precisa aparecer: worktree nova nao tem ambiente")
	}
}

func TestParseEvidenceReadsJSONL(t *testing.T) {
	in := strings.NewReader(
		`{"kind":"trecho","ref":"a.go:1","text":"x"}` + "\n" +
			`{"kind":"erro","text":"boom"}` + "\n")
	items, err := ParseEvidence(in)
	if err != nil {
		t.Fatalf("ParseEvidence: %v", err)
	}
	if len(items) != 2 || items[0].Ref != "a.go:1" || items[1].Kind != "erro" {
		t.Errorf("parse errado: %+v", items)
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/gate/ -v`
Expected: FAIL — `undefined: BuildBriefing`.

- [ ] **Step 3: Implementar evidência e briefing**

```go
// internal/gate/evidence.go
package gate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Evidence e um item de contexto verbatim vindo do orquestrador.
type Evidence struct {
	Kind string `json:"kind"` // trecho | erro | comando | fonte
	Ref  string `json:"ref"`  // arquivo:linha, quando houver
	Text string `json:"text"`
	Kept bool   `json:"kept"`
}

// ParseEvidence le o JSONL de evidencias.
func ParseEvidence(r io.Reader) ([]Evidence, error) {
	var out []Evidence
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e Evidence
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("evidencia linha %d: %w", line, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}
```

```go
// internal/gate/briefing.go
package gate

import (
	"fmt"
	"strings"
)

// BriefingLimits sao os dados operacionais que o briefing precisa carregar.
type BriefingLimits struct {
	TestCmd         string
	SuiteCmd        string
	LintCmd         string
	VenvPath        string
	NodeModulesPath string
	Forbidden       []string
}

// BuildBriefing monta o briefing a partir do template fixo e das evidencias
// que sobreviveram. Evidencia nunca e reescrita: so entra ou nao entra.
func BuildBriefing(task string, kept []Evidence, l BriefingLimits) string {
	var b strings.Builder

	b.WriteString("# Tarefa\n\n")
	b.WriteString(task)
	b.WriteString("\n\n")

	if refs := refsOf(kept); len(refs) > 0 {
		fmt.Fprintf(&b, "Localizacao: %s\n\n", strings.Join(refs, ", "))
	}

	writeSection(&b, "## Contrato violado", kept, "fonte")
	writeSection(&b, "## Onde esta o defeito", kept, "trecho")
	writeSection(&b, "## Erro observado", kept, "erro")
	writeSection(&b, "## Comandos ja executados", kept, "comando")

	b.WriteString("## Ordem de trabalho\n\n")
	b.WriteString("1. Escreva primeiro o teste que expoe este defeito.\n")
	b.WriteString("2. Rode o teste e **confirme o vermelho** antes de tocar no codigo de producao. ")
	b.WriteString("Cole a saida da falha no relatorio.\n")
	b.WriteString("3. So entao corrija.\n")
	b.WriteString("4. Rode de novo e confirme o verde.\n\n")

	b.WriteString("## Comandos\n\n```bash\n")
	if l.VenvPath != "" {
		fmt.Fprintf(&b, "# interpretador desta tarefa (a worktree nao tem ambiente proprio)\n%s/bin/python -V\n", l.VenvPath)
	}
	if l.NodeModulesPath != "" {
		fmt.Fprintf(&b, "# dependencias: %s\n", l.NodeModulesPath)
	}
	if l.TestCmd != "" {
		fmt.Fprintf(&b, "%s\n", l.TestCmd)
	}
	if l.SuiteCmd != "" {
		fmt.Fprintf(&b, "%s\n", l.SuiteCmd)
	}
	if l.LintCmd != "" {
		fmt.Fprintf(&b, "%s\n", l.LintCmd)
	}
	b.WriteString("```\n\n")

	b.WriteString("## Limites\n\n")
	b.WriteString("- Nao faca commit e nao faca push.\n")
	b.WriteString("- Nao acesse a rede.\n")
	b.WriteString("- Trabalhe apenas nesta worktree.\n")
	for _, f := range l.Forbidden {
		fmt.Fprintf(&b, "- Nao altere nada em `%s`.\n", f)
	}
	b.WriteString("\n")

	b.WriteString("## Relatorio final\n\n")
	b.WriteString("Ao terminar, responda com: o teste que voce escreveu, ")
	b.WriteString("a mudanca feita arquivo por arquivo, e a saida literal dos comandos que voce executou, ")
	b.WriteString("incluindo a do vermelho inicial.\n")

	return b.String()
}

func refsOf(items []Evidence) []string {
	var refs []string
	for _, e := range items {
		if e.Ref != "" {
			refs = append(refs, e.Ref)
		}
	}
	return refs
}

func writeSection(b *strings.Builder, title string, items []Evidence, kind string) {
	var sel []Evidence
	for _, e := range items {
		if e.Kind == kind {
			sel = append(sel, e)
		}
	}
	if len(sel) == 0 {
		return
	}
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, e := range sel {
		if e.Ref != "" {
			fmt.Fprintf(b, "`%s`:\n", e.Ref)
		}
		b.WriteString("```\n")
		b.WriteString(e.Text)
		if !strings.HasSuffix(e.Text, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
	}
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/gate/ -v`
Expected: PASS nos quatro testes.

- [ ] **Step 5: Escrever o teste dos gates**

Os gates são testados com um `Asker` falso: a calibragem do Jev real é assunto de `evals/`, não de teste unitário.

```go
// internal/gate/gate_test.go
package gate

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/heliowap/devin-plugin-cc/internal/jev"
)

// fakeAsker devolve respostas fixas por id de pergunta.
type fakeAsker struct {
	nouls   map[string]float64
	choices map[string]string
	scores  map[string]float64
}

func (f fakeAsker) Ask(_ context.Context, _ any, qs map[string]jev.Question) (jev.Result, error) {
	raw := map[string]any{}
	for id, q := range qs {
		switch q.(type) {
		case jev.Noul:
			raw[id] = map[string]any{"type": "noul", "noul": f.nouls[id]}
		case jev.Choice:
			raw[id] = map[string]any{"type": "choice", "choice": f.choices[id], "confidence": 0.9,
				"probabilities": map[string]any{f.choices[id]: 0.9}}
		case jev.Score:
			raw[id] = map[string]any{"type": "score", "score": f.scores[id], "confidence": 0.8}
		}
	}
	b, _ := json.Marshal(raw)
	var answers jev.Answers
	if err := json.Unmarshal(b, &answers); err != nil {
		return jev.Result{}, err
	}
	return jev.Result{Answers: answers, Usage: jev.Usage{InputTokens: 100}}, nil
}

func healthyAsker() fakeAsker {
	return fakeAsker{
		nouls: map[string]float64{
			"defeito_unico": 0.95, "desenho_em_aberto": 0.05, "cruza_pacotes": 0.1,
			"toca_sensivel": 0.02, "criterio_de_pronto": 0.93,
			"aponta_arquivo_linha": 0.97, "cita_fonte_do_contrato": 0.9,
			"pede_teste_antes_da_correcao": 0.96, "comandos_copiaveis": 0.98,
			"limites_explicitos": 0.94, "pede_relatorio": 0.95,
		},
		choices: map[string]string{"tipo_de_tarefa": "correcao_com_teste", "permissao": "smart"},
		scores:  map[string]float64{"complexidade": 1.2},
	}
}

func TestCheckApprovesHealthyTask(t *testing.T) {
	v, _, err := Check(context.Background(), healthyAsker(), "tarefa", "briefing", RepoFacts{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !v.Delegable {
		t.Fatalf("quero delegavel, Missing=%v", v.Missing)
	}
	if v.Permission != "smart" {
		t.Errorf("Permission = %q, quero smart", v.Permission)
	}
	if v.Kind != "correcao_com_teste" {
		t.Errorf("Kind = %q", v.Kind)
	}
}

func TestCheckRejectsOpenDesign(t *testing.T) {
	a := healthyAsker()
	a.nouls["desenho_em_aberto"] = 0.88

	v, _, err := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if v.Delegable {
		t.Fatal("tarefa com desenho em aberto nao e delegavel")
	}
	if !contains(v.Missing, "desenho_em_aberto") {
		t.Errorf("Missing = %v, quero incluir desenho_em_aberto", v.Missing)
	}
}

func TestCheckRejectsSensitiveTask(t *testing.T) {
	a := healthyAsker()
	a.nouls["toca_sensivel"] = 0.8

	v, _, _ := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
	if v.Delegable {
		t.Fatal("tarefa que toca credencial ou deploy nao e delegavel")
	}
}

func TestCheckNamesMissingBriefingItem(t *testing.T) {
	a := healthyAsker()
	a.nouls["pede_teste_antes_da_correcao"] = 0.2

	v, _, _ := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
	if v.Delegable {
		t.Fatal("briefing sem pedido de vermelho reprova")
	}
	if !contains(v.Missing, "pede_teste_antes_da_correcao") {
		t.Errorf("Missing = %v", v.Missing)
	}
}

// cruza_pacotes e aviso, nao reprovacao: a decisao fica com o autor.
func TestCheckWarnsButApprovesCrossPackage(t *testing.T) {
	a := healthyAsker()
	a.nouls["cruza_pacotes"] = 0.9

	v, _, _ := Check(context.Background(), a, "tarefa", "briefing", RepoFacts{})
	if !v.Delegable {
		t.Fatal("cruza_pacotes nao deve reprovar")
	}
	if !contains(v.Warnings, "cruza_pacotes") {
		t.Errorf("Warnings = %v", v.Warnings)
	}
}

func TestCheckMapsComplexityToEffort(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{{0.2, "medium"}, {1.4, "high"}, {2.8, "max"}}
	for _, c := range cases {
		a := healthyAsker()
		a.scores["complexidade"] = c.score
		v, _, _ := Check(context.Background(), a, "t", "b", RepoFacts{})
		if string(v.Effort) != c.want {
			t.Errorf("score %v -> Effort %q, quero %q", c.score, v.Effort, c.want)
		}
	}
}

// Regra global: a rota nunca escolhe dangerous, nem se o modelo responder isso.
func TestCheckNeverRoutesToDangerous(t *testing.T) {
	a := healthyAsker()
	a.choices["permissao"] = "dangerous"

	v, _, _ := Check(context.Background(), a, "t", "b", RepoFacts{})
	if v.Permission == "dangerous" {
		t.Fatal("dangerous nunca pode sair da rota")
	}
	if v.Permission != "smart" {
		t.Errorf("Permission = %q, quero o fallback smart", v.Permission)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 6: Rodar e confirmar o vermelho**

Run: `go test ./internal/gate/ -run TestCheck -v`
Expected: FAIL — `undefined: Check`.

- [ ] **Step 7: Implementar os gates**

```go
// internal/gate/gate.go
package gate

import (
	"context"
	"fmt"
	"sort"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
	"github.com/heliowap/devin-plugin-cc/internal/jev"
)

// Asker e o que os gates precisam de um cliente Jev. Interface estreita para
// que os testes nao precisem de HTTP.
type Asker interface {
	Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error)
}

// Thresholds sao os limiares de decisao. Ficam em configuracao, nunca
// espalhados como constante magica.
type Thresholds struct {
	DesignOpen    float64 // acima disto, reprova
	Sensitive     float64 // acima disto, reprova
	DoneCriteria  float64 // abaixo disto, reprova
	SingleDefect  float64 // abaixo disto, avisa
	CrossPackage  float64 // acima disto, avisa
	BriefingItem  float64 // abaixo disto, reprova o item
}

// DefaultThresholds traz os valores de partida, a recalibrar com evals/.
func DefaultThresholds() Thresholds {
	return Thresholds{
		DesignOpen: 0.60, Sensitive: 0.50, DoneCriteria: 0.60,
		SingleDefect: 0.50, CrossPackage: 0.70, BriefingItem: 0.60,
	}
}

// RepoFacts sao os fatos que o companion levanta sozinho, sem modelo.
type RepoFacts struct {
	BranchBase string   `json:"branch_base"`
	CitedFiles []string `json:"arquivos_citados"`
	Packages   []string `json:"pacotes_atingidos"`
}

// Verdict e a decisao do gate.
type Verdict struct {
	Delegable  bool          `json:"delegavel"`
	Kind       string        `json:"tipo"`
	Missing    []string      `json:"faltando"`
	Warnings   []string      `json:"avisos"`
	Effort     devin.Effort  `json:"esforco"`
	Permission string        `json:"permissao"`
}

// Check roda delegabilidade, gate de briefing e rota numa unica requisicao.
// Perguntas independentes sobre o mesmo state vao juntas: elas sao avaliadas
// em paralelo e nao veem as respostas umas das outras.
func Check(ctx context.Context, a Asker, task, briefing string, facts RepoFacts) (Verdict, jev.Usage, error) {
	th := DefaultThresholds()

	state := map[string]any{
		"tarefa":    map[string]any{"texto": task},
		"briefing":  map[string]any{"texto": briefing},
		"repo":      facts,
	}

	qs := map[string]jev.Question{}
	for id, q := range jev.DelegabilityQuestions() {
		qs[id] = q
	}
	for id, q := range jev.BriefingQuestions() {
		qs[id] = q
	}
	for id, q := range jev.RouteQuestions() {
		qs[id] = q
	}

	res, err := a.Ask(ctx, state, qs)
	if err != nil {
		return Verdict{}, jev.Usage{}, fmt.Errorf("gate: %w", err)
	}

	v := Verdict{Delegable: true}

	if ch, ok := res.Answers.ChoiceOf("tipo_de_tarefa"); ok {
		v.Kind = ch.Choice
		if ch.Choice == "nao_delegavel" {
			v.Delegable = false
			v.Missing = append(v.Missing, "tipo_de_tarefa=nao_delegavel")
		}
	}

	reject := func(id string, p float64, above bool, limit float64) {
		if (above && p > limit) || (!above && p < limit) {
			v.Delegable = false
			v.Missing = append(v.Missing, id)
		}
	}
	if p, ok := res.Answers.NoulOf("desenho_em_aberto"); ok {
		reject("desenho_em_aberto", p, true, th.DesignOpen)
	}
	if p, ok := res.Answers.NoulOf("toca_sensivel"); ok {
		reject("toca_sensivel", p, true, th.Sensitive)
	}
	if p, ok := res.Answers.NoulOf("criterio_de_pronto"); ok {
		reject("criterio_de_pronto", p, false, th.DoneCriteria)
	}

	// Avisos: sinalizam, mas a decisao fica com o autor.
	if p, ok := res.Answers.NoulOf("cruza_pacotes"); ok && p > th.CrossPackage {
		v.Warnings = append(v.Warnings, "cruza_pacotes")
	}
	if p, ok := res.Answers.NoulOf("defeito_unico"); ok && p < th.SingleDefect {
		v.Warnings = append(v.Warnings, "defeito_unico")
	}

	for id := range jev.BriefingQuestions() {
		if p, ok := res.Answers.NoulOf(id); ok && p < th.BriefingItem {
			v.Delegable = false
			v.Missing = append(v.Missing, id)
		}
	}

	v.Permission = routePermission(res.Answers)
	v.Effort = effortFor(res.Answers)

	sort.Strings(v.Missing)
	sort.Strings(v.Warnings)
	return v, res.Usage, nil
}

// routePermission traduz a Choice em modo de permissao. dangerous nunca sai
// daqui, mesmo que o modelo responda isso: e regra do projeto, nao do modelo.
func routePermission(a jev.Answers) string {
	ch, ok := a.ChoiceOf("permissao")
	if !ok {
		return "smart"
	}
	switch ch.Choice {
	case "auto", "accept-edits", "smart":
		return ch.Choice
	default:
		return "smart"
	}
}

// effortFor traduz o Score de complexidade em nivel de esforco. Os cortes
// acompanham os quatro niveis descritos na pergunta.
func effortFor(a jev.Answers) devin.Effort {
	sc, ok := a.ScoreOf("complexidade")
	if !ok {
		return devin.EffortHigh
	}
	switch {
	case sc.Score < 1.0:
		return devin.EffortMedium
	case sc.Score < 2.0:
		return devin.EffortHigh
	default:
		return devin.EffortMax
	}
}

// SelectEvidence mantem apenas a evidencia necessaria a esta tarefa. Nunca
// reescreve: cada item entra inteiro ou nao entra.
func SelectEvidence(ctx context.Context, a Asker, task string, items []Evidence) ([]Evidence, jev.Usage, error) {
	var total jev.Usage
	var kept []Evidence

	for i := range items {
		state := map[string]any{
			"tarefa": map[string]any{"texto": task},
			"item":   map[string]any{"tipo": items[i].Kind, "ref": items[i].Ref, "texto": items[i].Text},
		}
		res, err := a.Ask(ctx, state, jev.EvidenceQuestion())
		if err != nil {
			return nil, total, fmt.Errorf("selecao de evidencia: %w", err)
		}
		total.InputTokens += res.Usage.InputTokens

		p, ok := res.Answers.NoulOf("evidencia_necessaria")
		if ok && p >= 0.5 {
			items[i].Kept = true
			kept = append(kept, items[i])
		}
	}
	return kept, total, nil
}
```

- [ ] **Step 8: Rodar e confirmar o verde**

Run: `go test ./internal/gate/ -v && go vet ./...`
Expected: PASS nos onze testes.

- [ ] **Step 9: Escrever o teste do subcomando `plan`**

```go
// internal/cli/plan_test.go
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeEvidence(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "evidence.jsonl")
	body := `{"kind":"trecho","ref":"pkg/svc/rota.go:42","text":"return nil"}` + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Sem TYPESAFE_API_KEY o plan nao pode fingir que aprovou: ele recusa,
// porque despachar sem gate e exatamente o que este plugin existe para evitar.
func TestPlanWithoutAPIKeyFailsLoudly(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	dir := t.TempDir()
	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{
		"plan", "--task", "corrigir x", "--evidence", writeEvidence(t, dir), "--worktree", dir,
	}, &out, &errBuf)

	if code != 1 {
		t.Fatalf("exit = %d, quero 1", code)
	}
	if !bytes.Contains(errBuf.Bytes(), []byte("TYPESAFE_API_KEY")) {
		t.Errorf("stderr nao explica a causa: %q", errBuf.String())
	}
}

func TestPlanRejectedTaskExits3WithJSON(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	srv := fakeJevServer(t, map[string]float64{"desenho_em_aberto": 0.95})
	t.Setenv("TYPESAFE_API_KEY", "k")
	t.Setenv("TYPESAFE_BASE_URL", srv)

	dir := t.TempDir()
	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{
		"plan", "--task", "refatorar tudo", "--evidence", writeEvidence(t, dir), "--worktree", dir, "--json",
	}, &out, &errBuf)

	if code != 3 {
		t.Fatalf("exit = %d, quero 3 (reprovado)", code)
	}
	var v struct {
		Delegable bool     `json:"delegavel"`
		Missing   []string `json:"faltando"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("stdout nao e JSON: %v\n%s", err, out.String())
	}
	if v.Delegable {
		t.Error("delegavel deveria ser false")
	}
	if len(v.Missing) == 0 {
		t.Error("faltando deveria nomear o item reprovado")
	}
}
```

Adicione o helper `fakeJevServer` a `internal/cli/testhelpers_test.go`:

```go
// internal/cli/testhelpers_test.go
package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeJevServer responde a qualquer pergunta com valores "saudaveis", exceto
// os ids sobrescritos em overrides. Devolve a URL base.
func fakeJevServer(t *testing.T, overrides map[string]float64) string {
	t.Helper()

	healthy := map[string]float64{
		"defeito_unico": 0.95, "desenho_em_aberto": 0.05, "cruza_pacotes": 0.1,
		"toca_sensivel": 0.02, "criterio_de_pronto": 0.93,
		"aponta_arquivo_linha": 0.97, "cita_fonte_do_contrato": 0.9,
		"pede_teste_antes_da_correcao": 0.96, "comandos_copiaveis": 0.98,
		"limites_explicitos": 0.94, "pede_relatorio": 0.95,
		"evidencia_necessaria": 0.9,
	}
	for k, v := range overrides {
		healthy[k] = v
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Questions map[string]map[string]any `json:"questions"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)

		answers := map[string]any{}
		for id, q := range req.Questions {
			switch {
			case q["criteria"] == nil, isNoul(q):
				answers[id] = map[string]any{"type": "noul", "noul": healthy[id]}
			case isScore(q):
				answers[id] = map[string]any{"type": "score", "score": 1.2, "confidence": 0.8}
			default:
				choice := "correcao_com_teste"
				if id == "permissao" {
					choice = "smart"
				}
				answers[id] = map[string]any{"type": "choice", "choice": choice, "confidence": 0.9,
					"probabilities": map[string]any{choice: 0.9}}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1.13.0", "answers": answers,
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 0},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func isNoul(q map[string]any) bool {
	c, ok := q["criteria"].(map[string]any)
	if !ok {
		return q["criteria"] == nil
	}
	_, hasTrue := c["true"]
	return hasTrue
}

func isScore(q map[string]any) bool {
	_, ok := q["criteria"].([]any)
	return ok
}
```

- [ ] **Step 10: Rodar e confirmar o vermelho**

Run: `go test ./internal/cli/ -run TestPlan -v`
Expected: FAIL — subcomando `plan` não registrado.

- [ ] **Step 11: Implementar o subcomando**

```go
// internal/cli/plan.go
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/heliowap/devin-plugin-cc/internal/gate"
	"github.com/heliowap/devin-plugin-cc/internal/jev"
	"github.com/heliowap/devin-plugin-cc/internal/job"
)

// ExitRejected e o codigo de saida quando o gate reprova a tarefa.
const ExitRejected = 3

type planOutput struct {
	gate.Verdict
	JobID       string  `json:"job_id"`
	Briefing    string  `json:"briefing_path"`
	Model       string  `json:"modelo"`
	JevUSD      float64 `json:"custo_jev_usd"`
	EvidenceIn  int     `json:"evidencias_recebidas"`
	EvidenceOut int     `json:"evidencias_mantidas"`
}

func runPlan(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		task     = fs.String("task", "", "o defeito em uma frase")
		evidence = fs.String("evidence", "", "JSONL de evidencias verbatim")
		worktree = fs.String("worktree", "", "worktree isolada desta tarefa")
		testCmd  = fs.String("test-cmd", "", "comando de teste, copiavel")
		suiteCmd = fs.String("suite-cmd", "", "comando da suite do pacote tocado")
		lintCmd  = fs.String("lint-cmd", "", "comando de lint do repositorio")
		venv     = fs.String("venv", "", "caminho absoluto do venv a reusar")
		nodeMods = fs.String("node-modules", "", "caminho de node_modules a reusar")
		asJSON   = fs.Bool("json", false, "saida em JSON")
	)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
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
		return 1
	}
	_ = ledger.Record("selecao_evidencia", usage)

	briefing := gate.BuildBriefing(*task, kept, gate.BriefingLimits{
		TestCmd: *testCmd, SuiteCmd: *suiteCmd, LintCmd: *lintCmd,
		VenvPath: *venv, NodeModulesPath: *nodeMods,
	})
	if err := os.WriteFile(j.Path("briefing.md"), []byte(briefing), 0o644); err != nil {
		fmt.Fprintf(stderr, "plan: gravando briefing: %v\n", err)
		return 1
	}

	verdict, usage, err := gate.Check(ctx, client, *task, briefing, gate.RepoFacts{})
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return 1
	}
	_ = ledger.Record("gates", usage)

	_, usd, _ := ledger.Total()
	out := planOutput{
		Verdict: verdict, JobID: j.ID, Briefing: j.Path("briefing.md"),
		JevUSD: usd, EvidenceIn: len(items), EvidenceOut: len(kept),
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
	} else {
		renderPlan(stdout, out)
	}

	if !verdict.Delegable {
		_ = job.Release(j.ID)
		return ExitRejected
	}
	return 0
}

func renderPlan(w io.Writer, o planOutput) {
	fmt.Fprintf(w, "job:       %s\n", o.JobID)
	fmt.Fprintf(w, "tipo:      %s\n", o.Kind)
	fmt.Fprintf(w, "evidencia: %d recebidas, %d mantidas\n", o.EvidenceIn, o.EvidenceOut)
	fmt.Fprintf(w, "rota:      %s, esforco %s\n", o.Permission, o.Effort)
	fmt.Fprintf(w, "jev:       $%.5f\n", o.JevUSD)
	if len(o.Warnings) > 0 {
		fmt.Fprintf(w, "avisos:    %v\n", o.Warnings)
	}
	if !o.Delegable {
		fmt.Fprintf(w, "\nREPROVADO. Corrija e rode de novo: %v\n", o.Missing)
		return
	}
	fmt.Fprintf(w, "briefing:  %s\n", o.Briefing)
}
```

Registre em `handlers()` no `internal/cli/cli.go`:

```go
func handlers() map[string]handler {
	return map[string]handler{
		"doctor": runDoctor,
		"plan":   runPlan,
	}
}
```

- [ ] **Step 12: Escrever o teste da resolução de modelo**

O spec §7 exige que o modelo saia da lista viva da conta. Sem este passo o
`plan` decide o esforço e joga fora, e o `task` dispara sem `--model`.

```go
// internal/cli/model_test.go
package cli

import (
	"testing"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
)

// Nenhum UID aparece no codigo: a escolha vem sempre da lista da conta.
func TestResolveModelPrefersFreeForRequestedEffort(t *testing.T) {
	models := []devin.Model{
		{UID: "claude-opus-5-max", ContextTokens: 1_000_000, InputUSDPerM: 5},
		{UID: "swe-2-max", ContextTokens: 262_000, Free: true},
		{UID: "swe-2-medium", ContextTokens: 262_000, Free: true},
	}
	got, err := resolveModel(models, devin.EffortMax)
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if got != "swe-2-max" {
		t.Errorf("got = %q, quero swe-2-max", got)
	}
}

// Quando a familia gratuita sai da promocao, a rota muda sozinha.
func TestResolveModelFallsBackWhenFreeFamilyDisappears(t *testing.T) {
	models := []devin.Model{
		{UID: "claude-opus-5-max", ContextTokens: 1_000_000, InputUSDPerM: 5},
		{UID: "gpt-5-6-luna-max", ContextTokens: 1_000_000, InputUSDPerM: 0.2},
	}
	got, err := resolveModel(models, devin.EffortMax)
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if got != "gpt-5-6-luna-max" {
		t.Errorf("got = %q, quero o mais barato", got)
	}
}

func TestResolveModelErrorsOnEmptyList(t *testing.T) {
	if _, err := resolveModel(nil, devin.EffortMax); err == nil {
		t.Fatal("quero erro com lista vazia")
	}
}
```

- [ ] **Step 13: Rodar e confirmar o vermelho**

Run: `go test ./internal/cli/ -run TestResolveModel -v`
Expected: FAIL — `undefined: resolveModel`.

- [ ] **Step 14: Ligar a rota ao job**

Primeiro, acrescente o listador a `internal/devin/models.go` — ele consome o
`ParseModelList` da Task 3 e e o unico ponto do projeto que executa
`devin models list`:

```go
// acrescentar a internal/devin/models.go
// (imports novos: "bytes", "context", "os/exec")

// ListModels le a lista viva de modelos da conta.
func ListModels(ctx context.Context) ([]Model, error) {
	out, err := exec.CommandContext(ctx, "devin", "models", "list").Output()
	if err != nil {
		return nil, fmt.Errorf("devin models list: %w", err)
	}
	return ParseModelList(bytes.NewReader(out))
}
```

Depois crie `internal/cli/model.go`:

```go
// internal/cli/model.go
package cli

import (
	"fmt"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
)

// resolveModel escolhe o UID do modelo para o esforco pedido. Existe como
// funcao propria para ser testavel sem executar o devin.
func resolveModel(models []devin.Model, want devin.Effort) (string, error) {
	m, err := devin.SelectModel(models, want)
	if err != nil {
		return "", fmt.Errorf("resolvendo modelo: %w", err)
	}
	return m.UID, nil
}
```

Em `internal/cli/plan.go`, acrescente as flags de worktree e, depois de
`gate.Check`, resolva o modelo e persista a rota no job. Substitua o trecho
que vai de `_, usd, _ := ledger.Total()` ate o `return 0` final por:

```go
	// Guarda a evidencia recebida no job, com a marca do que sobreviveu
	// (spec §10): sem isso nao da para auditar uma selecao depois.
	if f, err := os.Create(j.Path("evidence.jsonl")); err == nil {
		enc := json.NewEncoder(f)
		for _, e := range items {
			_ = enc.Encode(e)
		}
		f.Close()
	}

	if verdict.Delegable {
		models, err := devin.ListModels(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "plan: %v\n", err)
			_ = job.Release(j.ID)
			return 1
		}
		uid, err := resolveModel(models, verdict.Effort)
		if err != nil {
			fmt.Fprintf(stderr, "plan: %v\n", err)
			_ = job.Release(j.ID)
			return 1
		}
		j.Model, j.Permission = uid, verdict.Permission
		j.Repo, j.Branch, j.Base = *repo, *branch, *base
		if err := j.Save(); err != nil {
			fmt.Fprintf(stderr, "plan: %v\n", err)
			return 1
		}
	}

	_, usd, _ := ledger.Total()
	out := planOutput{
		Verdict: verdict, JobID: j.ID, Briefing: j.Path("briefing.md"),
		Model: j.Model, JevUSD: usd, EvidenceIn: len(items), EvidenceOut: len(kept),
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
	} else {
		renderPlan(stdout, out)
	}

	if !verdict.Delegable {
		_ = job.Release(j.ID)
		return ExitRejected
	}
	return 0
}
```

Acrescente as flags ao `FlagSet` do `plan`:

```go
		repo   = fs.String("repo", ".", "repositorio de origem da worktree")
		branch = fs.String("branch", "", "branch a criar para a tarefa")
		base   = fs.String("base", "HEAD", "commit base da worktree")
```

E os campos ao `job.Job` em `internal/job/store.go`, junto de `Branch`:

```go
	Repo string `json:"repo"`
	Base string `json:"base"`
```

Acrescente `Model string \`json:"modelo"\`` a `planOutput`, que ja o declara,
e o import de `devin` em `plan.go`.

- [ ] **Step 15: Rodar e confirmar o verde**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 16: Commit**

```bash
git add internal/gate internal/cli internal/job
git commit -m "feat: montagem de briefing por selecao, gates e rota de modelo no plan"
```

---

### Task 10: Dispatch — `task` e `supervise`

**Files:**
- Create: `internal/gitx/worktree.go`
- Create: `internal/devin/exec.go`
- Create: `internal/cli/task.go`
- Create: `internal/cli/supervise.go`
- Test: `internal/devin/exec_test.go`, `internal/cli/task_test.go`

**Interfaces:**
- Consumes: `job.Job` (Task 7), `devin.Model`/`SelectModel` (Task 3).
- Produces:
  - `gitx.AddWorktree(ctx, repo, dir, branch, base string) error`, `gitx.RemoveWorktree(ctx, repo, dir string) error`.
  - `devin.RunArgs{Model, Permission, PromptFile, ExportFile string}` e `(RunArgs) Flags() []string`.
  - `devin.Start(ctx, args RunArgs, dir string, stdout io.Writer) (*exec.Cmd, error)` — spawn com `Setpgid`.
  - `devin.KillGroup(pgid int, grace time.Duration) error` — `SIGTERM` no grupo, `SIGKILL` após a carência.

- [ ] **Step 1: Escrever o teste da montagem de flags e do kill de grupo**

```go
// internal/devin/exec_test.go
package devin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/testsupport"
)

func TestRunArgsFlags(t *testing.T) {
	got := strings.Join(RunArgs{
		Model: "swe-2-max", Permission: "smart",
		PromptFile: "/j/briefing.md", ExportFile: "/j/export.json",
	}.Flags(), " ")

	for _, want := range []string{
		"--model swe-2-max",
		"--permission-mode smart",
		"--respect-workspace-trust false", // obrigatorio: -p nao exibe o prompt de confianca
		"--prompt-file /j/briefing.md",
		"--export /j/export.json",
		"-p",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("flags nao contem %q\ntenho: %s", want, got)
		}
	}
}

// dangerous so chega ao devin se vier de fora; a rota nunca o produz.
func TestRunArgsPassesPermissionThroughVerbatim(t *testing.T) {
	got := strings.Join(RunArgs{Permission: "auto"}.Flags(), " ")
	if !strings.Contains(got, "--permission-mode auto") {
		t.Errorf("flags = %s", got)
	}
}

func TestStartSetsProcessGroupAndKillGroupStopsIt(t *testing.T) {
	testsupport.InstallFakeDevin(t, "permission-block")
	t.Setenv("FAKEDEVIN_STEP_MS", "10")

	dir := t.TempDir()
	prompt := filepath.Join(dir, "b.md")
	if err := os.WriteFile(prompt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd, err := Start(context.Background(), RunArgs{
		PromptFile: prompt, ExportFile: filepath.Join(dir, "export.json"),
	}, dir, os.Stderr)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Getpgid: %v", err)
	}
	if pgid != cmd.Process.Pid {
		t.Errorf("pgid = %d, quero que seja o proprio pid %d", pgid, cmd.Process.Pid)
	}

	// O cenario permission-block trava de proposito; sem KillGroup ele nao sai.
	if err := KillGroup(pgid, 200*time.Millisecond); err != nil {
		t.Fatalf("KillGroup: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("o processo sobreviveu ao KillGroup")
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/devin/ -run "TestRunArgs|TestStart" -v`
Expected: FAIL — `undefined: RunArgs`.

- [ ] **Step 3: Implementar**

```go
// internal/devin/exec.go
package devin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// RunArgs sao os parametros de uma execucao nao interativa do devin.
type RunArgs struct {
	Model      string
	Permission string
	PromptFile string
	ExportFile string
}

// Flags monta a linha de comando. --respect-workspace-trust false e
// obrigatorio: o modo -p nao consegue exibir o prompt de confianca e falha
// sem isso num diretorio nao confiado.
func (a RunArgs) Flags() []string {
	var f []string
	if a.Model != "" {
		f = append(f, "--model", a.Model)
	}
	if a.Permission != "" {
		f = append(f, "--permission-mode", a.Permission)
	}
	f = append(f, "--respect-workspace-trust", "false")
	if a.PromptFile != "" {
		f = append(f, "--prompt-file", a.PromptFile)
	}
	if a.ExportFile != "" {
		f = append(f, "--export", a.ExportFile)
	}
	return append(f, "-p")
}

// Start dispara o devin em dir, em grupo de processos proprio, para que o
// cancelamento alcance os filhos que ele criar.
func Start(ctx context.Context, a RunArgs, dir string, stdout io.Writer) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "devin", a.Flags()...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("devin: start: %w", err)
	}
	return cmd, nil
}

// KillGroup manda SIGTERM ao grupo e, passada a carencia, SIGKILL.
func KillGroup(pgid int, grace time.Duration) error {
	if pgid <= 1 {
		return fmt.Errorf("devin: pgid invalido %d", pgid)
	}
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("devin: SIGTERM no grupo %d: %w", pgid, err)
	}
	time.Sleep(grace)
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("devin: SIGKILL no grupo %d: %w", pgid, err)
	}
	return nil
}

```

```go
// internal/gitx/worktree.go
package gitx

import (
	"context"
	"fmt"
	"os/exec"
)

func run(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return nil
}

// AddWorktree cria a worktree isolada da tarefa. Um agente por pasta.
func AddWorktree(ctx context.Context, repo, dir, branch, base string) error {
	return run(ctx, repo, "worktree", "add", "-b", branch, dir, base)
}

// RemoveWorktree descarta a worktree.
func RemoveWorktree(ctx context.Context, repo, dir string) error {
	return run(ctx, repo, "worktree", "remove", "--force", dir)
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/devin/ -v`
Expected: PASS.

- [ ] **Step 5: Escrever o teste do dispatch**

```go
// internal/cli/task_test.go
package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/job"
	"github.com/heliowap/devin-plugin-cc/internal/testsupport"
)

var jobIDRe = regexp.MustCompile(`job-[0-9a-f]{10}`)

// task devolve o id na hora: a delegacao nao pode bloquear o turno do Claude.
func TestTaskReturnsImmediatelyWithJobID(t *testing.T) {
	testsupport.InstallFakeDevin(t, "healthy")
	t.Setenv("FAKEDEVIN_STEP_MS", "200")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TYPESAFE_API_KEY", "")

	wt := t.TempDir()
	j, err := job.Create(wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.Path("briefing.md"), []byte("corrija"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	start := time.Now()
	code := Run(context.Background(), []string{"task", "--job", j.ID, "--no-watchdog"}, &out, &errBuf)
	elapsed := time.Since(start)

	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errBuf.String())
	}
	if elapsed > 3*time.Second {
		t.Errorf("task bloqueou por %v; deveria retornar na hora", elapsed)
	}
	if !jobIDRe.MatchString(out.String()) {
		t.Errorf("stdout nao traz o job id: %q", out.String())
	}

	waitForJobState(t, j.ID, job.StateCompleted, 20*time.Second)

	raw, err := os.ReadFile(filepath.Join(j.Dir(), "export.json"))
	if err != nil || len(raw) == 0 {
		t.Fatalf("o supervisor nao produziu export: %v", err)
	}
}

func waitForJobState(t *testing.T, id string, want job.State, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if j, err := job.Load(id); err == nil && j.State == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	j, _ := job.Load(id)
	t.Fatalf("timeout esperando estado %q; estado atual: %q", want, j.State)
}
```

- [ ] **Step 6: Rodar e confirmar o vermelho**

Run: `go test ./internal/cli/ -run TestTask -v`
Expected: FAIL — subcomando `task` não registrado.

- [ ] **Step 7: Implementar `task` e `supervise`**

```go
// internal/cli/task.go
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/heliowap/devin-plugin-cc/internal/job"
)

func runTask(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("task", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		jobID       = fs.String("job", "", "id do job criado por plan")
		noWatchdog  = fs.Bool("no-watchdog", false, "desliga o watchdog (diagnostico)")
	)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *jobID == "" {
		fmt.Fprintln(stderr, "task: --job e obrigatorio; rode plan antes")
		return ExitUsage
	}
	j, err := job.Load(*jobID)
	if err != nil {
		fmt.Fprintf(stderr, "task: %v\n", err)
		return 1
	}

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "task: %v\n", err)
		return 1
	}

	superviseArgs := []string{"supervise", "--job", j.ID}
	if *noWatchdog {
		superviseArgs = append(superviseArgs, "--no-watchdog")
	}

	// Destacado: o supervisor sobrevive ao fim deste processo e ao fim do
	// turno do Claude. Saida vai para o log do job, nao para este stdout.
	logFile, err := os.Create(j.Path("supervisor.log"))
	if err != nil {
		fmt.Fprintf(stderr, "task: %v\n", err)
		return 1
	}
	defer logFile.Close()

	cmd := exec.Command(self, superviseArgs...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "task: iniciando supervisor: %v\n", err)
		return 1
	}
	_ = cmd.Process.Release()

	fmt.Fprintf(stdout, "%s\n", j.ID)
	fmt.Fprintf(stdout, "worktree: %s\n", j.Worktree)
	fmt.Fprintf(stdout, "acompanhe: devin-companion wait-and-result %s\n", j.ID)
	return 0
}
```

```go
// internal/cli/supervise.go
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
	"github.com/heliowap/devin-plugin-cc/internal/job"
	"github.com/heliowap/devin-plugin-cc/internal/watchdog"
)

func runSupervise(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("supervise", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		jobID      = fs.String("job", "", "id do job")
		noWatchdog = fs.Bool("no-watchdog", false, "desliga o watchdog")
	)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	j, err := job.Load(*jobID)
	if err != nil {
		fmt.Fprintf(stderr, "supervise: %v\n", err)
		return 1
	}

	logFile, err := os.Create(j.Path("stdout.log"))
	if err != nil {
		fmt.Fprintf(stderr, "supervise: %v\n", err)
		return 1
	}
	defer logFile.Close()

	cmd, err := devin.Start(ctx, devin.RunArgs{
		Model:      j.Model,
		Permission: j.Permission,
		PromptFile: j.Path("briefing.md"),
		ExportFile: j.Path("export.json"),
	}, j.Worktree, logFile)
	if err != nil {
		j.State = job.StateFailed
		_ = j.Save()
		fmt.Fprintf(stderr, "supervise: %v\n", err)
		return 1
	}

	j.PID = cmd.Process.Pid
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		j.PGID = pgid
	}
	j.State = job.StateRunning
	if err := j.Save(); err != nil {
		fmt.Fprintf(stderr, "supervise: %v\n", err)
	}

	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	if !*noWatchdog {
		go watchdog.Watch(watchCtx, j)
	}

	waitErr := cmd.Wait()
	stopWatch()

	reloaded, err := job.Load(j.ID)
	if err == nil && reloaded.State == job.StateCancelled {
		// O watchdog ja decidiu; nao sobrescreva o motivo dele.
		return 0
	}
	if waitErr != nil {
		j.State = job.StateFailed
	} else {
		j.State = job.StateCompleted
	}
	_ = j.Save()
	return 0
}
```

Registre `task` e `supervise` em `handlers()`.

- [ ] **Step 7b: Criar a worktree no dispatch**

`gitx.AddWorktree` existe desde o Step 3 mas ainda não tem chamador. O spec
§5.2 põe a criação no `task`. Acrescente o teste:

```go
// internal/cli/worktree_test.go
package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnsureWorktreeCreatesWhenMissing(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-qm", "base")

	wt := filepath.Join(t.TempDir(), "wt-tarefa")
	if err := ensureWorktree(context.Background(), repo, wt, "tarefa-1", "HEAD"); err != nil {
		t.Fatalf("ensureWorktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, "a.txt")); err != nil {
		t.Errorf("a worktree nao foi criada: %v", err)
	}
}

// Worktree que ja existe nao e recriada: recriar apagaria trabalho.
func TestEnsureWorktreeIsIdempotent(t *testing.T) {
	existing := t.TempDir()
	if err := ensureWorktree(context.Background(), "/nao/existe", existing, "b", "HEAD"); err != nil {
		t.Errorf("diretorio existente deveria ser aceito como esta: %v", err)
	}
}
```

Implemente em `internal/cli/worktree.go`:

```go
// internal/cli/worktree.go
package cli

import (
	"context"
	"os"

	"github.com/heliowap/devin-plugin-cc/internal/gitx"
)

// ensureWorktree garante a pasta isolada da tarefa. Diretorio que ja existe
// e aceito como esta: recriar apagaria trabalho.
func ensureWorktree(ctx context.Context, repo, dir, branch, base string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	return gitx.AddWorktree(ctx, repo, dir, branch, base)
}
```

E chame-a em `runTask`, logo depois de carregar o job e antes de disparar o
supervisor:

```go
	if j.Repo != "" && j.Branch != "" {
		if err := ensureWorktree(ctx, j.Repo, j.Worktree, j.Branch, j.Base); err != nil {
			fmt.Fprintf(stderr, "task: %v\n", err)
			return 1
		}
	}
```

Run: `go test ./internal/cli/ -run TestEnsureWorktree -v`
Expected: PASS nos dois testes.

- [ ] **Step 8: Criar o stub do watchdog para compilar**

A Task 11 implementa de verdade; aqui só o suficiente para o supervisor compilar e o teste passar.

```go
// internal/watchdog/watchdog.go
package watchdog

import (
	"context"

	"github.com/heliowap/devin-plugin-cc/internal/job"
)

// Watch observa o job ate o contexto ser cancelado. A Task 11 preenche isto.
func Watch(ctx context.Context, j *job.Job) {
	<-ctx.Done()
}
```

- [ ] **Step 9: Rodar e confirmar o verde**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/gitx internal/devin/exec.go internal/devin/exec_test.go internal/cli internal/watchdog
git commit -m "feat: dispatch destacado com supervisor e grupo de processos"
```

---

### Task 11: Watchdog — sinais determinísticos

Nada de modelo aqui. O que dá para saber contando, se sabe contando.

**Files:**
- Create: `internal/watchdog/signals.go`
- Test: `internal/watchdog/signals_test.go`

**Interfaces:**
- Consumes: `devin.Turn` (Task 6).
- Produces:
  - `watchdog.Signal{Name string; Fired bool; Probability float64; Excerpt string}`
  - `watchdog.RepeatedFailure(turns []devin.Turn, n int) Signal` — mesmo comando com a mesma saída falhando `n` vezes. **Pós-run apenas**, consumido pelo `result` (Task 14) para nomear o que deu errado; ao vivo, quem cobre esse caso é o `sem_progresso` do Jev, porque o stdout não traz o par chamada/resultado estruturado.
  - `watchdog.OutOfScope(changed []string, allowed []string) Signal` — escrita fora dos prefixos permitidos. Recebe caminhos, não turnos: ao vivo eles vêm de `git status --porcelain` na worktree, que é deterministico e independe do formato da saída do `devin`.
  - `watchdog.Stalled(lastWrite time.Time, now time.Time, limit time.Duration) Signal`.
  - `watchdog.PermissionRejected(out string) Signal` — a linha literal que o `devin` emite ao esbarrar numa confirmação impossível em modo `-p`.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/watchdog/signals_test.go
package watchdog

import (
	"testing"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
)

func turnsWith(tools ...devin.ToolInteraction) []devin.Turn {
	var ts []devin.Turn
	for i, tool := range tools {
		ts = append(ts, devin.Turn{Index: i, Tools: []devin.ToolInteraction{tool}})
	}
	return ts
}

func TestRepeatedFailureFiresAfterThreshold(t *testing.T) {
	fail := devin.ToolInteraction{Name: "exec", Input: "go test ./x/", Output: "FAIL: no module", Status: "erro"}
	s := RepeatedFailure(turnsWith(fail, fail, fail), 3)
	if !s.Fired {
		t.Fatal("tres falhas identicas deveriam disparar")
	}
	if s.Excerpt == "" {
		t.Error("o sinal precisa carregar o trecho que o provocou")
	}
}

func TestRepeatedFailureDoesNotFireBelowThreshold(t *testing.T) {
	fail := devin.ToolInteraction{Name: "exec", Input: "go test ./x/", Output: "FAIL", Status: "erro"}
	if RepeatedFailure(turnsWith(fail, fail), 3).Fired {
		t.Error("duas falhas nao deveriam disparar com limiar 3")
	}
}

// Falhar, mudar de abordagem e falhar de novo nao e travar.
func TestRepeatedFailureIgnoresDifferentCommands(t *testing.T) {
	a := devin.ToolInteraction{Name: "exec", Input: "go test ./x/", Output: "FAIL", Status: "erro"}
	b := devin.ToolInteraction{Name: "exec", Input: "go test ./y/", Output: "FAIL", Status: "erro"}
	if RepeatedFailure(turnsWith(a, b, a), 3).Fired {
		t.Error("comandos diferentes nao caracterizam repeticao")
	}
}

// A entrada vem de `git status --porcelain`, entao so arquivo efetivamente
// alterado aparece: leitura fora do escopo nunca chega aqui, e e legitima.
func TestOutOfScopeDetectsForbiddenWrite(t *testing.T) {
	s := OutOfScope([]string{"pkg/svc/rota.go", "infra/deploy.yaml"}, []string{"pkg/svc/"})
	if !s.Fired {
		t.Fatal("escrita em infra/ deveria disparar")
	}
	if !contains(s.Excerpt, "infra/deploy.yaml") {
		t.Errorf("Excerpt = %q, quero nomear o arquivo", s.Excerpt)
	}
}

func TestOutOfScopeQuietWhenAllInside(t *testing.T) {
	if OutOfScope([]string{"pkg/svc/rota.go", "pkg/svc/rota_test.go"}, []string{"pkg/svc/"}).Fired {
		t.Error("tudo dentro do escopo nao deveria disparar")
	}
}

func TestOutOfScopeWithNoAllowListNeverFires(t *testing.T) {
	if OutOfScope([]string{"qualquer.go"}, nil).Fired {
		t.Error("sem lista de escopo declarada, o sinal fica desligado")
	}
}

// Medido contra devin 3000.10.31: ao esbarrar numa confirmacao que o modo -p
// nao consegue exibir, ele emite esta linha e encerra. Isso e string estavel,
// nao julgamento — nao tem por que custar uma chamada de modelo.
func TestPermissionRejectedDetectsLiteralWarning(t *testing.T) {
	out := "Vou inspecionar o repositorio.\nwarning: rejected a tool call that requires confirmation. " +
		"Running in non-interactive mode. Use --permission-mode dangerous to auto-approve all tools.\n"
	s := PermissionRejected(out)
	if !s.Fired {
		t.Fatal("a recusa de ferramenta deveria disparar")
	}
	if s.Probability != 1 {
		t.Errorf("Probability = %v; sinal deterministico vale 1", s.Probability)
	}
	if s.Excerpt == "" {
		t.Error("o sinal precisa carregar a linha que o provocou")
	}
}

func TestPermissionRejectedQuietOnNormalOutput(t *testing.T) {
	if PermissionRejected("rodando go test ./...\nok  pkg/svc  0.4s\n").Fired {
		t.Error("saida normal nao deveria disparar")
	}
}

func TestStalledFiresAfterLimit(t *testing.T) {
	now := time.Now()
	if !Stalled(now.Add(-10*time.Minute), now, 5*time.Minute).Fired {
		t.Error("10 min sem escrita com limite de 5 deveria disparar")
	}
	if Stalled(now.Add(-1*time.Minute), now, 5*time.Minute).Fired {
		t.Error("1 min sem escrita nao deveria disparar")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/watchdog/ -v`
Expected: FAIL — `undefined: RepeatedFailure`.

- [ ] **Step 3: Implementar**

```go
// internal/watchdog/signals.go
package watchdog

import (
	"fmt"
	"strings"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
)

// Signal e a deteccao de um sintoma de run condenado. Probability e 1 para
// sinais deterministicos: eles nao estimam, contam.
type Signal struct {
	Name        string
	Fired       bool
	Probability float64
	Excerpt     string
}

// RepeatedFailure dispara quando o mesmo comando falha n vezes com a mesma
// saida. Comando diferente, ou saida diferente, significa que ele mudou de
// abordagem — isso e progresso, nao repeticao.
func RepeatedFailure(turns []devin.Turn, n int) Signal {
	counts := map[string]int{}
	for _, t := range turns {
		for _, tool := range t.Tools {
			if tool.Status != "erro" {
				continue
			}
			key := tool.Name + "\x00" + tool.Input + "\x00" + tool.Output
			counts[key]++
			if counts[key] >= n {
				return Signal{
					Name: "comando_repetido", Fired: true, Probability: 1,
					Excerpt: fmt.Sprintf("%s(%s) falhou %d vezes com a mesma saida: %s",
						tool.Name, tool.Input, counts[key], truncate(tool.Output, 200)),
				}
			}
		}
	}
	return Signal{Name: "comando_repetido"}
}

// OutOfScope dispara quando um arquivo alterado sai dos prefixos permitidos.
// Recebe caminhos de `git status --porcelain`, nao turnos: assim o sinal nao
// depende do formato da saida do devin. Sem lista declarada fica desligado —
// nao se inventa escopo.
func OutOfScope(changed []string, allowed []string) Signal {
	if len(allowed) == 0 {
		return Signal{Name: "fora_do_escopo"}
	}
	for _, path := range changed {
		if !hasAnyPrefix(path, allowed) {
			return Signal{
				Name: "fora_do_escopo", Fired: true, Probability: 1,
				Excerpt: fmt.Sprintf("%s foi alterado, fora de %v", path, allowed),
			}
		}
	}
	return Signal{Name: "fora_do_escopo"}
}

// rejectionMarker e a linha que o devin emite ao esbarrar numa confirmacao
// que o modo -p nao consegue exibir. Medido contra devin 3000.10.31.
const rejectionMarker = "rejected a tool call that requires confirmation"

// PermissionRejected dispara na presenca dessa linha. E terminal por
// natureza: o processo nao se recupera de uma confirmacao que ninguem pode dar.
func PermissionRejected(out string) Signal {
	i := strings.Index(out, rejectionMarker)
	if i < 0 {
		return Signal{Name: "bloqueio_de_permissao"}
	}
	end := i + len(rejectionMarker) + 120
	if end > len(out) {
		end = len(out)
	}
	start := i - 120
	if start < 0 {
		start = 0
	}
	return Signal{
		Name: "bloqueio_de_permissao", Fired: true, Probability: 1,
		Excerpt: out[start:end],
	}
}

// Stalled dispara quando o stdout para de crescer por tempo demais.
func Stalled(lastWrite, now time.Time, limit time.Duration) Signal {
	idle := now.Sub(lastWrite)
	if idle < limit {
		return Signal{Name: "parado"}
	}
	return Signal{
		Name: "parado", Fired: true, Probability: 1,
		Excerpt: fmt.Sprintf("nenhum turno novo ha %s (limite %s)", idle.Round(time.Second), limit),
	}
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/watchdog/ -v && go vet ./...`
Expected: PASS nos sete testes.

- [ ] **Step 5: Commit**

```bash
git add internal/watchdog/signals.go internal/watchdog/signals_test.go
git commit -m "feat: sinais deterministicos do watchdog"
```

---

### Task 12: Watchdog — sinais Jev, política e cancelamento

Aqui o tempo começa a ser economizado de verdade.

**Files:**
- Modify: `internal/watchdog/watchdog.go` (substitui o stub da Task 10)
- Create: `internal/watchdog/policy.go`
- Test: `internal/watchdog/policy_test.go`, `internal/watchdog/watchdog_test.go`

> **Correção medida em 2026-09-20.** O spec original mandava o watchdog fazer poll de `export.json` "a cada turno", seguindo a documentação do Devin. Isso é falso: com um run em andamento e arquivos já escritos, `export.json` não existia, enquanto `stdout.log` crescia. O watchdog lê **stdout**, incrementalmente, por janelas de texto. `export.json` continua servindo à Task 14, onde já está completo.

**Interfaces:**
- Consumes: `devin.KillGroup`, `job.Job`/`CancelReason`, `jev.WatchdogQuestions`, `gate.Asker`.
- Produces:
  - `watchdog.Config{PollInterval, StallLimit time.Duration; RepeatThreshold int; AllowedPaths []string; NoProgress float64; ConsecutiveTurns int; WindowBytes int}` e `watchdog.DefaultConfig()`.
  - `watchdog.NewPolicy(cfg Config) *Policy` e `(*Policy) Observe(sigs []Signal) *Signal` — devolve o sinal que autoriza cancelar, ou `nil`.
  - `watchdog.Watch(ctx context.Context, j *job.Job, a gate.Asker, cfg Config)` — lê `<job>/stdout.log`.

- [ ] **Step 1: Escrever o teste da política**

```go
// internal/watchdog/policy_test.go
package watchdog

import "testing"

func fired(name string) Signal {
	return Signal{Name: name, Fired: true, Probability: 0.95, Excerpt: "trecho"}
}

// Um turno ruim nao cancela: agente que tropeca e se levanta e normal.
func TestPolicyRequiresTwoConsecutiveTurns(t *testing.T) {
	p := NewPolicy(DefaultConfig())

	if got := p.Observe([]Signal{fired("sem_progresso")}); got != nil {
		t.Fatal("um turno so nao deveria cancelar")
	}
	got := p.Observe([]Signal{fired("sem_progresso")})
	if got == nil {
		t.Fatal("dois turnos consecutivos deveriam cancelar")
	}
	if got.Name != "sem_progresso" {
		t.Errorf("Name = %q", got.Name)
	}
}

// Sinal que some no meio zera a contagem: nao se acumula evidencia velha.
func TestPolicyResetsWhenSignalClears(t *testing.T) {
	p := NewPolicy(DefaultConfig())

	p.Observe([]Signal{fired("sem_progresso")})
	p.Observe([]Signal{{Name: "sem_progresso"}}) // nao disparou
	if got := p.Observe([]Signal{fired("sem_progresso")}); got != nil {
		t.Fatal("a contagem deveria ter zerado no turno limpo")
	}
}

// Bloqueio de permissao e terminal por natureza: esperar o segundo turno
// seria esperar por algo que nunca vem.
func TestPolicyCancelsImmediatelyOnToolRejection(t *testing.T) {
	p := NewPolicy(DefaultConfig())
	got := p.Observe([]Signal{fired("bloqueio_de_permissao")})
	if got == nil {
		t.Fatal("bloqueio_de_permissao deveria cancelar no primeiro turno")
	}
}

func TestPolicyIgnoresLowProbability(t *testing.T) {
	p := NewPolicy(DefaultConfig())
	weak := Signal{Name: "sem_progresso", Fired: true, Probability: 0.3}
	p.Observe([]Signal{weak})
	if got := p.Observe([]Signal{weak}); got != nil {
		t.Fatal("probabilidade abaixo do limiar nao cancela")
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/watchdog/ -run TestPolicy -v`
Expected: FAIL — `undefined: NewPolicy`.

- [ ] **Step 3: Implementar a política**

```go
// internal/watchdog/policy.go
package watchdog

import "time"

// Config reune os limiares do watchdog. Todos vem de configuracao.
type Config struct {
	PollInterval     time.Duration
	StallLimit       time.Duration
	RepeatThreshold  int
	AllowedPaths     []string
	NoProgress       float64
	ConsecutiveTurns int
	WindowBytes      int // quanto de stdout novo vira uma janela de julgamento
}

// DefaultConfig traz os valores de partida, a recalibrar com uso real.
func DefaultConfig() Config {
	return Config{
		PollInterval:     5 * time.Second,
		StallLimit:       8 * time.Minute,
		RepeatThreshold:  3,
		NoProgress:       0.70,
		ConsecutiveTurns: 2,
		WindowBytes:      16 * 1024,
	}
}

// immediate sao os sinais que nao esperam confirmacao: esperar por um
// segundo turno seria esperar por algo que nao vai acontecer.
var immediate = map[string]bool{
	"bloqueio_de_permissao": true,
	"fora_do_escopo":        true,
	"parado":                true,
}

// Policy decide quando um sinal autoriza o cancelamento.
type Policy struct {
	cfg    Config
	streak map[string]int
}

// NewPolicy cria a politica.
func NewPolicy(cfg Config) *Policy {
	return &Policy{cfg: cfg, streak: map[string]int{}}
}

func (p *Policy) threshold(name string) float64 {
	if name == "sem_progresso" {
		return p.cfg.NoProgress
	}
	// Os demais sao deterministicos e chegam com probabilidade 1.
	return 0.5
}

// Observe processa os sinais de um turno e devolve o que autoriza cancelar.
func (p *Policy) Observe(sigs []Signal) *Signal {
	seen := map[string]bool{}

	for i := range sigs {
		s := sigs[i]
		seen[s.Name] = true
		if !s.Fired || s.Probability < p.threshold(s.Name) {
			p.streak[s.Name] = 0
			continue
		}
		p.streak[s.Name]++
		if immediate[s.Name] || p.streak[s.Name] >= p.cfg.ConsecutiveTurns {
			return &s
		}
	}
	for name := range p.streak {
		if !seen[name] {
			p.streak[name] = 0
		}
	}
	return nil
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/watchdog/ -run TestPolicy -v`
Expected: PASS.

- [ ] **Step 5: Escrever o teste de integração do watchdog**

```go
// internal/watchdog/watchdog_test.go
package watchdog

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/jev"
)

// stubAsker responde sempre o mesmo, para exercitar o laco sem rede.
type stubAsker struct{ noProgress, permBlock float64 }

func (s stubAsker) Ask(_ context.Context, _ any, qs map[string]jev.Question) (jev.Result, error) {
	raw, _ := json.Marshal(map[string]any{
		"sem_progresso":         map[string]any{"type": "noul", "noul": s.noProgress},
		"bloqueio_de_permissao": map[string]any{"type": "noul", "noul": s.permBlock},
	})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 50}}, nil
}

// O teste completo de cancelamento (que sobe o fakedevin, deixa o watchdog
// matar o processo e confere cancel-reason.json) vive em internal/cli, onde
// o supervisor existe. Aqui se confere o laco de leitura incremental do
// stdout — a fonte viva, medida em 2026-09-20.
func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func noPaths() []string { return nil }

func TestLoopReadsGrowingStdoutAndAsksJev(t *testing.T) {
	logPath := t.TempDir() + "/stdout.log"

	cfg := DefaultConfig()
	cfg.PollInterval = 20 * time.Millisecond
	cfg.WindowBytes = 200

	seen := make(chan *Signal, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go loop(ctx, logPath, noPaths, stubAsker{noProgress: 0.95}, cfg, func(s *Signal) { seen <- s })

	// Duas janelas cheias: a politica exige duas consecutivas para sem_progresso.
	for i := 0; i < 4; i++ {
		appendTo(t, logPath, strings.Repeat("explorando de novo, nada novo. ", 10)+"\n")
		time.Sleep(40 * time.Millisecond)
	}

	select {
	case s := <-seen:
		if s.Name != "sem_progresso" {
			t.Errorf("sinal = %q, quero sem_progresso", s.Name)
		}
	case <-ctx.Done():
		t.Fatal("o watchdog nao detectou sem_progresso em duas janelas")
	}
}

// A recusa de ferramenta nao espera janela cheia nem segunda ocorrencia:
// ela e terminal, e esperar seria esperar por algo que nunca vem.
func TestLoopCancelsImmediatelyOnRejectionInStdout(t *testing.T) {
	logPath := t.TempDir() + "/stdout.log"

	cfg := DefaultConfig()
	cfg.PollInterval = 20 * time.Millisecond

	seen := make(chan *Signal, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Asker nil: este caminho nao pode depender de rede nem de chave.
	go loop(ctx, logPath, noPaths, nil, cfg, func(s *Signal) { seen <- s })

	appendTo(t, logPath, "Vou inspecionar o repositorio.\n")
	time.Sleep(40 * time.Millisecond)
	appendTo(t, logPath, "warning: rejected a tool call that requires confirmation. "+
		"Running in non-interactive mode.\n")

	select {
	case s := <-seen:
		if s.Name != "bloqueio_de_permissao" {
			t.Errorf("sinal = %q, quero bloqueio_de_permissao", s.Name)
		}
	case <-ctx.Done():
		t.Fatal("a recusa no stdout nao foi detectada")
	}
}
```

- [ ] **Step 6: Rodar e confirmar o vermelho**

Run: `go test ./internal/watchdog/ -run TestWatch -v`
Expected: FAIL — `undefined: loop`.

- [ ] **Step 7: Implementar o watchdog**

```go
// internal/watchdog/watchdog.go
package watchdog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
	"github.com/heliowap/devin-plugin-cc/internal/jev"
	"github.com/heliowap/devin-plugin-cc/internal/job"
)

// asker e o que o watchdog precisa de um cliente Jev.
type asker interface {
	Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error)
}

// loop observa o stdout do devin e chama onCancel quando a politica autoriza.
// Le incrementalmente: cada passada pega so o que chegou desde a anterior.
// Separado de Watch para ser testavel sem job, sem processo e sem rede.
//
// A fonte e stdout, nao o export: mediu-se em 2026-09-20 que --export so e
// escrito no encerramento, enquanto o stdout cresce durante o trabalho.
func loop(ctx context.Context, logPath string, changedPaths func() []string, a asker, cfg Config, onCancel func(*Signal)) {
	policy := NewPolicy(cfg)
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	var offset int64
	var pending string
	var prevWindow string
	lastGrowth := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		chunk, next, err := readSince(logPath, offset)
		if err != nil {
			continue // ainda nao criado
		}

		if next == offset {
			// Nada novo. O unico sinal possivel e estar parado tempo demais.
			if s := Stalled(lastGrowth, time.Now(), cfg.StallLimit); s.Fired {
				if fire := policy.Observe([]Signal{s}); fire != nil {
					onCancel(fire)
					return
				}
			}
			continue
		}
		offset = next
		lastGrowth = time.Now()
		pending += chunk

		// A recusa de ferramenta e terminal e barata de achar: nao espera
		// completar janela.
		if s := PermissionRejected(pending); s.Fired {
			if fire := policy.Observe([]Signal{s}); fire != nil {
				onCancel(fire)
				return
			}
		}

		if len(pending) < cfg.WindowBytes {
			continue // acumula ate ter uma janela que valha uma pergunta
		}
		window := pending
		pending = ""

		sigs := []Signal{OutOfScope(changedPaths(), cfg.AllowedPaths)}
		sigs = append(sigs, jevSignals(ctx, a, window, prevWindow)...)
		prevWindow = window

		if fire := policy.Observe(sigs); fire != nil {
			onCancel(fire)
			return
		}
	}
}

// readSince le o que foi acrescentado ao arquivo depois de offset.
func readSince(path string, offset int64) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", offset, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", offset, err
	}
	if info.Size() <= offset {
		return "", offset, nil
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", offset, err
	}
	buf := make([]byte, info.Size()-offset)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", offset, err
	}
	return string(buf[:n]), offset + int64(n), nil
}

// jevSignals pergunta ao Jev apenas o que exige leitura semantica. Hoje isso
// e uma pergunta so: bloqueio_de_permissao saiu daqui quando se mediu que o
// devin o anuncia numa string estavel.
func jevSignals(ctx context.Context, a asker, window, prev string) []Signal {
	if a == nil || window == "" {
		return nil
	}
	budget, err := jev.StateBudget(jev.WatchdogQuestions(), jev.DefaultLimits())
	if err != nil {
		return nil
	}
	state := map[string]any{
		"janela": map[string]any{
			"turno_atual":    fitTo(window, budget/2),
			"turnos_previos": fitTo(prev, budget/2),
		},
	}

	res, err := a.Ask(ctx, state, jev.WatchdogQuestions())
	if err != nil {
		return nil // falha de rede nao cancela job
	}
	p, ok := res.Answers.NoulOf("sem_progresso")
	if !ok {
		return nil
	}
	return []Signal{{
		Name: "sem_progresso", Fired: true, Probability: p,
		Excerpt: truncate(window, 400),
	}}
}

// fitTo corta pelo fim, preservando o inicio do texto.
func fitTo(s string, budgetTokens int) string {
	maxChars := budgetTokens * 4
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars]
}


// changedIn lista os arquivos alterados na worktree, via git. Deterministico
// e independente do formato da saida do devin.
func changedIn(dir string) []string {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if len(line) > 3 {
			paths = append(paths, strings.TrimSpace(line[3:]))
		}
	}
	return paths
}

// Watch observa o job e cancela o grupo de processos quando a politica manda.
func Watch(ctx context.Context, j *job.Job, a asker, cfg Config) {
	loop(ctx, j.Path("stdout.log"), func() []string { return changedIn(j.Worktree) }, a, cfg, func(s *Signal) {
		reason := job.CancelReason{
			Signal:      s.Name,
			Probability: s.Probability,
			TurnExcerpt: s.Excerpt,
			ResumeCommand: fmt.Sprintf(
				"cd %s && devin -c --model %s --permission-mode %s --respect-workspace-trust false -p \"Continue de onde parou: <o que falta>\"",
				j.Worktree, j.Model, j.Permission),
			At: time.Now(),
		}

		if raw, err := json.MarshalIndent(reason, "", "  "); err == nil {
			_ = os.WriteFile(j.Path("cancel-reason.json"), raw, 0o644)
		}

		j.CancelReason = &reason
		j.State = job.StateCancelled
		_ = j.Save()

		if j.PGID > 1 {
			_ = devin.KillGroup(j.PGID, 10*time.Second)
		}
	})
}
```

Ajuste a chamada em `internal/cli/supervise.go`, que na Task 10 passava só o job:

```go
	if !*noWatchdog {
		cfg := watchdog.DefaultConfig()
		var a gateAsker // nil quando nao ha chave: o watchdog fica so com os sinais deterministicos
		if key := os.Getenv("TYPESAFE_API_KEY"); key != "" {
			a = jev.New(jev.Options{APIKey: key, BaseURL: os.Getenv("TYPESAFE_BASE_URL")})
		}
		go watchdog.Watch(watchCtx, j, a, cfg)
	}
```

com `type gateAsker = interface {
	Ask(context.Context, any, map[string]jev.Question) (jev.Result, error)
}` declarado em `internal/cli/supervise.go`.

- [ ] **Step 8: Escrever o teste de cancelamento de ponta a ponta**

```go
// internal/cli/cancel_e2e_test.go
package cli

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/job"
	"github.com/heliowap/devin-plugin-cc/internal/testsupport"
)

// Este e o teste que prova a tese do plugin: run condenado morre sozinho,
// com motivo legivel, sem ninguem olhando.
func TestWatchdogCancelsBlockedRun(t *testing.T) {
	testsupport.InstallFakeDevin(t, "permission-block")
	t.Setenv("FAKEDEVIN_STEP_MS", "20")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TYPESAFE_API_KEY", "k")
	t.Setenv("TYPESAFE_BASE_URL", fakeJevServer(t, map[string]float64{
		"bloqueio_de_permissao": 0.97,
		"sem_progresso":         0.1,
	}))

	wt := t.TempDir()
	j, err := job.Create(wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.Path("briefing.md"), []byte("corrija"), 0o644); err != nil {
		t.Fatal(err)
	}
	j.Model, j.Permission = "swe-2-max", "smart"
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	if code := Run(context.Background(), []string{"task", "--job", j.ID}, &out, &errBuf); code != 0 {
		t.Fatalf("task: exit %d: %s", code, errBuf.String())
	}

	waitForJobState(t, j.ID, job.StateCancelled, 30*time.Second)

	got, err := job.Load(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CancelReason == nil {
		t.Fatal("cancelamento sem motivo registrado")
	}
	if got.CancelReason.Signal != "bloqueio_de_permissao" {
		t.Errorf("Signal = %q", got.CancelReason.Signal)
	}
	if got.CancelReason.TurnExcerpt == "" {
		t.Error("o motivo precisa carregar o trecho verbatim do turno")
	}
	if !bytes.Contains([]byte(got.CancelReason.ResumeCommand), []byte("devin -c")) {
		t.Errorf("ResumeCommand = %q, quero um comando de retomada colavel", got.CancelReason.ResumeCommand)
	}
	if _, err := os.Stat(j.Path("cancel-reason.json")); err != nil {
		t.Errorf("cancel-reason.json ausente: %v", err)
	}
}
```

- [ ] **Step 9: Rodar tudo e confirmar o verde**

Run: `go test ./... -v && go vet ./...`
Expected: PASS, inclusive `TestWatchdogCancelsBlockedRun`.

- [ ] **Step 10: Commit**

```bash
git add internal/watchdog internal/cli
git commit -m "feat: watchdog cancela run condenado com motivo verbatim"
```

---

### Task 13: Verificação em código

**Files:**
- Create: `internal/gitx/diff.go`
- Create: `internal/verify/verify.go`
- Test: `internal/gitx/diff_test.go`, `internal/verify/verify_test.go`

**Interfaces:**
- Consumes: `gitx` (Task 10).
- Produces:
  - `gitx.Diff(ctx, dir string) (string, error)`, `gitx.DiffStat(ctx, dir string) (added, removed, files int, err error)`.
  - `gitx.RevertNonTest(ctx, dir string, testGlobs []string) error` — desfaz, em `dir`, as mudanças de arquivos que não casam com `testGlobs`.
  - `verify.Step{Name, Command string; ExitCode int; Stdout string; Skipped bool}`.
  - `verify.Report{Diff string; Added, Removed, Files int; Steps []Step; MutationProved bool}` com `(Report) Green() bool` e `(Report) Step(name string) (Step, bool)`.
  - `verify.Run(ctx, dir string, cfg Config) (Report, error)` com `Config{TestCmd, SuiteCmd, LintCmd string; TestGlobs []string; Timeout time.Duration}`.

- [ ] **Step 1: Escrever o teste do teste de mutação**

Este é o teste mais importante da tarefa: ele prova que o plugin pega um teste que não prova nada.

```go
// internal/verify/verify_test.go
package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// repoWithFix monta um repo git de verdade com uma correcao e um teste.
// provesTheFix decide se o teste realmente depende da correcao.
func repoWithFix(t *testing.T, provesTheFix bool) string {
	t.Helper()
	dir := t.TempDir()

	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	write("go.mod", "module exemplo\n\ngo 1.27\n")
	write("soma.go", "package exemplo\n\nfunc Soma(a, b int) int { return a - b }\n") // defeito
	git("init", "-q")
	git("add", ".")
	git("commit", "-qm", "base")

	// A correcao.
	write("soma.go", "package exemplo\n\nfunc Soma(a, b int) int { return a + b }\n")

	if provesTheFix {
		write("soma_test.go", "package exemplo\n\nimport \"testing\"\n\nfunc TestSoma(t *testing.T) {\n\tif Soma(2, 3) != 5 {\n\t\tt.Fatal(\"quero 5\")\n\t}\n}\n")
	} else {
		// Teste que passa com ou sem a correcao: nao prova nada.
		write("soma_test.go", "package exemplo\n\nimport \"testing\"\n\nfunc TestSoma(t *testing.T) {\n\tif Soma(0, 0) != 0 {\n\t\tt.Fatal(\"quero 0\")\n\t}\n}\n")
	}
	return dir
}

func cfg() Config {
	return Config{
		TestCmd:   "go test ./...",
		TestGlobs: []string{"*_test.go"},
		Timeout:   60 * time.Second,
	}
}

func TestRunProvesMutationWhenTestDependsOnFix(t *testing.T) {
	rep, err := Run(context.Background(), repoWithFix(t, true), cfg())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.MutationProved {
		t.Error("o teste depende da correcao; a mutacao deveria ficar vermelha")
	}
	if !rep.Green() {
		t.Errorf("a suite deveria estar verde: %+v", rep.Steps)
	}
}

// Este e o caso que o protocolo manda pegar: teste que passa mesmo sem a
// correcao. Foi assim que se descobriu um teste que fixava a funcao errada.
func TestRunFlagsTestThatProvesNothing(t *testing.T) {
	rep, err := Run(context.Background(), repoWithFix(t, false), cfg())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.MutationProved {
		t.Error("o teste passa sem a correcao; a mutacao NAO deveria provar nada")
	}
}

func TestRunSkipsStepsWithoutCommand(t *testing.T) {
	c := cfg()
	c.LintCmd = ""
	rep, err := Run(context.Background(), repoWithFix(t, true), c)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, s := range rep.Steps {
		if s.Name == "lint" && !s.Skipped {
			t.Error("lint sem comando deveria ficar marcado como pulado, nao inventado")
		}
	}
}

func TestRunCapturesDiff(t *testing.T) {
	rep, err := Run(context.Background(), repoWithFix(t, true), cfg())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Files == 0 {
		t.Error("a verificacao precisa capturar o diff")
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/verify/ -v`
Expected: FAIL — `undefined: Run`.

- [ ] **Step 3: Implementar**

```go
// internal/gitx/diff.go
package gitx

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func output(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// Diff devolve o diff completo da worktree, incluindo arquivos novos.
func Diff(ctx context.Context, dir string) (string, error) {
	if _, err := output(ctx, dir, "add", "-AN"); err != nil {
		return "", fmt.Errorf("git add -AN: %w", err)
	}
	return output(ctx, dir, "diff")
}

// DiffStat resume o diff em numeros.
func DiffStat(ctx context.Context, dir string) (int, int, int, error) {
	if _, err := output(ctx, dir, "add", "-AN"); err != nil {
		return 0, 0, 0, err
	}
	out, err := output(ctx, dir, "diff", "--numstat")
	if err != nil {
		return 0, 0, 0, err
	}
	var added, removed, files int
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		a, _ := strconv.Atoi(f[0])
		r, _ := strconv.Atoi(f[1])
		added, removed, files = added+a, removed+r, files+1
	}
	return added, removed, files, nil
}

// RevertNonTest desfaz, em dir, as mudancas de arquivos que nao casam com
// testGlobs. Usado no teste de mutacao: o teste tem que ficar vermelho.
func RevertNonTest(ctx context.Context, dir string, testGlobs []string) error {
	out, err := output(ctx, dir, "diff", "--name-only")
	if err != nil {
		return err
	}
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if name == "" || isTestFile(name, testGlobs) {
			continue
		}
		if _, err := output(ctx, dir, "checkout", "--", name); err != nil {
			return fmt.Errorf("revertendo %s: %w", name, err)
		}
	}
	return nil
}

func isTestFile(name string, globs []string) bool {
	base := filepath.Base(name)
	for _, g := range globs {
		if ok, _ := filepath.Match(g, base); ok {
			return true
		}
	}
	return false
}
```

```go
// internal/verify/verify.go
package verify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/gitx"
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
	Name     string `json:"nome"`
	Command  string `json:"comando"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Skipped  bool   `json:"pulado"`
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

// Green indica que todo passo executado terminou com exit code zero.
func (r Report) Green() bool {
	for _, s := range r.Steps {
		if !s.Skipped && s.ExitCode != 0 {
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
	copyDir, err := os.MkdirTemp("", "devin-mutacao-*")
	if err != nil {
		return Step{}, err
	}
	defer os.RemoveAll(copyDir)

	target := filepath.Join(copyDir, "wt")
	if out, err := exec.CommandContext(ctx, "cp", "-R", dir, target).CombinedOutput(); err != nil {
		return Step{}, fmt.Errorf("copiando worktree: %w: %s", err, out)
	}
	if err := gitx.RevertNonTest(ctx, target, cfg.TestGlobs); err != nil {
		return Step{}, err
	}

	s := runCmd(ctx, target, "mutacao", cfg.TestCmd, cfg.Timeout)
	s.Command = cfg.TestCmd + "   (com a correcao desfeita)"
	return s, nil
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/verify/ ./internal/gitx/ -v && go vet ./...`
Expected: PASS. Em especial, `TestRunFlagsTestThatProvesNothing` precisa passar.

- [ ] **Step 5: Commit**

```bash
git add internal/gitx internal/verify
git commit -m "feat: verificacao em codigo com teste de mutacao"
```

---

### Task 14: Compactação da volta, coerência e `result`

**Files:**
- Create: `internal/compact/compact.go`
- Create: `internal/render/result.go`
- Create: `internal/cli/result.go`
- Test: `internal/compact/compact_test.go`, `internal/cli/result_test.go`

**Interfaces:**
- Consumes: `devin.Turn`, `jev.CompactionQuestions`, `jev.ReportQuestion`, `verify.Report`.
- Produces:
  - `compact.Decision{Keep, KeepOutput bool}` e `compact.Trace(ctx, a Asker, task string, turns []devin.Turn) ([]devin.Turn, jev.Usage, error)` — só deleta, nunca reescreve.
  - `compact.ClaimsGreen(ctx, a Asker, report string) (float64, jev.Usage, error)`.
  - `render.Result(w io.Writer, in render.Input)` com `Input{Job *job.Job; Verify verify.Report; Turns []devin.Turn; ClaimsGreen float64; JevUSD float64; Raw bool}`.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/compact/compact_test.go
package compact

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
	"github.com/heliowap/devin-plugin-cc/internal/jev"
)

// byID responde conforme o id da interacao embutido no state.
type byID struct{ keep map[string]bool }

func (b byID) Ask(_ context.Context, state any, _ map[string]jev.Question) (jev.Result, error) {
	m := state.(map[string]any)["interacao"].(map[string]any)
	id := m["id"].(string)
	v := 0.1
	if b.keep[id] {
		v = 0.95
	}
	raw, _ := json.Marshal(map[string]any{
		"chamada_necessaria":            map[string]any{"type": "noul", "noul": v},
		"resultado_necessario_verbatim": map[string]any{"type": "noul", "noul": v},
	})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 10}}, nil
}

func sample() []devin.Turn {
	return []devin.Turn{
		{Index: 0, Text: "explorando", Tools: []devin.ToolInteraction{{ID: "t0", Name: "exec", Input: "ls", Output: "total 4"}}},
		{Index: 1, Text: "vermelho", Tools: []devin.ToolInteraction{{ID: "t1", Name: "exec", Input: "go test ./x/", Output: "FAIL: TestSoma"}}},
		{Index: 2, Text: "corrigindo", Tools: []devin.ToolInteraction{{ID: "t2", Name: "edit", Input: "soma.go", Output: "1 hunk"}}},
	}
}

// Delecao, nunca reescrita: o que fica, fica palavra por palavra.
func TestTraceDeletesWithoutRewriting(t *testing.T) {
	kept, _, err := Trace(context.Background(), byID{keep: map[string]bool{"t1": true, "t2": true}}, "tarefa", sample())
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}

	var ids []string
	for _, turn := range kept {
		for _, tool := range turn.Tools {
			ids = append(ids, tool.ID)
			if tool.ID == "t1" && tool.Output != "FAIL: TestSoma" {
				t.Errorf("resultado foi reescrito: %q", tool.Output)
			}
		}
	}
	if len(ids) != 2 || ids[0] != "t1" || ids[1] != "t2" {
		t.Errorf("ids mantidos = %v, quero [t1 t2]", ids)
	}
}

// Turno cujo texto importa mas cujo resultado e volumoso: mantem a chamada,
// trunca o resultado. Tres desfechos, como no fast-jev-compaction.
func TestTraceKeepsCallAndTruncatesOutput(t *testing.T) {
	turns := []devin.Turn{{Index: 0, Tools: []devin.ToolInteraction{
		{ID: "t0", Name: "exec", Input: "go build ./...", Output: "linha\n" + longOutput()},
	}}}

	kept, _, err := Trace(context.Background(), splitAsker{}, "tarefa", turns)
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("quero 1 turno, tenho %d", len(kept))
	}
	out := kept[0].Tools[0].Output
	if len(out) >= len(longOutput()) {
		t.Error("o resultado deveria ter sido truncado")
	}
	if !contains(out, "(truncado") {
		t.Errorf("o truncamento precisa ser declarado, nao silencioso: %q", out)
	}
}

// splitAsker mantem a chamada e descarta o resultado verbatim.
type splitAsker struct{}

func (splitAsker) Ask(context.Context, any, map[string]jev.Question) (jev.Result, error) {
	raw, _ := json.Marshal(map[string]any{
		"chamada_necessaria":            map[string]any{"type": "noul", "noul": 0.9},
		"resultado_necessario_verbatim": map[string]any{"type": "noul", "noul": 0.1},
	})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 5}}, nil
}

func longOutput() string {
	s := ""
	for i := 0; i < 500; i++ {
		s += "ruido de build sem valor de conferencia\n"
	}
	return s
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/compact/ -v`
Expected: FAIL — `undefined: Trace`.

- [ ] **Step 3: Implementar**

```go
// internal/compact/compact.go
package compact

import (
	"context"
	"fmt"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
	"github.com/heliowap/devin-plugin-cc/internal/jev"
)

// Asker e o que a compactacao precisa de um cliente Jev.
type Asker interface {
	Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error)
}

// maxKeptOutput e o teto de um resultado mantido sem ser julgado necessario
// verbatim. Truncar e sempre declarado no texto.
const maxKeptOutput = 400

// Trace compacta o trace por delecao. Tres desfechos por interacao: mantem
// os dois, mantem a chamada com o resultado truncado, ou remove o par.
// Nada e reescrito — resumo perde caminho de arquivo, erro exato e comando.
func Trace(ctx context.Context, a Asker, task string, turns []devin.Turn) ([]devin.Turn, jev.Usage, error) {
	var total jev.Usage
	var out []devin.Turn

	for _, turn := range turns {
		var kept []devin.ToolInteraction

		for _, tool := range turn.Tools {
			state := map[string]any{
				"tarefa": map[string]any{"texto": task},
				"interacao": map[string]any{
					"id":        tool.ID,
					"chamada":   fmt.Sprintf("%s(%s)", tool.Name, tool.Input),
					"resultado": tool.Output,
					"status":    tool.Status,
				},
			}
			res, err := a.Ask(ctx, state, jev.CompactionQuestions())
			if err != nil {
				return nil, total, fmt.Errorf("compactacao: %w", err)
			}
			total.InputTokens += res.Usage.InputTokens

			keepCall, _ := res.Answers.NoulOf("chamada_necessaria")
			keepOut, _ := res.Answers.NoulOf("resultado_necessario_verbatim")

			if keepCall < 0.5 {
				continue // remove o par
			}
			if keepOut < 0.5 && len(tool.Output) > maxKeptOutput {
				tool.Output = tool.Output[:maxKeptOutput] +
					fmt.Sprintf("\n… (truncado: %d caracteres omitidos)", len(tool.Output)-maxKeptOutput)
			}
			kept = append(kept, tool)
		}

		if len(kept) > 0 || turn.Text != "" && len(turn.Tools) == 0 {
			turn.Tools = kept
			out = append(out, turn)
		}
	}
	return out, total, nil
}

// ClaimsGreen devolve a probabilidade de o relatorio afirmar que os testes
// passaram. O outro lado da comparacao e o exit code, que e fato.
func ClaimsGreen(ctx context.Context, a Asker, report string) (float64, jev.Usage, error) {
	res, err := a.Ask(ctx, map[string]any{"relatorio": map[string]any{"texto": report}}, jev.ReportQuestion())
	if err != nil {
		return 0, jev.Usage{}, fmt.Errorf("coerencia do relatorio: %w", err)
	}
	p, _ := res.Answers.NoulOf("relatorio_afirma_verde")
	return p, res.Usage, nil
}
```

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./internal/compact/ -v`
Expected: PASS.

- [ ] **Step 5: Escrever o teste do render**

O render é onde a flag de divergência aparece. Ela é o motivo de o plugin existir.

```go
// internal/cli/result_test.go
package cli

import (
	"bytes"
	"testing"

	"github.com/heliowap/devin-plugin-cc/internal/render"
	"github.com/heliowap/devin-plugin-cc/internal/verify"
)

func TestResultFlagsReportClaimingGreenOnRedSuite(t *testing.T) {
	var b bytes.Buffer
	render.Result(&b, render.Input{
		Verify: verify.Report{Steps: []verify.Step{
			{Name: "teste", Command: "go test ./...", ExitCode: 1, Stdout: "FAIL: TestSoma"},
		}},
		ClaimsGreen: 0.95,
	})

	out := b.String()
	if !bytes.Contains([]byte(out), []byte("DIVERGENCIA")) {
		t.Errorf("relatorio afirma verde e a suite esta vermelha; falta a flag:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("FAIL: TestSoma")) {
		t.Error("a saida real do teste precisa aparecer")
	}
}

func TestResultFlagsTestThatProvesNothing(t *testing.T) {
	var b bytes.Buffer
	render.Result(&b, render.Input{
		Verify: verify.Report{
			Steps:          []verify.Step{{Name: "teste", ExitCode: 0}},
			MutationProved: false,
		},
		ClaimsGreen: 0.9,
	})
	if !bytes.Contains(b.Bytes(), []byte("mutacao")) {
		t.Errorf("mutacao que nao provou nada precisa aparecer:\n%s", b.String())
	}
}

func TestResultNoFlagWhenConsistent(t *testing.T) {
	var b bytes.Buffer
	render.Result(&b, render.Input{
		Verify: verify.Report{
			Steps:          []verify.Step{{Name: "teste", ExitCode: 0}, {Name: "suite", ExitCode: 0}},
			MutationProved: true,
		},
		ClaimsGreen: 0.95,
	})
	if bytes.Contains(b.Bytes(), []byte("DIVERGENCIA")) {
		t.Errorf("sem divergencia, nao deveria haver flag:\n%s", b.String())
	}
}
```

- [ ] **Step 6: Rodar e confirmar o vermelho**

Run: `go test ./internal/cli/ -run TestResult -v`
Expected: FAIL — pacote `render` não existe.

- [ ] **Step 7: Implementar o render e o subcomando**

```go
// internal/render/result.go
package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
	"github.com/heliowap/devin-plugin-cc/internal/job"
	"github.com/heliowap/devin-plugin-cc/internal/verify"
)

// Input e o que o relatorio final precisa saber.
type Input struct {
	Job         *job.Job
	Verify      verify.Report
	Turns       []devin.Turn
	ClaimsGreen float64
	JevUSD      float64
	Raw         bool
}

// Result escreve o relatorio: veredito verificado primeiro, trace depois.
func Result(w io.Writer, in Input) {
	if in.Job != nil {
		fmt.Fprintf(w, "## Job: %s (%s)\n\n", in.Job.ID, in.Job.State)
		if r := in.Job.CancelReason; r != nil {
			fmt.Fprintf(w, "### CANCELADO PELO WATCHDOG\n\n")
			fmt.Fprintf(w, "Sinal: **%s** (%.2f)\n\n", r.Signal, r.Probability)
			fmt.Fprintf(w, "Trecho que provocou:\n\n```\n%s\n```\n\n", r.TurnExcerpt)
			fmt.Fprintf(w, "Para retomar:\n\n```bash\n%s\n```\n\n", r.ResumeCommand)
		}
	}

	writeDivergences(w, in)

	fmt.Fprintf(w, "### Veredito verificado\n\n")
	fmt.Fprintf(w, "Diff: %d arquivos, +%d/-%d\n\n", in.Verify.Files, in.Verify.Added, in.Verify.Removed)
	for _, s := range in.Verify.Steps {
		if s.Skipped {
			fmt.Fprintf(w, "- %s: pulado (sem comando declarado)\n", s.Name)
			continue
		}
		status := "verde"
		if s.ExitCode != 0 {
			status = fmt.Sprintf("vermelho (exit %d)", s.ExitCode)
		}
		fmt.Fprintf(w, "- %s: %s — `%s`\n", s.Name, status, s.Command)
	}
	fmt.Fprintln(w)

	for _, s := range in.Verify.Steps {
		if !s.Skipped && s.ExitCode != 0 && s.Stdout != "" {
			fmt.Fprintf(w, "Saida de `%s`:\n\n```\n%s\n```\n\n", s.Name, strings.TrimSpace(s.Stdout))
		}
	}

	fmt.Fprintf(w, "### Output\n\n")
	if len(in.Turns) == 0 {
		fmt.Fprintln(w, "(sem turnos registrados)")
	}
	for _, t := range in.Turns {
		fmt.Fprintf(w, "```\n%s```\n", t.Summary())
	}

	if in.JevUSD > 0 {
		fmt.Fprintf(w, "\n---\nJev: $%.5f\n", in.JevUSD)
	}
}

func writeDivergences(w io.Writer, in Input) {
	var notes []string

	if in.ClaimsGreen >= 0.7 && !in.Verify.Green() {
		notes = append(notes, "O relatorio afirma que os testes passaram, mas ao menos um comando "+
			"verificado aqui terminou com erro. Leia a saida abaixo antes de aceitar qualquer coisa.")
	}
	if teste, ok := in.Verify.Step("teste"); ok && !teste.Skipped && teste.ExitCode == 0 && !in.Verify.MutationProved {
		notes = append(notes, "O teste passa **tambem com a correcao desfeita** (mutacao). "+
			"Ele nao prova a correcao: verifique se fixa o comportamento certo.")
	}
	if len(notes) == 0 {
		return
	}
	fmt.Fprintf(w, "### DIVERGENCIA\n\n")
	for _, n := range notes {
		fmt.Fprintf(w, "- %s\n", n)
	}
	fmt.Fprintln(w)
}
```

```go
// internal/cli/result.go
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/heliowap/devin-plugin-cc/internal/compact"
	"github.com/heliowap/devin-plugin-cc/internal/devin"
	"github.com/heliowap/devin-plugin-cc/internal/jev"
	"github.com/heliowap/devin-plugin-cc/internal/job"
	"github.com/heliowap/devin-plugin-cc/internal/render"
	"github.com/heliowap/devin-plugin-cc/internal/verify"
)

func runResult(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("result", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		raw      = fs.Bool("raw", false, "mostra o trace inteiro, sem compactar")
		testCmd  = fs.String("test-cmd", "", "comando de teste")
		suiteCmd = fs.String("suite-cmd", "", "comando da suite do pacote tocado")
		lintCmd  = fs.String("lint-cmd", "", "comando de lint")
	)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "uso: result <job-id> [flags]")
		return ExitUsage
	}

	j, err := job.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "result: %v\n", err)
		return 1
	}

	vrep, err := verify.Run(ctx, j.Worktree, verify.Config{
		TestCmd: *testCmd, SuiteCmd: *suiteCmd, LintCmd: *lintCmd,
		TestGlobs: []string{"*_test.go", "test_*.py", "*.test.ts", "*.spec.ts"},
	})
	if err != nil {
		fmt.Fprintf(stderr, "result: verificacao: %v\n", err)
	}

	exportRaw, _ := os.ReadFile(j.Path("export.json"))
	turns, _ := devin.ParseATIF(exportRaw)

	in := render.Input{Job: j, Verify: vrep, Turns: turns, Raw: *raw}

	if key := os.Getenv("TYPESAFE_API_KEY"); key != "" && !*raw {
		client := jev.New(jev.Options{APIKey: key, BaseURL: os.Getenv("TYPESAFE_BASE_URL")})
		ledger := &jev.Ledger{Path: j.Path("jev.jsonl")}

		if kept, usage, err := compact.Trace(ctx, client, "", turns); err == nil {
			in.Turns = kept
			_ = ledger.Record("compactacao", usage)
		}
		stdoutLog, _ := os.ReadFile(j.Path("stdout.log"))
		if p, usage, err := compact.ClaimsGreen(ctx, client, string(stdoutLog)); err == nil {
			in.ClaimsGreen = p
			_ = ledger.Record("coerencia", usage)
		}
		_, usd, _ := ledger.Total()
		in.JevUSD = usd
	}

	render.Result(stdout, in)
	return 0
}
```

Registre `result` em `handlers()`.

- [ ] **Step 8: Rodar e confirmar o verde**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/compact internal/render internal/cli
git commit -m "feat: compactacao por delecao, flag de divergencia e result"
```

---

### Task 15: `status`, `wait-and-result` e `cancel`

**Files:**
- Create: `internal/cli/status.go`, `internal/cli/waitresult.go`, `internal/cli/cancel.go`
- Test: `internal/cli/status_test.go`

**Interfaces:**
- Consumes: `job`, `render`, `devin.KillGroup`.
- Produces: subcomandos `status <id> [--json]`, `wait-and-result <id> [--max-wait N]`, `cancel <id>`. Códigos de `wait-and-result`: `0` pronto, `2` ainda rodando, `1` erro.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/cli/status_test.go
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/job"
)

func TestStatusJSONReportsState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	j, err := job.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	j.State = job.StateRunning
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	if code := Run(context.Background(), []string{"status", j.ID, "--json"}, &out, &errBuf); code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	var got struct {
		State string `json:"estado"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout nao e JSON: %v\n%s", err, out.String())
	}
	if got.State != "running" {
		t.Errorf("estado = %q", got.State)
	}
}

// Job cancelado precisa mostrar o motivo no topo, nao escondido num arquivo.
func TestStatusShowsCancelReasonProminently(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	j, err := job.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	j.State = job.StateCancelled
	j.CancelReason = &job.CancelReason{
		Signal: "bloqueio_de_permissao", Probability: 0.97,
		TurnExcerpt: "aguardando confirmacao", ResumeCommand: "devin -c ...", At: time.Now(),
	}
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	Run(context.Background(), []string{"status", j.ID}, &out, &errBuf)

	for _, want := range []string{"bloqueio_de_permissao", "aguardando confirmacao", "devin -c"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Errorf("status nao mostra %q:\n%s", want, out.String())
		}
	}
}

// Timeout nao e erro: e "ainda rodando", e o chamador repete.
func TestWaitAndResultExits2WhileRunning(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	j, err := job.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	j.State = job.StateRunning
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{"wait-and-result", j.ID, "--max-wait", "1"}, &out, &errBuf)
	if code != 2 {
		t.Errorf("exit = %d, quero 2 (ainda rodando)", code)
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/cli/ -run "TestStatus|TestWaitAndResult" -v`
Expected: FAIL — subcomandos não registrados.

- [ ] **Step 3: Implementar**

```go
// internal/cli/status.go
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/heliowap/devin-plugin-cc/internal/jev"
	"github.com/heliowap/devin-plugin-cc/internal/job"
)

type statusOut struct {
	ID       string            `json:"id"`
	State    string            `json:"estado"`
	Worktree string            `json:"worktree"`
	Model    string            `json:"modelo"`
	JevUSD   float64           `json:"custo_jev_usd"`
	Cancel   *job.CancelReason `json:"cancelamento,omitempty"`
}

func runStatus(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "saida em JSON")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "uso: status <job-id> [--json]")
		return ExitUsage
	}
	j, err := job.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "status: %v\n", err)
		return 1
	}
	_, usd, _ := (&jev.Ledger{Path: j.Path("jev.jsonl")}).Total()

	out := statusOut{ID: j.ID, State: string(j.State), Worktree: j.Worktree,
		Model: j.Model, JevUSD: usd, Cancel: j.CancelReason}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	// O aviso do cancelamento vem primeiro: quem le precisa ver isso antes
	// de qualquer outra coisa.
	if r := j.CancelReason; r != nil {
		fmt.Fprintf(stdout, "CANCELADO PELO WATCHDOG — sinal %s (%.2f)\n\n", r.Signal, r.Probability)
		fmt.Fprintf(stdout, "O que provocou:\n%s\n\n", r.TurnExcerpt)
		fmt.Fprintf(stdout, "Para retomar:\n%s\n\n", r.ResumeCommand)
	}
	fmt.Fprintf(stdout, "job:      %s\nestado:   %s\nworktree: %s\nmodelo:   %s\njev:      $%.5f\n",
		out.ID, out.State, out.Worktree, out.Model, out.JevUSD)
	return 0
}
```

```go
// internal/cli/waitresult.go
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/job"
)

// ExitStillRunning sinaliza que o job nao terminou: o chamador repete.
// Existe porque o Bash do Claude corta em 10 minutos e run de Devin nao
// respeita esse limite.
const ExitStillRunning = 2

func runWaitAndResult(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("wait-and-result", flag.ContinueOnError)
	fs.SetOutput(stderr)
	maxWait := fs.Int("max-wait", 480, "segundos de espera nesta chamada")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "uso: wait-and-result <job-id> [--max-wait N]")
		return ExitUsage
	}
	id := fs.Arg(0)

	deadline := time.Now().Add(time.Duration(*maxWait) * time.Second)
	for {
		j, err := job.Load(id)
		if err != nil {
			fmt.Fprintf(stderr, "wait-and-result: %v\n", err)
			return 1
		}
		if j.State.Terminal() {
			return runResult(ctx, []string{id}, stdout, stderr)
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(stderr, "ainda rodando (%s); chame de novo\n", j.State)
			return ExitStillRunning
		}
		select {
		case <-ctx.Done():
			return 1
		case <-time.After(3 * time.Second):
		}
	}
}
```

```go
// internal/cli/cancel.go
package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/heliowap/devin-plugin-cc/internal/devin"
	"github.com/heliowap/devin-plugin-cc/internal/job"
)

func runCancel(_ context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "uso: cancel <job-id>")
		return ExitUsage
	}
	j, err := job.Load(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "cancel: %v\n", err)
		return 1
	}
	if j.State.Terminal() {
		fmt.Fprintf(stdout, "job %s ja esta %s\n", j.ID, j.State)
		return 0
	}
	if j.PGID > 1 {
		if err := devin.KillGroup(j.PGID, 10*time.Second); err != nil {
			fmt.Fprintf(stderr, "cancel: %v\n", err)
		}
	}
	j.State = job.StateCancelled
	j.CancelReason = &job.CancelReason{
		Signal: "cancelamento_manual", Probability: 1,
		TurnExcerpt:   "cancelado pelo usuario",
		ResumeCommand: fmt.Sprintf("cd %s && devin -c --respect-workspace-trust false -p \"Continue\"", j.Worktree),
		At:            time.Now(),
	}
	if err := j.Save(); err != nil {
		fmt.Fprintf(stderr, "cancel: %v\n", err)
		return 1
	}
	_ = job.Release(j.ID)
	fmt.Fprintf(stdout, "job %s cancelado\n", j.ID)
	return 0
}
```

Registre `status`, `wait-and-result` e `cancel` em `handlers()`.

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli
git commit -m "feat: status, wait-and-result e cancel"
```

---

### Task 16: Modo revisor e triagem de achados

**Files:**
- Create: `internal/triage/triage.go`, `internal/cli/review.go`
- Test: `internal/triage/triage_test.go`

**Interfaces:**
- Consumes: `jev.FindingQuestions`, `gate.Asker`.
- Produces:
  - `triage.Finding{Text string; HasFileLine bool; Scenario float64; Severity float64}`
  - `triage.SplitFindings(report string) []string` — separa por cabeçalho ou bullet.
  - `triage.Rank(ctx, a Asker, findings []string) ([]Finding, jev.Usage, error)` — ordena por severidade, marca a camada de estilo.
  - `(Finding) Actionable() bool` — tem `arquivo:linha` **e** cenário reproduzível.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/triage/triage_test.go
package triage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliowap/devin-plugin-cc/internal/jev"
)

type scripted struct{ scenario, severity map[string]float64 }

func (s scripted) Ask(_ context.Context, state any, _ map[string]jev.Question) (jev.Result, error) {
	text := state.(map[string]any)["achado"].(map[string]any)["texto"].(string)
	key := text[:min(12, len(text))]
	raw, _ := json.Marshal(map[string]any{
		"tem_cenario_reproduzivel": map[string]any{"type": "noul", "noul": s.scenario[key]},
		"severidade":               map[string]any{"type": "score", "score": s.severity[key], "confidence": 0.8},
	})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 20}}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSplitFindingsSeparatesBullets(t *testing.T) {
	got := SplitFindings("- achado um\n- achado dois\n- achado tres\n")
	if len(got) != 3 {
		t.Fatalf("len = %d, quero 3: %v", len(got), got)
	}
}

// Achado sem arquivo:linha e sem cenario e opiniao de estilo. Descartar isso
// antes do modelo caro e o ponto: 6 de ~30 alegacoes nao se sustentaram.
func TestRankMarksStyleOpinionAsNotActionable(t *testing.T) {
	findings := []string{
		"pkg/svc/rota.go:42 devolve 200 com token expirado quando o cache esta quente",
		"este nome de variavel podia ser melhor",
	}
	a := scripted{
		scenario: map[string]float64{"pkg/svc/rot": 0.95, "este nome d": 0.05},
		severity: map[string]float64{"pkg/svc/rot": 2.7, "este nome d": 0.1},
	}

	got, _, err := Rank(context.Background(), a, findings)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if !got[0].Actionable() {
		t.Error("o achado com arquivo:linha e cenario deveria ser acionavel")
	}
	if got[1].Actionable() {
		t.Error("opiniao de estilo nao e acionavel")
	}
	if !strings.Contains(got[0].Text, "rota.go:42") {
		t.Error("o mais severo deveria vir primeiro")
	}
}

func TestFindingDetectsFileLine(t *testing.T) {
	cases := map[string]bool{
		"pkg/svc/rota.go:42 esta errado":        true,
		"src/app/main.ts:7: falta await":        true,
		"a funcao Rota esta errada":             false,
		"veja o arquivo rota.go":                false,
	}
	for text, want := range cases {
		if got := hasFileLine(text); got != want {
			t.Errorf("hasFileLine(%q) = %v, quero %v", text, got, want)
		}
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/triage/ -v`
Expected: FAIL — `undefined: SplitFindings`.

- [ ] **Step 3: Implementar**

```go
// internal/triage/triage.go
package triage

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/heliowap/devin-plugin-cc/internal/jev"
)

// Asker e o que a triagem precisa de um cliente Jev.
type Asker interface {
	Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error)
}

// Finding e um achado de revisao ja triado.
type Finding struct {
	Text        string  `json:"texto"`
	HasFileLine bool    `json:"tem_arquivo_linha"`
	Scenario    float64 `json:"cenario_reproduzivel"`
	Severity    float64 `json:"severidade"`
}

// Actionable indica achado que vale o tempo de um humano: localizado e com
// cenario. Sem cenario, o que chega e opiniao de estilo.
func (f Finding) Actionable() bool { return f.HasFileLine && f.Scenario >= 0.6 }

// fileLineRe casa caminho/arquivo.ext:linha. Isto e regex, nao modelo:
// presenca de localizador se confere contando, nao julgando.
var fileLineRe = regexp.MustCompile(`[\w./\-]+\.\w+:\d+`)

func hasFileLine(s string) bool { return fileLineRe.MatchString(s) }

var bulletRe = regexp.MustCompile(`(?m)^\s*(?:[-*]|\d+\.)\s+`)

// SplitFindings separa o relatorio de revisao em achados.
func SplitFindings(report string) []string {
	idx := bulletRe.FindAllStringIndex(report, -1)
	if len(idx) == 0 {
		var out []string
		for _, p := range strings.Split(report, "\n\n") {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	var out []string
	for i, m := range idx {
		end := len(report)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		if s := strings.TrimSpace(report[m[1]:end]); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Rank tria os achados e ordena do mais severo ao menos. O que nao e
// acionavel fica no fim, marcado, em vez de ser apagado: descartar em
// silencio tiraria do autor a chance de discordar.
func Rank(ctx context.Context, a Asker, findings []string) ([]Finding, jev.Usage, error) {
	var total jev.Usage
	out := make([]Finding, 0, len(findings))

	for _, text := range findings {
		state := map[string]any{"achado": map[string]any{"texto": text}}
		res, err := a.Ask(ctx, state, jev.FindingQuestions())
		if err != nil {
			return nil, total, fmt.Errorf("triagem: %w", err)
		}
		total.InputTokens += res.Usage.InputTokens

		f := Finding{Text: text, HasFileLine: hasFileLine(text)}
		f.Scenario, _ = res.Answers.NoulOf("tem_cenario_reproduzivel")
		if sc, ok := res.Answers.ScoreOf("severidade"); ok {
			f.Severity = sc.Score
		}
		out = append(out, f)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Actionable() != out[j].Actionable() {
			return out[i].Actionable()
		}
		return out[i].Severity > out[j].Severity
	})
	return out, total, nil
}
```

```go
// internal/cli/review.go
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/heliowap/devin-plugin-cc/internal/jev"
	"github.com/heliowap/devin-plugin-cc/internal/triage"
)

func runReview(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	reportPath := fs.String("report", "", "arquivo com o relatorio de revisao a triar")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *reportPath == "" {
		fmt.Fprintln(stderr, "review: --report e obrigatorio")
		return ExitUsage
	}
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		fmt.Fprintln(stderr, "review: TYPESAFE_API_KEY ausente; sem ela nao ha triagem")
		return 1
	}
	raw, err := os.ReadFile(*reportPath)
	if err != nil {
		fmt.Fprintf(stderr, "review: %v\n", err)
		return 1
	}

	client := jev.New(jev.Options{APIKey: key, BaseURL: os.Getenv("TYPESAFE_BASE_URL")})
	findings, usage, err := triage.Rank(ctx, client, triage.SplitFindings(string(raw)))
	if err != nil {
		fmt.Fprintf(stderr, "review: %v\n", err)
		return 1
	}

	var acionaveis, estilo int
	fmt.Fprintf(stdout, "### Achados acionaveis\n\n")
	for _, f := range findings {
		if !f.Actionable() {
			estilo++
			continue
		}
		acionaveis++
		fmt.Fprintf(stdout, "- [severidade %.1f] %s\n", f.Severity, f.Text)
	}
	if estilo > 0 {
		fmt.Fprintf(stdout, "\n### Sem localizador ou sem cenario (%d)\n\n", estilo)
		for _, f := range findings {
			if !f.Actionable() {
				fmt.Fprintf(stdout, "- %s\n", f.Text)
			}
		}
	}
	fmt.Fprintf(stdout, "\n---\n%d acionaveis de %d achados. Jev: ~$%.5f\n",
		acionaveis, len(findings), float64(usage.InputTokens)/1_000_000*jev.InputUSDPerMillion)
	return 0
}
```

Registre `review` em `handlers()`.

- [ ] **Step 4: Rodar e confirmar o verde**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/triage internal/cli/review.go
git commit -m "feat: triagem de achados de revisao"
```

---

### Task 17: Superfície do plugin

Nenhuma lógica em Markdown. Estes arquivos só roteiam para o binário.

**Files:**
- Create: `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`
- Create: `commands/{rescue,review,status,result,cancel,setup}.md`
- Create: `agents/devin-rescue.md`
- Create: `skills/devin-runtime/SKILL.md`, `skills/devin-briefing/SKILL.md`, `skills/devin-verification/SKILL.md`
- Create: `hooks/hooks.json`, `scripts/cancel-notice-hook.sh`
- Test: `internal/pluginfiles/pluginfiles_test.go`

**Interfaces:**
- Consumes: os subcomandos das Tasks 1 a 16.
- Produces: a superfície instalável.

- [ ] **Step 1: Escrever o teste de conformidade da superfície**

O plugin quebra em silêncio quando um Markdown referencia um subcomando que não existe. O teste fecha isso.

```go
// internal/pluginfiles/pluginfiles_test.go
package pluginfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func root(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
}

func TestPluginManifestIsValid(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(root(t), ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Name, Version, Description string }
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "devin" || m.Version == "" || m.Description == "" {
		t.Errorf("manifest incompleto: %+v", m)
	}
}

func TestEveryMarkdownHasFrontmatter(t *testing.T) {
	for _, dir := range []string{"commands", "agents"} {
		entries, err := os.ReadDir(filepath.Join(root(t), dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			raw, err := os.ReadFile(filepath.Join(root(t), dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(raw), "---\n") {
				t.Errorf("%s/%s nao comeca com frontmatter", dir, e.Name())
			}
		}
	}
}

// Markdown que chama um subcomando inexistente quebra em silencio no uso.
var cmdRe = regexp.MustCompile(`companion"?\s+([a-z][a-z-]+)`)

func TestMarkdownOnlyReferencesRealSubcommands(t *testing.T) {
	known := map[string]bool{
		"doctor": true, "plan": true, "task": true, "supervise": true,
		"status": true, "result": true, "wait-and-result": true,
		"cancel": true, "review": true,
	}
	for _, dir := range []string{"commands", "agents", "skills"} {
		base := filepath.Join(root(t), dir)
		_ = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			raw, _ := os.ReadFile(path)
			for _, m := range cmdRe.FindAllStringSubmatch(string(raw), -1) {
				if !known[m[1]] {
					t.Errorf("%s referencia subcomando inexistente: %q", path, m[1])
				}
			}
			return nil
		})
	}
}

// Regra global: dangerous nunca e sugerido pela superficie do plugin.
func TestNoMarkdownSuggestsDangerous(t *testing.T) {
	for _, dir := range []string{"commands", "agents", "skills"} {
		_ = filepath.Walk(filepath.Join(root(t), dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			raw, _ := os.ReadFile(path)
			if strings.Contains(string(raw), "--permission-mode dangerous") {
				t.Errorf("%s sugere dangerous", path)
			}
			return nil
		})
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/pluginfiles/ -v`
Expected: FAIL — arquivos ausentes.

- [ ] **Step 3: Criar os manifestos**

```bash
mkdir -p .claude-plugin commands agents hooks skills/devin-runtime skills/devin-briefing skills/devin-verification

cat > .claude-plugin/plugin.json <<'EOF'
{
  "name": "devin",
  "version": "0.1.0",
  "description": "Usa o Devin CLI como subagente, com Jev como classificador barato nos gates, no watchdog e na compactacao.",
  "author": { "name": "Helio Pinheiro" }
}
EOF

cat > .claude-plugin/marketplace.json <<'EOF'
{
  "name": "heliowap-devin",
  "owner": { "name": "Helio Pinheiro" },
  "plugins": [
    { "name": "devin", "source": "./", "description": "Devin como subagente, com gates e watchdog por Jev." }
  ]
}
EOF
```

- [ ] **Step 4: Criar os comandos**

```bash
cat > commands/rescue.md <<'EOF'
---
description: Delega uma correcao ao Devin CLI, com gate de delegabilidade, watchdog e verificacao
argument-hint: "[--wait|--background] <o defeito em uma frase>"
allowed-tools: Bash(*/scripts/companion:*)
---

Roteie este pedido para o subagente `devin:devin-rescue`.
A resposta final ao usuario e a saida do companion, sem parafrase.

Pedido bruto:
$ARGUMENTS

Antes de rotear, monte o JSONL de evidencias a partir do que voce **ja leu
nesta sessao**: trechos com `arquivo:linha`, a mensagem de erro exata, os
comandos que voce rodou e a saida deles, e a citacao do ADR ou spec que
define o contrato violado. Um objeto por linha:

```
{"kind":"trecho","ref":"pkg/svc/rota.go:42","text":"<o codigo, verbatim>"}
{"kind":"erro","text":"<a mensagem exata>"}
{"kind":"comando","text":"<comando e saida>"}
{"kind":"fonte","ref":"docs/adr/0007.md","text":"<o que a fonte diz>"}
```

Nao resuma nada nesse arquivo: o companion decide o que entra, e resumo
perde justamente o caminho e o erro que importam. Nao invente evidencia que
voce nao leu.

Se o gate reprovar (exit 3), o companion diz qual item faltou. Corrija o que
faltou e rode de novo; nao tente contornar o gate.
EOF

cat > commands/review.md <<'EOF'
---
description: Tria um relatorio de revisao, separando achado acionavel de opiniao de estilo
argument-hint: "<caminho do relatorio de revisao>"
allowed-tools: Bash(*/scripts/companion:*)
---

Rode a triagem sobre o relatorio indicado:

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/companion" review --report $ARGUMENTS
```

Devolva a saida como esta. Achado que aparecer na secao "sem localizador ou
sem cenario" nao deve ser repassado adiante como defeito: revisao de agente
rende falso positivo, e confirmar cada um custa mais do que descarta-los.
EOF

cat > commands/status.md <<'EOF'
---
description: Mostra o estado de um job do Devin, incluindo motivo de cancelamento
argument-hint: "<job-id>"
allowed-tools: Bash(*/scripts/companion:*)
---

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/companion" status $ARGUMENTS
```

Se o job aparecer como cancelado, leia o sinal e o trecho para o usuario e
ofereca o comando de retomada que vem na saida. Nao retome por conta propria.
EOF

cat > commands/result.md <<'EOF'
---
description: Verifica o trabalho do Devin e mostra o veredito com o trace compactado
argument-hint: "<job-id> [--test-cmd ...] [--suite-cmd ...] [--lint-cmd ...] [--raw]"
allowed-tools: Bash(*/scripts/companion:*)
---

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/companion" result $ARGUMENTS
```

Devolva a saida como esta. Se houver um bloco DIVERGENCIA, leia-o em voz alta
para o usuario antes de qualquer resumo: ele indica que o relatorio do Devin
nao bate com o que foi verificado aqui, ou que o teste nao prova a correcao.
EOF

cat > commands/cancel.md <<'EOF'
---
description: Cancela um job do Devin em andamento
argument-hint: "<job-id>"
allowed-tools: Bash(*/scripts/companion:*)
---

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/companion" cancel $ARGUMENTS
```
EOF

cat > commands/setup.md <<'EOF'
---
description: Confere o ambiente local do plugin (Go, devin CLI, chave do TypeSafe)
allowed-tools: Bash(*/scripts/companion:*)
---

```bash
"${CLAUDE_PLUGIN_ROOT}/scripts/companion" doctor
```

O wrapper compila o companion na primeira execucao. Se faltar Go, ele diz
isso. Se faltar `TYPESAFE_API_KEY`, os gates e o watchdog ficam desligados —
avise o usuario, porque despachar sem gate e o que este plugin evita.
EOF
```

- [ ] **Step 5: Criar o subagente e as skills**

```bash
cat > agents/devin-rescue.md <<'EOF'
---
name: devin-rescue
description: Use quando o Claude deve entregar uma correcao localizada ao Devin CLI, com gate, watchdog e verificacao pelo companion
tools: Bash
skills:
  - devin-runtime
  - devin-briefing
---

Voce e um encaminhador fino para o companion do Devin. Nao resolva a tarefa
voce mesmo, nao leia o repositorio, nao rode git por fora do companion.

Sequencia, sempre esta:

1. `plan` — com `--task`, `--evidence`, `--worktree` e os comandos de teste e
   lint. Exit 3 significa reprovado: devolva ao chamador o JSON com o campo
   `faltando`, sem tentar contornar.
2. `task --job <id>` — devolve o job id na hora.
3. `wait-and-result <id> --max-wait 480` em laco, ate 20 vezes. Exit 0 devolve
   o relatorio; exit 2 significa que ainda roda, repita; exit 1 e erro.
4. Devolva a saida do passo 3 exatamente como veio.

Nunca devolva "monitorando", "aguardando" ou "tarefa enviada" como resposta
final: isso e falha de encaminhamento, nao resultado. Se o laco quebrar,
responda numa linha: `ERRO: falha no encaminhamento (<motivo>)`.

Nunca passe `--permission-mode dangerous`. Se o usuario pedir, diga que o
plugin nao faz isso e explique que com ele o Devin poderia commitar ou fazer
push.
EOF

cat > skills/devin-runtime/SKILL.md <<'EOF'
---
name: devin-runtime
description: Contrato interno para chamar o companion do Devin a partir do Claude Code
user-invocable: false
---

# Runtime do companion

Use apenas dentro do subagente `devin:devin-rescue`.

O binario e sempre chamado pelo wrapper:
`"${CLAUDE_PLUGIN_ROOT}/scripts/companion" <subcomando>`

Subcomandos: `doctor`, `plan`, `task`, `status`, `result`,
`wait-and-result`, `cancel`, `review`.

Codigos de saida que mudam o seu comportamento:

- `plan` exit 3 — gate reprovou. O JSON traz `faltando`. Corrija e repita.
- `wait-and-result` exit 2 — ainda rodando. Chame de novo. Nao e erro.
- Qualquer exit 1 — erro real. Devolva o motivo.

Nunca use `tail -f` no log em vez de `wait-and-result`: o Bash do Claude
corta em 10 minutos e o run pode passar disso.

Nunca chame `supervise` diretamente. Quem o dispara e o `task`.
EOF

cat > skills/devin-briefing/SKILL.md <<'EOF'
---
name: devin-briefing
description: Como reunir a evidencia que vira briefing do Devin, e o que nunca delegar
---

# Evidencia para o briefing

Voce nao escreve o briefing: voce entrega o defeito em uma frase e a
evidencia verbatim. O companion seleciona e monta.

Reuna, verbatim, da sua propria sessao:

1. **Trecho com `arquivo:linha`** — onde esta o defeito. Sem isto, o gate
   reprova, porque o Devin segue a fonte quando voce a nomeia e improvisa
   quando nao nomeia.
2. **Fonte do contrato** — o ADR, spec, issue ou comentario normativo que
   diz qual e o comportamento certo, com o texto do que ele diz.
3. **Erro observado** — a mensagem exata, nao a sua parafrase dela.
4. **Comandos e saidas** — o que voce rodou e o que apareceu.

Passe tambem os comandos de teste, de suite e de lint, copiaveis, e os
caminhos de ambiente (`--venv`, `--node-modules`): worktree nova nao tem
ambiente, e sem saber rodar teste o Devin inventa ou nao roda.

Nao delegue: decisao de desenho em aberto, mudanca que cruza pacotes,
qualquer coisa que toque credencial, segredo, dado pessoal ou deploy, nem
tres temas diferentes no mesmo pedido. Dois defeitos que sao a mesma causa
funcionam; tres assuntos, nao.

Quando o trabalho for do Devin, diga isso no relatorio ou no commit, e diga
o que voce conferiu. "Implementado pelo Devin CLI; teste, mutacao e suite
conferidos nesta sessao" e uma linha honesta e barata.
EOF

cat > skills/devin-verification/SKILL.md <<'EOF'
---
name: devin-verification
description: O que o companion verifica sozinho e o que continua sendo seu
---

# Verificacao

O `result` executa e captura, sem modelo nenhum no caminho: o diff, o comando
de teste, o **teste de mutacao** (desfaz a correcao numa copia descartavel e
exige que o teste fique vermelho), a suite do pacote tocado e o lint.

Dois blocos merecem sua atencao imediata quando aparecem:

**DIVERGENCIA — relatorio afirma verde, comando deu erro.** O relatorio do
Devin afirma "vermelho, correcao, verde" com a mesma confianca quando rodou e
quando nao rodou. Aqui o exit code discorda dele. Leia a saida capturada.

**Mutacao nao provou nada.** O teste passa tambem com a correcao desfeita.
Ele pode estar fixando a funcao errada, ou nao tocar o caminho corrigido.

O que o companion nao faz por voce: ler o diff inteiro. Faca isso antes de
trazer qualquer coisa para a sua branch. E rode a suite do pacote, nao so o
arquivo tocado — uma mudanca de comportamento correta ja derrubou 2 testes de
unidade e 125 cenarios de e2e em fixtures que montavam objeto incompleto.

O companion nunca faz commit nem push. Trazer o trabalho e decisao sua.
EOF
```

- [ ] **Step 6: Criar o hook de aviso de cancelamento**

O usuário pediu que o watchdog cancele sozinho **e avise**. Este hook é o caminho que não depende de ninguém rodar `status`.

```bash
cat > hooks/hooks.json <<'EOF'
{
  "description": "Avisa o Claude quando o watchdog cancelou um job do Devin.",
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash|Agent",
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${CLAUDE_PLUGIN_ROOT}/scripts/cancel-notice-hook.sh\"",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
EOF

cat > scripts/cancel-notice-hook.sh <<'EOF'
#!/bin/sh
# Injeta no contexto do Claude o motivo de qualquer job cancelado pelo
# watchdog desde a ultima verificacao. Cancelamento sem aviso vira so
# frustracao; este e o caminho que nao depende de alguem rodar status.
set -eu

state="${XDG_STATE_HOME:-$HOME/.local/state}/devin-plugin-cc/jobs"
[ -d "$state" ] || exit 0

marker="$state/.last-notice"
[ -f "$marker" ] || : > "$marker"

found=""
for reason in "$state"/*/cancel-reason.json; do
    [ -f "$reason" ] || continue
    [ "$reason" -nt "$marker" ] || continue
    job=$(basename "$(dirname "$reason")")
    found="${found}
Job $job foi cancelado pelo watchdog do Devin. Motivo registrado em $reason:
$(cat "$reason")
"
done

: > "$marker"
[ -n "$found" ] || exit 0

printf '%s' "O watchdog cancelou um ou mais jobs do Devin. Avise o usuario, leia o sinal e o trecho, e ofereca o comando de retomada — nao retome sozinho.$found" >&2
exit 2
EOF

chmod +x scripts/cancel-notice-hook.sh
```

- [ ] **Step 7: Rodar e confirmar o verde**

Run: `go test ./internal/pluginfiles/ -v && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 8: Instalar e exercitar de ponta a ponta**

```bash
./scripts/companion doctor
```

No Claude Code, registre o marketplace local e instale o plugin a partir deste diretório; depois confirme que `/devin:setup` responde com o mesmo diagnóstico do comando acima.

- [ ] **Step 9: Commit**

```bash
git add .claude-plugin commands agents skills hooks scripts internal/pluginfiles
git commit -m "feat: superficie do plugin, hook de aviso e skills de uso"
```

---

### Task 18: `AGENTS.md` para o Codex, README, evals e CI

**Files:**
- Create: `AGENTS.md`, `evals/README.md`, `evals/fixtures.json`, `internal/jev/evals_test.go`, `.github/workflows/ci.yml`
- Modify: `README.md`

**Interfaces:**
- Consumes: tudo.
- Produces: a porta de entrada do Codex e a rede de segurança de calibragem.

- [ ] **Step 1: Escrever as fixtures e o teste de evals**

```bash
cat > evals/fixtures.json <<'EOF'
{
  "briefing": [
    {
      "nome": "briefing completo",
      "texto": "# Tarefa\n\nA rota devolve 200 com token expirado.\n\n## Contrato violado\n\n`docs/adr/0007.md`: Toda rota autenticada devolve 401 quando o token expira.\n\n## Onde esta o defeito\n\n`pkg/svc/rota.go:42`:\n```\nif user.ID == \"\" { return nil }\n```\n\n## Ordem de trabalho\n\n1. Escreva primeiro o teste que expoe este defeito.\n2. Rode o teste e confirme o vermelho antes de tocar no codigo de producao.\n3. So entao corrija.\n\n## Comandos\n\n```bash\ngo test ./pkg/svc/ -run TestRota\ngolangci-lint run ./pkg/svc/\n```\n\n## Limites\n\n- Nao faca commit e nao faca push.\n- Nao acesse a rede.\n\n## Relatorio final\n\nResponda com o teste escrito, a mudanca por arquivo e a saida dos comandos.",
      "esperado": { "aponta_arquivo_linha": true, "cita_fonte_do_contrato": true, "pede_teste_antes_da_correcao": true, "comandos_copiaveis": true, "limites_explicitos": true, "pede_relatorio": true }
    },
    {
      "nome": "briefing sem pedido de vermelho",
      "texto": "# Tarefa\n\nA rota devolve 200 com token expirado, em `pkg/svc/rota.go:42`.\n\nConforme `docs/adr/0007.md`, deveria devolver 401.\n\nCorrija e escreva um teste.\n\n```bash\ngo test ./pkg/svc/\n```\n\nNao faca commit. Ao terminar, relate a mudanca por arquivo e a saida dos comandos.",
      "esperado": { "aponta_arquivo_linha": true, "cita_fonte_do_contrato": true, "pede_teste_antes_da_correcao": false, "comandos_copiaveis": true, "limites_explicitos": true, "pede_relatorio": true }
    },
    {
      "nome": "briefing vago",
      "texto": "Arruma o bug do login, ta quebrado faz tempo. Testa direito antes de entregar.",
      "esperado": { "aponta_arquivo_linha": false, "cita_fonte_do_contrato": false, "pede_teste_antes_da_correcao": false, "comandos_copiaveis": false, "limites_explicitos": false, "pede_relatorio": false }
    }
  ],
  "watchdog": [
    {
      "nome": "turno saudavel",
      "atual": "turno 3: corrigindo\n  edit(pkg/svc/rota.go) -> [ok] 1 hunk\n",
      "previos": "turno 2: confirmando o vermelho\n  exec(go test ./pkg/svc/) -> [ok] FAIL: TestRota\n",
      "esperado": { "sem_progresso": false, "bloqueio_de_permissao": false }
    },
    {
      "nome": "travado repetindo",
      "atual": "turno 6: tentando de novo\n  exec(go test ./x/) -> [erro] FAIL: cannot find module\n",
      "previos": "turno 4: tentando de novo\n  exec(go test ./x/) -> [erro] FAIL: cannot find module\nturno 5: tentando de novo\n  exec(go test ./x/) -> [erro] FAIL: cannot find module\n",
      "esperado": { "sem_progresso": true, "bloqueio_de_permissao": false }
    },
    {
      "nome": "bloqueado por permissao",
      "atual": "turno 1: aguardando confirmacao para rodar o teste\n  exec(go test ./...) -> [pendente] aguardando confirmacao do usuario\n",
      "previos": "turno 0: lendo\n  read(pkg/svc/rota.go) -> [ok] func Rota() {...}\n",
      "esperado": { "sem_progresso": false, "bloqueio_de_permissao": true }
    }
  ],
  "achados": [
    {
      "nome": "achado acionavel",
      "texto": "pkg/svc/rota.go:42 devolve 200 quando o token expirou e o cache esta quente, porque a checagem de expiracao roda antes da leitura do cache.",
      "esperado": { "tem_cenario_reproduzivel": true, "severidade_minima": 2.0 }
    },
    {
      "nome": "opiniao de estilo",
      "texto": "O nome da variavel `u` podia ser mais descritivo neste arquivo.",
      "esperado": { "tem_cenario_reproduzivel": false, "severidade_maxima": 1.0 }
    }
  ]
}
EOF
```

```go
// internal/jev/evals_test.go
//go:build evals

package jev

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// Estes testes rodam contra o Jev real e conferem que cada probabilidade cai
// do lado certo do limiar. Eles nao validam o modelo: validam a calibragem
// das nossas perguntas no nosso dominio. Rode com:
//   go test -tags evals ./internal/jev/ -v
func client(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY ausente; evals pulados")
	}
	return New(Options{APIKey: key})
}

type fixtures struct {
	Briefing []struct {
		Nome     string          `json:"nome"`
		Texto    string          `json:"texto"`
		Esperado map[string]bool `json:"esperado"`
	} `json:"briefing"`
	Watchdog []struct {
		Nome     string          `json:"nome"`
		Atual    string          `json:"atual"`
		Previos  string          `json:"previos"`
		Esperado map[string]bool `json:"esperado"`
	} `json:"watchdog"`
}

func load(t *testing.T) fixtures {
	t.Helper()
	raw, err := os.ReadFile("../../evals/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestEvalBriefingGate(t *testing.T) {
	c := client(t)
	for _, tc := range load(t).Briefing {
		t.Run(tc.Nome, func(t *testing.T) {
			state := map[string]any{"briefing": map[string]any{"texto": tc.Texto}}
			res, err := c.Ask(context.Background(), state, BriefingQuestions())
			if err != nil {
				t.Fatalf("Ask: %v", err)
			}
			for id, want := range tc.Esperado {
				p, ok := res.Answers.NoulOf(id)
				if !ok {
					t.Errorf("%s: resposta ausente", id)
					continue
				}
				got := p >= 0.60 // o limiar BriefingItem de gate.DefaultThresholds
				if got != want {
					t.Errorf("%s: noul=%.2f -> %v, quero %v", id, p, got, want)
				}
			}
		})
	}
}

func TestEvalWatchdog(t *testing.T) {
	c := client(t)
	limiar := map[string]float64{"sem_progresso": 0.70, "bloqueio_de_permissao": 0.90}

	for _, tc := range load(t).Watchdog {
		t.Run(tc.Nome, func(t *testing.T) {
			state := map[string]any{"janela": map[string]any{
				"turno_atual": tc.Atual, "turnos_previos": tc.Previos,
			}}
			res, err := c.Ask(context.Background(), state, WatchdogQuestions())
			if err != nil {
				t.Fatalf("Ask: %v", err)
			}
			for id, want := range tc.Esperado {
				p, ok := res.Answers.NoulOf(id)
				if !ok {
					t.Errorf("%s: resposta ausente", id)
					continue
				}
				if got := p >= limiar[id]; got != want {
					t.Errorf("%s: noul=%.2f (limiar %.2f) -> %v, quero %v", id, p, limiar[id], got, want)
				}
			}
		})
	}
}
```

```bash
cat > evals/README.md <<'EOF'
# Evals

Fixtures rotuladas para conferir se as perguntas do `internal/jev/questions.go`
caem do lado certo do limiar no nosso dominio.

```bash
go test -tags evals ./internal/jev/ -v
```

Precisam de `TYPESAFE_API_KEY` e consomem tokens reais — poucos, porque o
Jev cobra $0,042 por milhao de tokens de entrada e a saida e gratuita.

Quando um eval falhar, a pergunta e qual dos dois esta errado: o limiar em
`gate.DefaultThresholds` / `watchdog.DefaultConfig`, ou o texto da pergunta.
Mudou o texto de uma pergunta, incremente `jev.QuestionsVersion`, senao a
auditoria em `jev.jsonl` deixa de ser interpretavel.

Estes testes nao rodam no CI: eles dependem de rede e de chave.
EOF
```

- [ ] **Step 2: Rodar os evals**

Run: `go test -tags evals ./internal/jev/ -v`
Expected: PASS com `TYPESAFE_API_KEY` no ambiente. Falha aqui significa recalibrar limiar ou reescrever a pergunta — anote qual dos dois você mudou.

Run sem a chave: `env -u TYPESAFE_API_KEY go test -tags evals ./internal/jev/ -v`
Expected: SKIP, não FAIL.

- [ ] **Step 3: Escrever o `AGENTS.md`**

```bash
cat > AGENTS.md <<'EOF'
# AGENTS.md

Contrato para agentes que trabalham neste repositorio, e porta de entrada
para o Codex usar este plugin.

## Usando o companion fora do Claude Code

O binario nao depende do Claude. Chame-o direto:

```bash
./scripts/companion doctor
./scripts/companion plan --task "<o defeito em uma frase>" \
    --evidence evidencias.jsonl --worktree ../wt-tarefa \
    --test-cmd "go test ./pkg/svc/ -run TestRota" \
    --suite-cmd "go test ./pkg/svc/" \
    --lint-cmd "golangci-lint run ./pkg/svc/" --json
./scripts/companion task --job <job-id>
./scripts/companion wait-and-result <job-id> --max-wait 480
```

Exit 3 no `plan` significa que o gate reprovou; o JSON traz `faltando`.
Exit 2 no `wait-and-result` significa que ainda roda: chame de novo.

Monte `evidencias.jsonl` com o contexto que voce ja leu, verbatim, um objeto
por linha, com `kind` em `trecho|erro|comando|fonte`. Nao resuma: o companion
decide o que entra, e resumo perde caminho de arquivo e erro exato.

## Regras do codigo

- Go 1.27+, stdlib pura. Nenhuma dependencia externa, nem o SDK do TypeSafe.
- Nenhum identificador de modelo do Devin hardcoded: sempre via `devin models list`.
- `--permission-mode dangerous` nunca e escolhido pela rota.
- `TYPESAFE_API_KEY` nunca vai para disco, log, relatorio ou mensagem de erro.
- Codigo primeiro, Jev depois: o que da para saber contando, se sabe contando.
- Toda mudanca entra por teste que falha antes.
- Mudou o texto de uma pergunta Jev? Incremente `jev.QuestionsVersion` e
  atualize a fixture correspondente em `evals/`.

## Documentation Maintenance

Para qualquer mudanca de codigo, configuracao, workflow, API, schema, UI,
seguranca, operacao, definicao de dado ou arquitetura, trate a manutencao da
documentacao como parte da Definition of Done.

No fechamento, inclua um bloco `Doc Delta`, a menos que o usuario peca
explicitamente uma resposta estreita sem implementacao ou revisao:

```md
### Doc Delta

- Doc impact: yes/no
- Docs updated:
- Docs that should change but were not changed:
- Relevant section anchors:
- Behavior/API/schema/config/runbook changes:
- Follow-up doc task:
- Reason if Doc impact is no:
```

Use `Doc impact: no` apenas quando a mudanca for interna e nao afetar
comportamento visivel, contrato de API, schema, setup, deploy, runbook,
postura de seguranca, definicao de metrica, fluxo de produto ou arquitetura
documentada.
EOF
```

- [ ] **Step 4: Escrever o README e o CI**

```bash
cat > README.md <<'EOF'
# devin-plugin-cc

Usa o [Devin CLI](https://docs.devin.ai/cli) como subagente do Claude Code,
com [Jev](https://docs.typesafe.ai) — o modelo System One da TypeSafe — como
classificador barato em cinco pontos do processo.

O ponto nao e automatizar a delegacao. E parar de pagar por ela duas vezes:
uma quando um run condenado ocupa quarenta minutos antes de alguem perceber,
outra quando o modelo caro le um relatorio inteiro para descobrir se "verde"
era verdade.

## O que o Jev faz aqui

1. **Porteiro de delegabilidade** — tarefa com desenho em aberto, que toca
   credencial ou sem criterio de pronto nao chega a ser despachada.
2. **Gate de briefing** — os seis itens do protocolo viram condicao
   verificavel; faltou um, o dispatch e recusado com o nome do que faltou.
3. **Watchdog** — poll do export a cada turno; run travado ou bloqueado por
   permissao e morto em minutos, com o trecho que o provocou e o comando de
   retomada.
4. **Rota** — modo de permissao e nivel de esforco escolhidos pela tarefa, em
   vez de chutados. `dangerous` nunca e escolhido.
5. **Compactacao por delecao** — evidencia verbatim na ida, trace enxuto na
   volta. Nunca reescreve: resumo perde caminho de arquivo e erro exato.

A $0,042 por milhao de tokens de entrada, os cinco somados custam fracoes de
centavo por tarefa.

## Verificacao

O `result` executa e captura o diff, o teste, o **teste de mutacao**, a suite
do pacote e o lint — fatos, sem modelo no caminho. Se o relatorio afirmar
verde e um exit code discordar, ou se o teste passar tambem com a correcao
desfeita, o relatorio abre com um bloco DIVERGENCIA.

## Instalacao

Precisa de Go 1.27+, o `devin` CLI autenticado e `TYPESAFE_API_KEY` no
ambiente. Registre este diretorio como marketplace local no Claude Code e
instale o plugin `devin`; o wrapper compila o binario na primeira execucao.

Fora do Claude Code, veja [AGENTS.md](AGENTS.md).

## Documentos

- [Design](docs/superpowers/specs/2026-09-20-devin-plugin-cc-design.md)
- [Plano de implementacao](docs/superpowers/plans/2026-09-20-devin-plugin-cc.md)
EOF

mkdir -p .github/workflows
cat > .github/workflows/ci.yml <<'EOF'
name: CI

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: read

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.27'
      - name: Sem dependencias externas
        run: |
          if grep -q '^require' go.mod; then
            echo "go.mod ganhou dependencia externa; o plugin e stdlib pura" >&2
            exit 1
          fi
      - run: go vet ./...
      - run: go test ./...
      - name: staticcheck
        run: |
          go install honnef.co/go/tools/cmd/staticcheck@latest
          staticcheck ./...
EOF
```

- [ ] **Step 5: Rodar tudo**

Run: `go vet ./... && go test ./... && ./scripts/companion doctor`
Expected: PASS, e o diagnóstico do ambiente.

- [ ] **Step 6: Commit**

```bash
git add AGENTS.md README.md evals .github internal/jev/evals_test.go
git commit -m "docs: AGENTS.md para o Codex, README, evals e CI"
```

---

## Ordem de execução e checkpoints

As tarefas 1 a 8 podem ser revisadas em bloco: são unidades puras, sem efeito
colateral. A 9 é o primeiro checkpoint real — a partir dela existe software
que recusa tarefa mal formada. A 12 é o segundo: é onde a tese do plugin se
prova ou não, e `TestWatchdogCancelsBlockedRun` é o teste que decide isso. A
13 é o terceiro, com `TestRunFlagsTestThatProvesNothing`. Da 14 em diante o
risco cai: é superfície e apresentação.

Se algo tiver que ser cortado por tempo, corte a 16 (modo revisor): ela é
valiosa mas independente, e o plugin entrega valor sem ela. Não corte a 2
(`fakedevin`), porque sem ela nenhuma das outras é testável de verdade.
