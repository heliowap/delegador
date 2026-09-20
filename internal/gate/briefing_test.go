package gate

import (
	"strings"
	"testing"
)

func TestBuildBriefingKeepsEvidenceVerbatim(t *testing.T) {
	kept := []Evidence{
		{Kind: "trecho", Ref: "pkg/svc/rota.go:42", Text: "if user.ID == \"\" { return nil }"},
		{Kind: "erro", Ref: "", Text: "panic: runtime error: index out of range [3] with length 3"},
		{Kind: "fonte", Ref: "docs/adr/0007.md", Text: "Toda rota autenticada devolve 401 quando o token expira."},
	}
	out := BuildBriefing("Rota devolve 200 com token expirado.", kept, BriefingLimits{
		TestCmd:   "go test ./pkg/svc/ -run TestRota",
		SuiteCmd:  "go test ./pkg/svc/",
		LintCmd:   "golangci-lint run ./pkg/svc/",
		Forbidden: []string{"infra/", "deploy/"},
	})

	for _, want := range []string{
		"pkg/svc/rota.go:42",
		"if user.ID == \"\" { return nil }",
		"panic: runtime error: index out of range [3] with length 3",
		"Toda rota autenticada devolve 401 quando o token expira.",
		"go test ./pkg/svc/ -run TestRota",
		"golangci-lint run ./pkg/svc/",
		"infra/",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("briefing nao contem %q", want)
		}
	}
}

// Os seis itens do protocolo precisam estar no texto montado, senao o gate
// reprova o proprio template — e o teste precisa pegar isso antes.
func TestBuildBriefingCoversProtocolItems(t *testing.T) {
	out := BuildBriefing("x", []Evidence{{Kind: "trecho", Ref: "a.go:1", Text: "y"}}, BriefingLimits{
		TestCmd: "go test ./...", LintCmd: "go vet ./...",
	})
	for _, want := range []string{
		"confirme o vermelho", // teste antes da correcao
		"nao faca commit",     // limites
		"Relatorio final",     // relatorio
	} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("briefing nao cobre %q", want)
		}
	}
}

func TestBuildBriefingIncludesEnvPathsWhenGiven(t *testing.T) {
	out := BuildBriefing("x", nil, BriefingLimits{
		TestCmd:  "pytest tests/",
		VenvPath: "/Users/h/wt-main/.venv",
	})
	if !strings.Contains(out, "/Users/h/wt-main/.venv") {
		t.Error("o caminho do venv precisa aparecer: worktree nova nao tem ambiente")
	}
}

func TestParseEvidenceReadsJSONL(t *testing.T) {
	in := strings.NewReader(
		`{"kind":"trecho","ref":"a.go:1","text":"x"}` + "\n" +
			`{"kind":"erro","text":"boom"}` + "\n")
	items, err := ParseEvidence(in)
	if err != nil {
		t.Fatalf("ParseEvidence: %v", err)
	}
	if len(items) != 2 || items[0].Ref != "a.go:1" || items[1].Kind != "erro" {
		t.Errorf("parse errado: %+v", items)
	}
}
