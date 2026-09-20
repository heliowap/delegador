package roster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// escreveRoster grava um roster temporario com dois modelos: um sondado
// ha muito tempo e um sem bloco sondado. Testes de WriteProbe NUNCA tocam
// o config/roster.yaml de verdade — sempre copias em t.TempDir().
func escreveRoster(t *testing.T, corpo string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "roster.yaml")
	if err := os.WriteFile(p, []byte(corpo), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const rosterBase = `as_of_sondagem: "2026-08-01"
modelos:
  - id: velho
    papel: barato
    sondado:
      tool_call: true
      reasoning_content: false
      tokens_base: 100
      latencia_s: 1.0
    benchmark: null
    humano:
      custo_usd_por_mtok: 0.10
      habilitado: true
    nota: >
      comentario do curator que nao pode sumir

  - id: semsonda
    papel: forte
    humano:
      custo_usd_por_mtok: 5.00
      habilitado: true
`

func TestWriteProbeAtualizaSondadoEGuardaData(t *testing.T) {
	path := escreveRoster(t, rosterBase)
	em := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)

	err := WriteProbe(path, "velho", Probe{
		ToolCall: false, ReasoningContent: true,
		TokensBase: 250, LatenciaS: 2.5, Em: em,
	})
	if err != nil {
		t.Fatalf("WriteProbe: %v", err)
	}

	models, err := Load(path)
	if err != nil {
		t.Fatalf("Load apos WriteProbe: %v", err)
	}
	var velho *Model
	for i := range models {
		if models[i].ID == "velho" {
			velho = &models[i]
		}
	}
	if velho == nil {
		t.Fatal("modelo velho sumiu")
	}
	if velho.Sondado.ToolCall != false || !velho.Sondado.ReasoningContent ||
		velho.Sondado.TokensBase != 250 || velho.Sondado.LatenciaS != 2.5 {
		t.Errorf("sondado nao atualizado: %+v", velho.Sondado)
	}
	if !velho.Sondado.Em.Equal(em) {
		t.Errorf("Em = %v, quero %v", velho.Sondado.Em, em)
	}
	// O as_of_sondagem do arquivo NAO se move numa re-sondagem parcial —
	// mexer nele fingiria frescura para os modelos nao re-sondados.
	var sem *Model
	for i := range models {
		if models[i].ID == "semsonda" {
			sem = &models[i]
		}
	}
	if sem.Sondado.Em.Format("2006-01-02") != "2026-08-01" {
		t.Errorf("as_of_sondagem do arquivo vazou para o outro modelo: %v", sem.Sondado.Em)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "comentario do curator") {
		t.Error("comentario do arquivo sumiu na reescrita")
	}
}

func TestWriteProbeCriaSondadoQuandoFalta(t *testing.T) {
	path := escreveRoster(t, rosterBase)
	em := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)

	err := WriteProbe(path, "semsonda", Probe{ToolCall: true, TokensBase: 300, Em: em})
	if err != nil {
		t.Fatalf("WriteProbe: %v", err)
	}
	models, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, m := range models {
		if m.ID == "semsonda" {
			if !m.Sondado.ToolCall || m.Sondado.TokensBase != 300 || !m.Sondado.Em.Equal(em) {
				t.Errorf("sondado criado errado: %+v", m.Sondado)
			}
			return
		}
	}
	t.Fatal("semsonda sumiu")
}

func TestWriteProbePreservaComentarioDeFimDeLinha(t *testing.T) {
	// O roster de verdade tem "tokens_base: 552  # maior do roster: ..."
	// dentro de um sondado — re-sondar nao pode apagar a nota.
	path := escreveRoster(t, `as_of_sondagem: "2026-08-01"
modelos:
  - id: comnota
    papel: barato
    sondado:
      tool_call: true
      tokens_base: 552      # maior do roster: prompt injetado pelo backend
      em: "2026-08-10"
    humano:
      custo_usd_por_mtok: 0.10
      habilitado: true
`)
	em := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	if err := WriteProbe(path, "comnota", Probe{ToolCall: true, TokensBase: 600, Em: em}); err != nil {
		t.Fatalf("WriteProbe: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "# maior do roster: prompt injetado pelo backend") {
		t.Errorf("comentario de fim de linha sumiu:\n%s", raw)
	}
	if !strings.Contains(string(raw), "tokens_base: 600") {
		t.Errorf("valor nao atualizado:\n%s", raw)
	}
}

func TestWriteProbeModeloDesconhecidoErra(t *testing.T) {
	path := escreveRoster(t, rosterBase)
	if err := WriteProbe(path, "fantasma", Probe{Em: time.Now()}); err == nil {
		t.Fatal("quero erro para modelo fora do roster")
	}
}

func TestParsePrefereEmDoModeloAoAsOfDoArquivo(t *testing.T) {
	path := escreveRoster(t, `as_of_sondagem: "2026-08-01"
modelos:
  - id: comem
    papel: barato
    sondado:
      tool_call: true
      em: "2026-09-15"
    humano:
      custo_usd_por_mtok: 0.10
      habilitado: true
`)
	models, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if models[0].Sondado.Em.Format("2006-01-02") != "2026-09-15" {
		t.Errorf("Em = %v, quero 2026-09-15 (o do modelo, nao o do arquivo)", models[0].Sondado.Em)
	}
}
