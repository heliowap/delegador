// exec.go — as ferramentas que o laço executa. Run passa por Allow antes de
// tocar em disco ou processo; a negação vira texto no Result, nunca erro de
// Go, porque é o modelo quem precisa ler o motivo e tentar outro caminho.
package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Result é a resposta da ferramenta ao modelo. Não há campo de erro de Go:
// falha de permissão, de disco ou de processo é texto em Output com IsError
// marcado.
type Result struct {
	Output  string
	IsError bool
}

// Registry executa as ferramentas. O valor zero já funciona.
type Registry struct {
	// MaxOutput é o teto em bytes do texto devolvido ao modelo. Zero usa
	// defaultMaxOutput.
	MaxOutput int
}

// defaultMaxOutput protege a janela de contexto: um `go test` verboso ou um
// grep largo não podem explodir a conversa.
const defaultMaxOutput = 64 * 1024

func (r *Registry) ceiling() int {
	if r.MaxOutput > 0 {
		return r.MaxOutput
	}
	return defaultMaxOutput
}

// Run executa a chamada. Allow decide primeiro — só depois de permitida é que
// algo toca em arquivo ou processo.
func (r *Registry) Run(ctx context.Context, c Call, p Policy) Result {
	if d := Allow(c, p); !d.Allowed {
		return Result{Output: d.Reason, IsError: true}
	}

	var res Result
	switch c.Name {
	case "read_file":
		res = readFile(c, p)
	case "write_file":
		res = writeFile(c, p)
	case "edit_file":
		res = editFile(c, p)
	case "list_dir":
		res = listDir(c, p)
	case "grep":
		res = grepFiles(c, p)
	case "exec":
		res = execCmd(ctx, c, p)
	default:
		// Inalcançável: Allow já negou ferramenta desconhecida. Fica de cinto.
		res = Result{Output: "ferramenta desconhecida: " + c.Name, IsError: true}
	}
	res.Output = r.truncate(res.Output)
	return res
}

// truncate declara o corte no próprio texto — saída cortada em silêncio parece
// saída completa, e o modelo conclui errado em cima dela.
func (r *Registry) truncate(s string) string {
	max := r.ceiling()
	if len(s) <= max {
		return s
	}
	return fmt.Sprintf("%s\n[saída truncada: mostrando %d de %d bytes]", s[:max], max, len(s))
}

// abs junta o caminho relativo à worktree. Allow já garantiu que ele fica
// dentro dela depois de resolvido.
func abs(p Policy, rel string) string {
	return filepath.Join(p.Worktree, rel)
}

func readFile(c Call, p Policy) Result {
	b, err := os.ReadFile(abs(p, c.Args["path"]))
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	return Result{Output: string(b)}
}

func writeFile(c Call, p Policy) Result {
	target := abs(p, c.Args["path"])
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	content := c.Args["content"]
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	return Result{Output: fmt.Sprintf("escrito %s (%d bytes)", c.Args["path"], len(content))}
}

// editFile substitui old por new exatamente uma vez. Zero ocorrência é erro;
// mais de uma também — escolher a primeira em silêncio é como um arquivo se
// corrompe sem ninguém ver.
func editFile(c Call, p Policy) Result {
	target := abs(p, c.Args["path"])
	b, err := os.ReadFile(target)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	old, repl := c.Args["old"], c.Args["new"]
	if old == "" {
		return Result{Output: "edit_file exige `old` não vazio", IsError: true}
	}

	content := string(b)
	switch n := strings.Count(content, old); {
	case n == 0:
		return Result{Output: fmt.Sprintf("trecho não encontrado em %s", c.Args["path"]), IsError: true}
	case n > 1:
		return Result{Output: fmt.Sprintf(
			"%d ocorrências do trecho em %s; refine `old` até casar uma só vez", n, c.Args["path"]),
			IsError: true}
	}

	if err := os.WriteFile(target, []byte(strings.Replace(content, old, repl, 1)), 0o644); err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	return Result{Output: fmt.Sprintf("editado %s", c.Args["path"])}
}

