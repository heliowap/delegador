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

// Medido em 2026-09-21 na issue expr-lang/expr#950: havia DOIS trechos — o
// diff do teste, sem linha, e a triagem, com caminho:linha. A selecao
// descartou os dois, a rede resgatou o primeiro por tipo, e o gate
// aponta_arquivo_linha reprovou a tarefa. Ser `trecho` nao implica carregar
// localizacao; a rede precisa preservar a PROPRIEDADE, nao so o tipo.
func TestSelecaoPreservaOTrechoQueLocaliza(t *testing.T) {
	items := []Evidence{
		{Kind: "fonte", Ref: "github.com/exemplo/x/issues/950", Text: "relato do usuario"},
		{Kind: "trecho", Ref: "x_test.go", Text: "o diff do teste do mantenedor"},
		{Kind: "trecho", Ref: "compiler/compiler.go:614", Text: "triagem: onde o identificador aparece"},
	}
	kept, _, err := SelectEvidence(context.Background(), descartaTudo{}, "tarefa", items)
	if err != nil {
		t.Fatalf("SelectEvidence: %v", err)
	}
	if !temLocalizacao(kept) {
		t.Errorf("nenhum trecho mantido cita caminho:linha; aponta_arquivo_linha reprovaria.\nmantidos: %+v", kept)
	}
}

// Quando nenhum trecho localiza, nao ha o que resgatar — a rede nao promove
// um item que nao tem a propriedade, e o gate reprova de verdade.
func TestSemTrechoQueLocalizaNaoPromoveNada(t *testing.T) {
	items := []Evidence{
		{Kind: "fonte", Ref: "issues/1", Text: "relato"},
		{Kind: "trecho", Ref: "x_test.go", Text: "so o sintoma"},
	}
	kept, _, err := SelectEvidence(context.Background(), descartaTudo{}, "tarefa", items)
	if err != nil {
		t.Fatalf("SelectEvidence: %v", err)
	}
	if n := len(kept); n != 2 {
		t.Errorf("quero os dois resgates por tipo, tenho %d: %+v", n, kept)
	}
}
