package roster

import "testing"

// A rota quer saber quanto TRABALHO o modelo precisa, nao quanto ele custa:
// o preco e decisao de quem monta o roster. TokensPorTarefa desfaz o preco
// de dentro do custo medido pelo benchmark.
func TestTokensPorTarefaDesfazOPreco(t *testing.T) {
	// custo 0.0061 com precos 0.15/0.50 => medio 0.325 => ~18.8k tokens
	b := &Benchmark{CustoPorTarefaUSD: 0.0061,
		PrecoEntradaUSDPorMTok: 0.15, PrecoSaidaUSDPorMTok: 0.50}
	got := b.TokensPorTarefa()
	if got < 18000 || got > 19500 {
		t.Errorf("quero ~18.8k tokens por tarefa, tenho %.0f", got)
	}
	// O caro e eficiente: fable custa 119x mais por tarefa que o glm em
	// dolar, mas gasta so 1.3x mais tokens para terminar.
	fable := &Benchmark{CustoPorTarefaUSD: 0.7261,
		PrecoEntradaUSDPorMTok: 10, PrecoSaidaUSDPorMTok: 50}
	opus := &Benchmark{CustoPorTarefaUSD: 0.4930,
		PrecoEntradaUSDPorMTok: 5, PrecoSaidaUSDPorMTok: 25}
	if fable.TokensPorTarefa() >= opus.TokensPorTarefa() {
		t.Errorf("fable %.0f deveria terminar com MENOS tokens que opus %.0f",
			fable.TokensPorTarefa(), opus.TokensPorTarefa())
	}
}

// Sem preco do benchmark nao ha o que desfazer: zero, e quem consome trata.
func TestSemPrecoDoBenchmarkNaoInventa(t *testing.T) {
	if got := (&Benchmark{CustoPorTarefaUSD: 0.5}).TokensPorTarefa(); got != 0 {
		t.Errorf("quero 0, tenho %v", got)
	}
}
