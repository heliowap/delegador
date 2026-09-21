package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

// Medido em 2026-09-21 nas issues expr-lang/expr#836 e #685: o veto
// `sem_escrita` nao distingue "leu doze vezes" de "tentou escrever doze
// vezes e falhou", e o trace compactado nao guardava a diferenca. O trace
// bruto precisa guardar, no minimo, o nome de cada ferramenta e se o
// resultado foi erro.
func TestTraceBrutoGuardaTentativaDeEscritaQueFalhou(t *testing.T) {
	turns := []agent.Turn{{
		Index: 7,
		Message: llm.Message{ToolCalls: []tools.Call{
			{Name: "edit_file", Args: map[string]string{"path": "checker/checker.go"}}}},
		Results: []tools.Result{{IsError: true, Output: "old string nao encontrada"}},
	}}
	p := filepath.Join(t.TempDir(), "turns.jsonl")
	var errb bytes.Buffer
	gravaTraceBruto(p, turns, &errb)
	if errb.Len() > 0 {
		t.Fatalf("stderr: %s", errb.String())
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var tb turnoBruto
	if err := json.Unmarshal(bytes.TrimSpace(b), &tb); err != nil {
		t.Fatal(err)
	}
	if tb.Turno != 7 || len(tb.Chamadas) != 1 || tb.Chamadas[0].Nome != "edit_file" {
		t.Errorf("chamada perdida: %+v", tb)
	}
	if len(tb.Resultados) != 1 || !tb.Resultados[0].Erro {
		t.Errorf("o erro da escrita e a informacao que falta no veto: %+v", tb.Resultados)
	}
}

func TestTraceBrutoCortaConteudoGrande(t *testing.T) {
	grande := strings.Repeat("x", maxTrechoBruto*3)
	turns := []agent.Turn{{Results: []tools.Result{{Output: grande}}}}
	p := filepath.Join(t.TempDir(), "turns.jsonl")
	gravaTraceBruto(p, turns, &bytes.Buffer{})
	b, _ := os.ReadFile(p)
	if len(b) > maxTrechoBruto*2 {
		t.Errorf("trace bruto nao pode carregar o conteudo inteiro dos arquivos lidos: %d bytes", len(b))
	}
}
