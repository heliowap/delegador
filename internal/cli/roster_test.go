// roster_test.go — o subcomando roster sobre arquivos temporarios: a
// listagem mostra elegibilidade e motivos, e --probe grava a sondagem nova
// no arquivo. NUNCA no config/roster.yaml de verdade.
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/heliowap/delegador/internal/roster"
)

func envVazio(string) string { return "" }

func TestRosterListaElegiveisEMotivos(t *testing.T) {
	var out, errB bytes.Buffer
	deps := doctorDeps{getenv: envVazio, now: time.Now()}
	code := rosterCmd(context.Background(), rosterComVencido(t), false, "", deps, &out, &errB)
	if code != 0 {
		t.Fatalf("exit = %d, quero 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "velho") || !strings.Contains(out.String(), "EXCLUIDO: sondagem") {
		t.Errorf("a exclusao por sondagem vencida nao apareceu: %q", out.String())
	}
	if !strings.Contains(out.String(), "1 de 2 elegiveis") {
		t.Errorf("a contagem nao apareceu: %q", out.String())
	}
}

func TestRosterProbeGravaNoArquivo(t *testing.T) {
	path := rosterComVencido(t)
	deps := doctorDeps{getenv: envVazio, now: time.Now()}
	deps.probe = func(_ context.Context, id string) (roster.Probe, error) {
		if id != "velho" {
			t.Errorf("sondou %q, quero 'velho'", id)
		}
		return roster.Probe{ToolCall: false, ReasoningContent: true, TokensBase: 99, Em: deps.now}, nil
	}

	var out, errB bytes.Buffer
	code := rosterCmd(context.Background(), path, true, "velho", deps, &out, &errB)
	if code != 0 {
		t.Fatalf("exit = %d, quero 0\n%s", code, out.String())
	}
	models, err := roster.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, m := range models {
		if m.ID == "velho" {
			if m.Sondado.ToolCall != false || !m.Sondado.ReasoningContent || m.Sondado.TokensBase != 99 {
				t.Errorf("a sondagem gravada nao e a medida: %+v", m.Sondado)
			}
		}
	}
}

func TestRosterProbeSemIdSondaOsVencidos(t *testing.T) {
	path := rosterComVencido(t)
	deps := doctorDeps{getenv: envVazio, now: time.Now()}
	var sondados []string
	deps.probe = func(_ context.Context, id string) (roster.Probe, error) {
		sondados = append(sondados, id)
		return roster.Probe{ToolCall: true, TokensBase: 10, Em: deps.now}, nil
	}

	var out, errB bytes.Buffer
	if code := rosterCmd(context.Background(), path, true, "", deps, &out, &errB); code != 0 {
		t.Fatalf("exit = %d, quero 0\n%s", code, out.String())
	}
	if len(sondados) != 1 || sondados[0] != "velho" {
		t.Errorf("sondou %v, quero [velho] — so o vencido", sondados)
	}
}

func TestRosterProbeSemVencidosNaoGasta(t *testing.T) {
	path := escreveRosterDoisModelos(t) // os dois sondados hoje
	deps := doctorDeps{getenv: envVazio, now: time.Now()}
	deps.probe = func(context.Context, string) (roster.Probe, error) {
		t.Error("sondagem em dia nao deveria ser re-medida")
		return roster.Probe{}, nil
	}

	var out, errB bytes.Buffer
	if code := rosterCmd(context.Background(), path, true, "", deps, &out, &errB); code != 0 {
		t.Fatalf("exit = %d, quero 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "nada a remeder") {
		t.Errorf("faltou a nota de tudo em dia: %q", out.String())
	}
}

func TestRosterProbeIdDesconhecidoErra(t *testing.T) {
	var out, errB bytes.Buffer
	deps := doctorDeps{getenv: envVazio, now: time.Now()}
	if code := rosterCmd(context.Background(), rosterComVencido(t), true, "fantasma", deps, &out, &errB); code != 1 {
		t.Fatalf("exit = %d, quero 1", code)
	}
	// Erros vao a stderr como nos irmaos; stdout fica so para o relatorio.
	if !strings.Contains(errB.String(), "fantasma") {
		t.Errorf("o erro do modelo desconhecido nao foi a stderr: %q", errB.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout tinha que ficar limpo no erro: %q", out.String())
	}
}
