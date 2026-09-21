// Package tools implementa as ferramentas do laço e, antes delas, a decisão
// de permissão — o único ponto entre o modelo e o disco.
package tools

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Call é uma chamada de ferramenta pedida pelo modelo.
type Call struct {
	Name string
	Args map[string]string
}

// Policy é o que este job permite. Vem do briefing e da configuração do repo;
// nada aqui é decidido por modelo.
type Policy struct {
	Worktree      string
	WritePrefixes []string
	AllowCommands []string
}

// Decision é a resposta da permissão. Reason é obrigatório quando nega: ele
// volta ao modelo como resultado de ferramenta, para ele tentar outro caminho
// em vez de morrer no meio.
type Decision struct {
	Allowed bool
	Reason  string
}

func deny(format string, a ...any) Decision {
	return Decision{Allowed: false, Reason: fmt.Sprintf(format, a...)}
}

var allow = Decision{Allowed: true}

// readTools leem; writeTools alteram o repositório.
var (
	readTools  = map[string]bool{"read_file": true, "list_dir": true, "grep": true}
	writeTools = map[string]bool{"write_file": true, "edit_file": true}
)

// Allow decide se a chamada pode executar. Ferramenta desconhecida é negada:
// não há benefício da dúvida aqui.
func Allow(c Call, p Policy) Decision {
	switch {
	case c.Name == "exec":
		return allowExec(c.Args["command"], p)
	case readTools[c.Name]:
		return allowPath(c.Args["path"], p, false)
	case writeTools[c.Name]:
		return allowPath(c.Args["path"], p, true)
	default:
		return deny("ferramenta desconhecida: %q", c.Name)
	}
}

// ---------- caminho ----------

// allowPath confere que o caminho fica dentro da worktree depois de resolver
// symlink, e — para escrita — que casa com um dos prefixos declarados.
func allowPath(rel string, p Policy, write bool) Decision {
	if rel == "" {
		return deny("caminho vazio")
	}
	if filepath.IsAbs(rel) {
		return deny("caminho absoluto não é aceito: %q", rel)
	}

	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return deny("caminho sai da worktree: %q", rel)
	}

	// .git e interno do git em qualquer nivel: hooks, config e textconv la
	// dentro executam codigo durante o verify e persistem ataque de
	// operador. A negação e por segmento — "a/.git/x" tambem e git interno.
	if write {
		for _, seg := range strings.Split(clean, string(filepath.Separator)) {
			if seg == ".git" {
				return deny("escrita em .git nao e permitida: %q", rel)
			}
		}
	}

	root, err := resolve(p.Worktree)
	if err != nil {
		return deny("worktree irresolúvel: %v", err)
	}
	target, err := resolve(filepath.Join(p.Worktree, clean))
	if err != nil {
		return deny("caminho irresolúvel: %v", err)
	}
	if !under(target, root) {
		return deny("caminho resolve para fora da worktree: %q", rel)
	}

	if !write {
		return allow
	}
	for _, prefix := range p.WritePrefixes {
		if prefix == "" {
			return allow // "" significa a worktree inteira; use com intenção
		}
		// Fronteira de diretório: o prefixo "pkg/svc" não autoriza "pkg/svcX".
		trimmed := strings.TrimSuffix(filepath.Clean(prefix), string(filepath.Separator))
		if clean == trimmed || strings.HasPrefix(clean, trimmed+string(filepath.Separator)) {
			return allow
		}
	}
	return deny("escrita fora dos prefixos permitidos %v: %q", p.WritePrefixes, rel)
}

// resolve segue symlink até o ancestral existente mais próximo e rejunta o
// resto. Comparar string sem resolver deixa passar symlink apontando para fora.
func resolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)

	rest := ""
	cur := abs
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			if rest == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, rest), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil // nada do caminho existe; usa o absoluto limpo
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

func under(target, root string) bool {
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

// ---------- comando ----------

// shellMeta são os caracteres que permitiriam encadear um comando negado
// dentro de um permitido. O comando tem que ser invocação simples.
var shellMeta = []string{"&", ";", "|", "`", "$(", ">", "<", "\n", "\r"}

// hardDenied são negações não sobreponíveis por configuração. Prefixos por
// token, não substring: `go test -run curl` não é execução de curl.
var hardDeniedPrefixes = [][]string{
	{"git", "push"},
	{"git", "commit"},
	{"git", "reset", "--hard"},
	{"rm", "-rf"}, {"rm", "-fr"}, {"rm", "-r"},
	{"curl"}, {"wget"}, {"ssh"}, {"scp"}, {"nc"}, {"sudo"},
}

var (
	credFlags = map[string]bool{
		"--api-key": true, "--apikey": true, "--token": true, "-token": true,
		"--password": true, "--secret": true, "--key": true,
	}
	credValue = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`)
)

func allowExec(cmd string, p Policy) Decision {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return deny("comando vazio")
	}
	for _, m := range shellMeta {
		if strings.Contains(cmd, m) {
			return deny("comando encadeado ou com metacaractere de shell (%q) não é aceito", m)
		}
	}

	fields := strings.Fields(cmd)

	// Negação dura primeiro, antes de qualquer consulta à configuração —
	// senão a configuração a fura. O primeiro token é comparado pelo nome do
	// binário: /usr/bin/curl é curl, e ../../bin/ssh é ssh.
	normalized := append([]string(nil), fields...)
	normalized[0] = filepath.Base(normalized[0])
	for _, pattern := range hardDeniedPrefixes {
		if hasPrefixTokens(normalized, pattern) {
			return deny("negação dura, não sobreponível por configuração: %q", strings.Join(pattern, " "))
		}
	}

	if credValue.MatchString(cmd) {
		return deny("o comando carrega o que parece ser uma credencial")
	}
	for i, f := range fields {
		name := f
		if eq := strings.Index(f, "="); eq > 0 {
			name = f[:eq]
		}
		if credFlags[name] {
			return deny("o comando passa credencial em %q", fields[i])
		}
	}

	for _, allowed := range p.AllowCommands {
		if hasPrefixTokens(fields, strings.Fields(allowed)) {
			return allow
		}
	}
	return deny("comando fora da allowlist %v: %q", p.AllowCommands, cmd)
}

func hasPrefixTokens(fields, prefix []string) bool {
	if len(prefix) == 0 || len(fields) < len(prefix) {
		return false
	}
	for i, tok := range prefix {
		if fields[i] != tok {
			return false
		}
	}
	return true
}
