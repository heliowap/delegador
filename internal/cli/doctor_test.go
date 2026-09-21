// doctor_test.go — os checks do doctor contra servidores falsos e rosters
// temporarios: nada de rede real nem do config/roster.yaml de verdade.
package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heliowap/delegador/internal/roster"
)

// depsDeTeste monta as costuras do doctor: ambiente controlado, http contra
// o servidor falso e relogio parado.
func depsDeTeste(env map[string]string, hc *http.Client, agora time.Time) doctorDeps {
	return doctorDeps{
		getenv: func(k string) string { return env[k] },
		http:   hc,
		now:    agora,
	}
}

func envDeTeste(base string) map[string]string {
	return map[string]string{
		"DELEGADOR_BASE_URL": base,
		"TYPESAFE_API_KEY":   "k",
	}
}

// sobeProxy devolve um servidor que finge o proxy OpenAI (GET /models) e a
// base URL para o cliente.
func sobeProxy(t *testing.T, status int) (string, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.Error(w, "rota desconhecida", http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, srv.Client()
}

func TestDoctorProntoQuandoTudoResponde(t *testing.T) {
	base, hc := sobeProxy(t, http.StatusOK)
	deps := depsDeTeste(envDeTeste(base), hc, time.Now())

	var out bytes.Buffer
	code := doctor(context.Background(), escreveRosterDoisModelos(t), false, deps, &out)
	if code != 0 {
		t.Fatalf("exit = %d, quero 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "ambiente pronto") {
		t.Errorf("faltou o veredito: %q", out.String())
	}
	if !strings.Contains(out.String(), "elegiveis: 2 de 2") {
		t.Errorf("faltou a contagem de elegiveis: %q", out.String())
	}
}

func TestDoctorFalhaComACausaQuandoProxyCai(t *testing.T) {
	// Servidor que sobe e morre: o GET /models da connection refused —
	// transporte morto e a falha que o doctor existe para antecipar.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close()

	deps := depsDeTeste(envDeTeste(base), srv.Client(), time.Now())
	var out bytes.Buffer
	code := doctor(context.Background(), escreveRosterDoisModelos(t), false, deps, &out)
	if code != 1 {
		t.Fatalf("exit = %d, quero 1\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "FALHOU") || !strings.Contains(out.String(), "proxy inalcançavel") {
		t.Errorf("a causa do proxy nao apareceu: %q", out.String())
	}
}

func TestDoctorAcusaChaveAusente(t *testing.T) {
	base, hc := sobeProxy(t, http.StatusOK)
	env := envDeTeste(base)
	delete(env, "TYPESAFE_API_KEY")
	deps := depsDeTeste(env, hc, time.Now())

	var out bytes.Buffer
	if code := doctor(context.Background(), escreveRosterDoisModelos(t), false, deps, &out); code != 1 {
		t.Fatalf("exit = %d, quero 1\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "TYPESAFE_API_KEY AUSENTE") {
		t.Errorf("a causa da chave nao apareceu: %q", out.String())
	}
}

func TestDoctorAcusaRosterIlegivel(t *testing.T) {
	base, hc := sobeProxy(t, http.StatusOK)
	deps := depsDeTeste(envDeTeste(base), hc, time.Now())

	var out bytes.Buffer
	code := doctor(context.Background(), filepath.Join(t.TempDir(), "nao-existe.yaml"), false, deps, &out)
	if code != 1 {
		t.Fatalf("exit = %d, quero 1\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "roster nao carrega") {
		t.Errorf("a causa do roster nao apareceu: %q", out.String())
	}
}

// rosterComVencido grava um roster com um modelo elegivel e outro com a
// sondagem velha demais — o motivo da exclusao precisa aparecer na listagem.
func rosterComVencido(t *testing.T) string {
	t.Helper()
	yaml := "as_of_sondagem: \"2026-08-01\"\n" +
		"modelos:\n" +
		"  - id: velho\n" +
		"    papel: barato\n" +
		"    sondado:\n" +
		"      tool_call: true\n" +
		"      em: \"2020-01-01\"\n" +
		"    humano:\n" +
		"      custo_usd_por_mtok: 0.10\n" +
		"      habilitado: true\n" +
		"  - id: fresco\n" +
		"    papel: forte\n" +
		"    sondado:\n" +
		"      tool_call: true\n" +
		"      em: \"" + time.Now().Format("2006-01-02") + "\"\n" +
		"    humano:\n" +
		"      custo_usd_por_mtok: 5.00\n" +
		"      habilitado: true\n"
	p := filepath.Join(t.TempDir(), "roster.yaml")
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDoctorListaMotivoDeCadaExclusao(t *testing.T) {
	base, hc := sobeProxy(t, http.StatusOK)
	deps := depsDeTeste(envDeTeste(base), hc, time.Now())

	var out bytes.Buffer
	code := doctor(context.Background(), rosterComVencido(t), false, deps, &out)
	if code != 0 {
		t.Fatalf("exit = %d, quero 0 (ainda ha um elegivel)\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "excluido: velho: sondagem") {
		t.Errorf("o motivo da exclusao por sondagem vencida nao apareceu: %q", out.String())
	}
	if !strings.Contains(out.String(), "1 vencida(s): velho") {
		t.Errorf("o aviso de sondagem vencida nao apareceu: %q", out.String())
	}
}

func TestDoctorFalhaQuandoNinguemEElegivel(t *testing.T) {
	base, hc := sobeProxy(t, http.StatusOK)
	deps := depsDeTeste(envDeTeste(base), hc, time.Now())

	// Um modelo so, sem tool call na sondagem: zero elegiveis e o job
	// morreria na rota.
	yaml := "as_of_sondagem: \"2026-08-01\"\n" +
		"modelos:\n" +
		"  - id: unico\n" +
		"    papel: barato\n" +
		"    sondado:\n" +
		"      tool_call: false\n" +
		"      em: \"2020-01-01\"\n" +
		"    humano:\n" +
		"      custo_usd_por_mtok: 0.10\n" +
		"      habilitado: true\n"
	p := filepath.Join(t.TempDir(), "roster.yaml")
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := doctor(context.Background(), p, false, deps, &out); code != 1 {
		t.Fatalf("exit = %d, quero 1\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "nenhum modelo elegivel") {
		t.Errorf("a causa nao apareceu: %q", out.String())
	}
}

func TestDoctorProbeRemedeAGravaNoRoster(t *testing.T) {
	base, hc := sobeProxy(t, http.StatusOK)
	path := rosterComVencido(t)
	deps := depsDeTeste(envDeTeste(base), hc, time.Now())

	sondado := false
	deps.probe = func(_ context.Context, id string) (roster.Probe, error) {
		sondado = true
		if id != "velho" {
			t.Errorf("sondou %q, quero so o vencido 'velho'", id)
		}
		return roster.Probe{ToolCall: true, TokensBase: 200, LatenciaS: 1.2, Em: deps.now}, nil
	}

	var out bytes.Buffer
	code := doctor(context.Background(), path, true, deps, &out)
	if code != 0 {
		t.Fatalf("exit = %d, quero 0\n%s", code, out.String())
	}
	if !sondado {
		t.Fatal("o probe injetado nunca foi chamado")
	}
	models, err := roster.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, m := range models {
		if m.ID == "velho" {
			if m.Sondado.TokensBase != 200 || m.Sondado.Em.Format("2006-01-02") != deps.now.Format("2006-01-02") {
				t.Errorf("a sondagem gravada nao e a medida: %+v", m.Sondado)
			}
		}
	}
	if !strings.Contains(out.String(), "apos a sondagem") {
		t.Errorf("a re-avaliacao pos-sondagem nao apareceu: %q", out.String())
	}
}

func TestDoctorProbeReportaFalhaDaSondagem(t *testing.T) {
	base, hc := sobeProxy(t, http.StatusOK)
	deps := depsDeTeste(envDeTeste(base), hc, time.Now())
	deps.probe = func(context.Context, string) (roster.Probe, error) {
		return roster.Probe{}, errors.New("proxy recusou")
	}

	var out bytes.Buffer
	code := doctor(context.Background(), rosterComVencido(t), true, deps, &out)
	if !strings.Contains(out.String(), "sondagem:  velho — FALHOU") {
		t.Errorf("a falha da sondagem nao apareceu: %q", out.String())
	}
	// Um elegivel continua la, entao o ambiente segue pronto.
	if code != 0 {
		t.Fatalf("exit = %d, quero 0\n%s", code, out.String())
	}
}
