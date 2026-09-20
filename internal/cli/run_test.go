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
