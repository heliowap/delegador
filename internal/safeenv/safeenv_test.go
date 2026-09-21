package safeenv

import (
	"strings"
	"testing"
)

// A chave que o pai usa para assinar nunca pode chegar ao ambiente do
// filho — e codigo escrito por modelo que le o filho.
func TestListStripsCredentialsAndNamespaces(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "segredo-jev")
	t.Setenv("TYPESAFE_BASE_URL", "https://x")
	t.Setenv("DELEGADOR_API_KEY", "segredo-proxy")
	t.Setenv("DELEGADOR_BASE_URL", "http://x")
	t.Setenv("OPENROUTER_BASE_URL", "https://y")
	t.Setenv("MINHA_APP_TOKEN", "tok")
	t.Setenv("DB_PASSWORD", "pw")
	t.Setenv("VAULT_SECRET", "sec")
	t.Setenv("GCP_CREDENTIALS", "cred")
	t.Setenv("OUTRA_KEY", "k")

	env := List()
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		for _, plantada := range []string{
			"TYPESAFE_API_KEY", "TYPESAFE_BASE_URL", "DELEGADOR_API_KEY",
			"DELEGADOR_BASE_URL", "OPENROUTER_BASE_URL", "MINHA_APP_TOKEN",
			"DB_PASSWORD", "VAULT_SECRET", "GCP_CREDENTIALS", "OUTRA_KEY",
		} {
			if name == plantada {
				t.Errorf("variavel negada vazou para o filho: %s", name)
			}
		}
	}
}

// O filtro tira credencial, nao o ambiente: PATH e HOME tem que chegar —
// sem eles nenhum comando roda.
func TestListKeepsOperationalEnv(t *testing.T) {
	for _, want := range []string{"PATH", "HOME"} {
		found := false
		for _, kv := range List() {
			if name, _, _ := strings.Cut(kv, "="); name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s sumiu do ambiente do filho", want)
		}
	}
}

// GOPRIVATE/GONOSUMDB nao sao credencial: builds de modulo privado rodam
// DENTRO do filho e morrem sem elas.
func TestListKeepsGoPrivateVars(t *testing.T) {
	t.Setenv("GOPRIVATE", "github.com/org/*")
	t.Setenv("GONOSUMDB", "github.com/org/*")

	var found int
	for _, kv := range List() {
		if strings.HasPrefix(kv, "GOPRIVATE=") || strings.HasPrefix(kv, "GONOSUMDB=") {
			found++
		}
	}
	if found != 2 {
		t.Errorf("GOPRIVATE/GONOSUMDB deveriam sobreviver ao filtro: %d", found)
	}
}
