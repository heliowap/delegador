package jev

import (
	"encoding/json"
	"testing"
)

// Medido em 2026-09-21 contra a API real: a distribuicao de um Score chega
// como OBJETO com o indice do nivel na chave, nao como lista. O codigo so
// aceitava lista, o type assertion falhava calado e Probabilities vinha nil
// em producao — enquanto a fixture, que usava lista, passava.
func TestScoreAceitaDistribuicaoNasDuasFormas(t *testing.T) {
	casos := map[string]string{
		"objeto": `{"c":{"type":"score","score":1.79,"confidence":0.65,
			"probabilities":{"0":0.0,"1":0.28,"2":0.65,"3":0.07}}}`,
		"lista": `{"c":{"type":"score","score":1.79,"confidence":0.65,
			"probabilities":[0.0,0.28,0.65,0.07]}}`,
	}
	for nome, raw := range casos {
		t.Run(nome, func(t *testing.T) {
			var a Answers
			if err := json.Unmarshal([]byte(raw), &a); err != nil {
				t.Fatal(err)
			}
			sa, ok := a.ScoreOf("c")
			if !ok {
				t.Fatal("resposta nao lida")
			}
			if len(sa.Probabilities) != 4 {
				t.Fatalf("quero 4 niveis, tenho %v", sa.Probabilities)
			}
			if sa.Probabilities[2] != 0.65 {
				t.Errorf("nivel 2 = %v, quero 0.65 — a ordem dos niveis se perdeu", sa.Probabilities[2])
			}
		})
	}
}

// Nivel de massa zero omitido nao pode deslocar os demais.
func TestObjetoComNivelOmitidoNaoDesloca(t *testing.T) {
	var a Answers
	if err := json.Unmarshal([]byte(
		`{"c":{"type":"score","score":2,"probabilities":{"1":0.3,"3":0.7}}}`), &a); err != nil {
		t.Fatal(err)
	}
	sa, _ := a.ScoreOf("c")
	if len(sa.Probabilities) != 4 || sa.Probabilities[3] != 0.7 || sa.Probabilities[1] != 0.3 {
		t.Errorf("niveis fora de lugar: %v", sa.Probabilities)
	}
}
