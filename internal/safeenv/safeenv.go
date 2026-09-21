// Package safeenv devolve o ambiente que processos filhos podem herdar:
// o os.Environ() dos pais menos as variaveis que carregam credencial ou
// apontam para a infraestrutura do delegador. Comando allowlistado roda
// codigo escrito por modelo (teste, script, Makefile) — se o filho
// herdar TYPESAFE_API_KEY, esse codigo le a chave e a exfiltra numa
// saida de teste. O pai assina as requisicoes; o filho nao precisa
// de nenhuma delas.
package safeenv

import (
	"os"
	"strings"
)

// denyPrefixes sao os namespaces do proprio delegador: chave e endpoint do
// Jev, proxy do executor, roster alternativo e o endpoint de benchmark.
// Todas somem do filho — ate as sem segredo, porque URL interna e mapa.
var denyPrefixes = []string{"TYPESAFE_", "DELEGADOR_", "OPENROUTER_"}

// denySuffixes sao os sufixos de credencial por convencao, de qualquer
// namespace: AWS_SECRET_ACCESS_KEY, GITHUB_TOKEN, NPM_PASSWORD e afins —
// o filho de um teste nao precisa de nenhum deles.
var denySuffixes = []string{"_KEY", "_TOKEN", "_SECRET", "_PASSWORD", "_CREDENTIALS"}

// List devolve os pares NOME=valor seguros para cmd.Env. GOPRIVATE,
// GONOSUMDB e GOFLAGS sobrevivem de proposito: builds de modulo privado
// precisam deles dentro do filho, e nao carregam credencial.
func List() []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if denied(name) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func denied(name string) bool {
	for _, p := range denyPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	for _, s := range denySuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}
