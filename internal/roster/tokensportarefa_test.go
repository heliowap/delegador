package roster

import "testing"

// A rota quer saber quanto TRABALHO o modelo precisa, nao quanto ele custa:
// o preco e decisao de quem monta o roster. TokensPorTarefa desfaz o preco
// de dentro do custo medido pelo benchmark.
//
// A proporcao entre entrada e saida importa na conta, e nao e chute: medida
// em 2026-09-21 sobre 7.142.353 tokens de entrada contra 93.556 de saida
// nas catorze execucoes do eval, a saida e 1,31% da entrada. A primeira
// versao desta derivacao supunha 50/50 e a medicao a falsificou.
func TestTokensPorTarefaDesfazOPreco(t *testing.T) {
	// glm: custo 0,0061 com precos 0,15/0,50
	// entrada = 0,0061 / (0,15 + 0,0131 x 0,50) = ~38,9k; total ~39,5k
	b := &Benchmark{CustoPorTarefaUSD: 0.0061,
		PrecoEntradaUSDPorMTok: 0.15, PrecoSaidaUSDPorMTok: 0.50}
	if got := b.TokensPorTarefa(); got < 38000 || got > 41000 {
		t.Errorf("quero ~39,5k tokens por tarefa, tenho %.0f", got)
	}
}

// A ordem do roster real, com os precos do benchmark. E a ordem que a rota
// usa; o valor absoluto nao serve para prever o custo de um run (medido, o
// opus precisou de ~250k onde esta conta preve ~85k).
func TestOrdemDoRosterReal(t *testing.T) {
	b := func(custo, in, out float64) *Benchmark {
		return &Benchmark{CustoPorTarefaUSD: custo,
			PrecoEntradaUSDPorMTok: in, PrecoSaidaUSDPorMTok: out}
	}
	glm := b(0.0061, 0.15, 0.50)
	deepseek := b(0.0075, 0.11, 0.33)
	opus := b(0.4930, 5.50, 27.50)
	fable := b(0.7261, 5.00, 25.00)
	gemini := b(0.2365, 1.35, 6.75)

	emOrdem := []struct {
		nome string
		b    *Benchmark
	}{{"glm", glm}, {"deepseek", deepseek}, {"opus", opus}, {"fable", fable}, {"gemini", gemini}}
	for i := 1; i < len(emOrdem); i++ {
		a, c := emOrdem[i-1], emOrdem[i]
		if a.b.TokensPorTarefa() >= c.b.TokensPorTarefa() {
			t.Errorf("%s (%.0f) deveria precisar de menos tokens que %s (%.0f)",
				a.nome, a.b.TokensPorTarefa(), c.nome, c.b.TokensPorTarefa())
		}
	}
	// O caso que o preco erra: o gemini custa 3x MENOS por tarefa que o
	// fable e precisa de MAIS tokens para terminar.
	if gemini.CustoPorTarefaUSD >= fable.CustoPorTarefaUSD {
		t.Fatal("a fixture perdeu o sentido: o gemini tem de ser o mais barato dos dois")
	}
	if gemini.TokensPorTarefa() <= fable.TokensPorTarefa() {
		t.Error("o gemini precisa ser o mais VERBOSO dos dois, senao nada distingue os criterios")
	}
}

// Sem preco do benchmark nao ha o que desfazer: zero, e quem consome trata.
func TestSemPrecoDoBenchmarkNaoInventa(t *testing.T) {
	if got := (&Benchmark{CustoPorTarefaUSD: 0.5}).TokensPorTarefa(); got != 0 {
		t.Errorf("quero 0, tenho %v", got)
	}
}
