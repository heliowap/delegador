// internal/cli/run_test.go
package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/job"
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

	// A escalada TROCA de modelo: a segunda tentativa nao pode voltar ao
	// barato que a verificacao reprovou — o re-roteio filtra o falho.
	var barato, forte int
	for _, r := range *env.Requests {
		switch r.Model {
		case "barato":
			barato++
		case "forte":
			forte++
		}
	}
	if forte == 0 {
		t.Error("a segunda tentativa tinha que ir ao modelo forte; nenhum pedido foi")
	}
	if barato == 0 {
		t.Error("a primeira tentativa tinha que ir ao modelo barato; nenhum pedido foi")
	}

	j, err := job.Load(env.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Escaladas != 1 {
		t.Errorf("Escaladas = %d, quero 1", j.Escaladas)
	}
	if !j.State.Terminal() {
		t.Errorf("State = %q, quero estado terminal", j.State)
	}
	// As duas tentativas deixam o proprio verify: numeracao continua, sem
	// sobrescrever o artefato da anterior.
	for _, name := range []string{"verify-1.json", "verify-2.json"} {
		if _, err := os.Stat(j.Path(name)); err != nil {
			t.Errorf("%s ausente no diretorio do job: %v", name, err)
		}
	}
}

// Um job vetado antes, retomado e que morre num retorno cedo (aqui: a
// rota esgota — um modelo so no roster, escalada sem candidato) nao pode
// salvar de volta o veto da tentativa anterior: status mentiria CANCELADO.
func TestRunRetornoCedoNaoCarregaCancelReasonVelho(t *testing.T) {
	env := setupRunEnv(t, cenarioQueFalhaDepoisPassa())
	// Roster de um modelo so: a verificacao reprova o barato, a cascata
	// pede escalada e nao ha candidato — retorno cedo de rota esgotada.
	t.Setenv("DELEGADOR_ROSTER", escreveRosterUmModelo(t))

	j, err := job.Load(env.JobID)
	if err != nil {
		t.Fatal(err)
	}
	j.State = job.StateFailed
	j.CancelReason = &job.CancelReason{
		Signal: "sem_progresso", Probability: 0.9,
		TurnExcerpt:   "veto da tentativa anterior",
		ResumeCommand: "delegador run --job " + env.JobID,
	}
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	if code := Run(context.Background(), []string{"run", "--job", env.JobID}, &out, &errBuf); code != 1 {
		t.Fatalf("exit %d, quero 1 (rota esgotada): %s", code, errBuf.String())
	}

	depois, err := job.Load(env.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if depois.CancelReason != nil {
		t.Errorf("CancelReason sobreviveu ao retorno cedo: %+v", depois.CancelReason)
	}
	if depois.State != job.StateFailed {
		t.Errorf("State = %q, quero failed", depois.State)
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