func listDir(c Call, p Policy) Result {
	entries, err := os.ReadDir(abs(p, c.Args["path"]))
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	var sb strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		sb.WriteString(name)
		sb.WriteByte('\n')
	}
	return Result{Output: sb.String()}
}

// grepFiles casa pattern (regexp Go) linha a linha. Com diretório, desce
// recursivamente pulando .git; com arquivo, busca só nele.
func grepFiles(c Call, p Policy) Result {
	re, err := regexp.Compile(c.Args["pattern"])
	if err != nil {
		return Result{Output: "padrão inválido: " + err.Error(), IsError: true}
	}
	root := abs(p, c.Args["path"])
	info, err := os.Stat(root)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}

	var sb strings.Builder
	match := func(path, rel string) error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if re.MatchString(line) {
				fmt.Fprintf(&sb, "%s:%d: %s\n", rel, i+1, line)
			}
		}
		return nil
	}

	if !info.IsDir() {
		err = match(root, c.Args["path"])
	} else {
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(p.Worktree, path)
			if err != nil {
				return err
			}
			return match(path, rel)
		})
	}
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	return Result{Output: sb.String()}
}

// execCmd roda o comando sem shell: a allowlist já barrou metacaractere, e
// `sh -c` reabriria o que Allow fechou. O comando falhar não esconde a saída —
// ela volta junto com o IsError para o modelo se corrigir.
func execCmd(ctx context.Context, c Call, p Policy) Result {
	fields := strings.Fields(c.Args["command"])
	cmd := exec.CommandContext(ctx, fields[0], fields[1:]...)
	cmd.Dir = p.Worktree

	out, err := cmd.CombinedOutput()
	res := Result{Output: string(out)}
	if err != nil {
		if res.Output != "" {
			res.Output += "\n"
		}
		res.Output += "execução falhou: " + err.Error()
		res.IsError = true
	}
	return res
}

// Schemas devolve as definições no formato do campo `tools` da API OpenAI:
// cada entrada é {"type":"function","function":{name,description,parameters}}.
func (*Registry) Schemas() []map[string]any {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{
			"type":       "object",
			"properties": props,
			"required":   required,
		}
	}
	fn := func(name, desc string, params map[string]any) map[string]any {
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": desc,
				"parameters":  params,
			},
		}
	}
	pathArg := str("caminho relativo à worktree")

	return []map[string]any{
		fn("read_file", "lê um arquivo da worktree e devolve o conteúdo",
			obj(map[string]any{"path": pathArg}, "path")),
		fn("write_file", "cria ou sobrescreve um arquivo dentro dos prefixos de escrita permitidos",
			obj(map[string]any{
				"path":    pathArg,
				"content": str("conteúdo completo do arquivo"),
			}, "path", "content")),
		fn("edit_file", "substitui um trecho exato do arquivo; falha se o trecho não existe ou aparece mais de uma vez",
			obj(map[string]any{
				"path": pathArg,
				"old":  str("trecho exato a substituir; tem que casar uma única vez"),
				"new":  str("texto que entra no lugar"),
			}, "path", "old", "new")),
		fn("list_dir", "lista as entradas de um diretório da worktree (diretórios terminam em /)",
			obj(map[string]any{"path": pathArg}, "path")),
		fn("grep", "busca linhas que casam com uma expressão regular em arquivo ou diretório da worktree",
			obj(map[string]any{
				"pattern": str("expressão regular Go (RE2)"),
				"path":    pathArg,
			}, "pattern", "path")),
		fn("exec", "executa um comando da allowlist na raiz da worktree e devolve a saída combinada",
			obj(map[string]any{
				"command": str("invocação simples, sem metacaracteres de shell"),
			}, "command")),
	}
}
