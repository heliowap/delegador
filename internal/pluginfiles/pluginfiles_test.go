// pluginfiles_test.go — conformidade da superfície do plugin: manifesto
// válido, frontmatter em todo Markdown, nenhum Markdown referenciando
// subcomando que não existe e nenhum ensinando a contornar a permissão.
// O plugin quebra em silêncio quando um Markdown mente sobre o binário —
// estes testes fecham isso.
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

// root localiza a raiz do módulo subindo do arquivo de teste até o go.mod.
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

// markdownFiles enumera todo .md sob os diretórios da superfície.
func markdownFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, dir := range []string{"commands", "agents", "skills"} {
		base := filepath.Join(root(t), dir)
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return err
			}
			out = append(out, path)
			return nil
		})
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
	}
	if len(out) == 0 {
		t.Fatal("nenhum Markdown na superficie — o teste passaria de mentira")
	}
	return out
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
	if m.Name != "delegador" || m.Version == "" || m.Description == "" {
		t.Errorf("manifest incompleto: %+v", m)
	}

	// O marketplace tambem e superficie: JSON invalido quebra a instalacao.
	raw, err = os.ReadFile(filepath.Join(root(t), ".claude-plugin", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mk struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &mk); err != nil {
		t.Fatal(err)
	}
	if len(mk.Plugins) == 0 || mk.Plugins[0].Name != "delegador" {
		t.Errorf("marketplace incompleto: %+v", mk)
	}
}

func TestEveryMarkdownHasFrontmatter(t *testing.T) {
	for _, path := range markdownFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(raw), "---\n") {
			t.Errorf("%s nao comeca com frontmatter", path)
		}
	}
}

// Markdown que chama um subcomando inexistente quebra em silencio no uso.
// A convencao que torna isso testavel: `delegador` em prosa vai sempre
// entre crases (que quebram o match); em codigo, `delegador <sub>` tem de
// ser um subcomando de verdade.
var cmdRe = regexp.MustCompile(`delegador"?[ \t]+([a-z][a-z-]+)`)

func TestMarkdownOnlyReferencesRealSubcommands(t *testing.T) {
	known := map[string]bool{
		"plan": true, "run": true, "status": true,
		"result": true, "roster": true, "doctor": true,
	}
	for _, path := range markdownFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range cmdRe.FindAllStringSubmatch(string(raw), -1) {
			if !known[m[1]] {
				t.Errorf("%s referencia subcomando inexistente: %q", path, m[1])
			}
		}
	}
}

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
