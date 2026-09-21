package route

import (
	"path/filepath"
	"testing"

	"github.com/heliowap/delegador/internal/roster"
)

// O PisoTauMinimo nao tem rotulo de verdade que o calibre. O que ele tem e
// um efeito verificavel sobre o roster real, e e isso que este teste fixa:
// numa tarefa pouco autocontida, os dois modelos mais fracos em horizonte
// longo saem, e os demais ficam.
//
// Se o roster mudar e este teste quebrar, reveja o NUMERO, nao o teste.
func TestPisoExcluiOsFracosEmAgenticoDoRosterReal(t *testing.T) {
	ms, err := roster.Load(filepath.Join("..", "..", "config", "roster.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var medidos []roster.Model
	for _, m := range ms {
		if m.Benchmark != nil {
			medidos = append(medidos, m)
		}
	}
	if len(medidos) < 4 {
		t.Skipf("roster com %d modelos medidos; poucos para o teste valer", len(medidos))
	}

	passa := map[string]bool{}
	for _, m := range comPisoDeTau(medidos, PisoTauMinimo) {
		passa[m.ID] = true
	}

	// Medido em 2026-09-21. tau_bench do roster:
	//   deepseek 0.730  gemini 0.730  glm 0.758  fable 0.783  opus 0.792
	for _, id := range []string{"cpa-fw-glm-5.3-flash", "claude-fable-5-1", "cpa-claude-opus-5(low)"} {
		if !passa[id] {
			t.Errorf("%s deveria passar o piso: esta na metade de cima de tau_bench", id)
		}
	}
	for _, id := range []string{"devin/gemini-3-6-flash", "cpa-or-deepseek-v4.1-flash"} {
		if passa[id] {
			t.Errorf("%s nao deveria passar: agentic_index 29.0 e 41.0, os dois piores do roster", id)
		}
	}
}

// O piso nunca pode esvaziar a disputa: trabalho parado e pior que trabalho
// feito por modelo imperfeito.
func TestPisoNuncaEsvaziaORoster(t *testing.T) {
	ms, err := roster.Load(filepath.Join("..", "..", "config", "roster.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var medidos []roster.Model
	for _, m := range ms {
		if m.Benchmark != nil {
			medidos = append(medidos, m)
		}
	}
	if n := len(comPisoDeTau(medidos, PisoTauMinimo)); n == 0 {
		t.Error("o piso zerou os candidatos do roster real")
	}
}

// O limiar calibrado tem de ficar entre as classes medidas, com folga dos
// dois lados. Guarda contra alguem "ajustar" o numero sem remedir.
func TestLimiarAutocontidaFicaNoVaoMedido(t *testing.T) {
	const maiorFalse = 0.350 // medido em 2026-09-21, 7 fixtures, mediana de 3
	const menorTrue = 0.900
	if LimiarAutocontida <= maiorFalse || LimiarAutocontida >= menorTrue {
		t.Fatalf("LimiarAutocontida = %.3f fora do vao medido (%.3f..%.3f)",
			LimiarAutocontida, maiorFalse, menorTrue)
	}
	margemAbaixo := LimiarAutocontida - maiorFalse
	margemAcima := menorTrue - LimiarAutocontida
	if margemAbaixo < 0.15 || margemAcima < 0.15 {
		t.Errorf("margens assimetricas demais: %.3f abaixo, %.3f acima", margemAbaixo, margemAcima)
	}
}
