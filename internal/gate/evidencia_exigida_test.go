package gate

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/heliowap/delegador/internal/jev"
)

// descarta tudo: simula a selecao jogando fora ate o que o gate exige.
type descartaTudo struct{}

func (descartaTudo) Ask(context.Context, any, map[string]jev.Question) (jev.Result, error) {
	raw, _ := json.Marshal(map[string]any{
		"evidencia_necessaria": map[string]any{"type": "noul", "noul": 0.05}})
	var a jev.Answers
	_ = json.Unmarshal(raw, &a)
	return jev.Result{Answers: a, Usage: jev.Usage{InputTokens: 5}}, nil
}

// Medido em 2026-09-21: a selecao descartou a fonte do contrato e o gate
// cita_fonte_do_contrato reprovou por falta dela. Duas etapas discordando
// garante reprovacao — e acoplamento, nao julgamento.
func TestSelecaoNuncaDescartaOUnicoItemDeTipoExigido(t *testing.T) {
	items := []Evidence{
		{Kind: "fonte", Ref: "docs/adr/0007.md", Text: "contrato"},
		{Kind: "trecho", Ref: "pkg/svc/rota.go:42", Text: "codigo"},
		{Kind: "comando", Text: "go test ./..."},
	}
	kept, _, err := SelectEvidence(context.Background(), descartaTudo{}, "tarefa", items)
	if err != nil {
		t.Fatalf("SelectEvidence: %v", err)
	}
	for _, tipo := range []string{"fonte", "trecho"} {
		if !temTipo(kept, tipo) {
			t.Errorf("tipo %q foi descartado; o gate que o exige reprovaria", tipo)
		}
	}
	if temTipo(kept, "comando") {
		t.Error("comando nao e tipo exigido; nao deveria ser resgatado")
	}
}

// Sem item do tipo, nao ha o que resgatar — e nao se inventa evidencia.
func TestSemItemDoTipoNaoInventa(t *testing.T) {
	kept, _, err := SelectEvidence(context.Background(), descartaTudo{},
		"tarefa", []Evidence{{Kind: "comando", Text: "go test ./..."}})
	if err != nil {
		t.Fatalf("SelectEvidence: %v", err)
	}
	if len(kept) != 0 {
		t.Errorf("quero nenhum item mantido, tenho %d", len(kept))
	}
}
