package cli

import "testing"

// Sem isto a escalada nasce morta: o contador de custo e acumulado por job,
// entao a segunda tentativa comeca ja gastando o que a primeira gastou. Na
// issue expr-lang/expr#836, a primeira tentativa queimou US$ 2,17 de um
// teto de US$ 3,61 — o modelo novo levaria veto antes do primeiro turno.
func TestCadaTentativaRecebeOTetoDeNovo(t *testing.T) {
	const base = 3.61
	if got := tetoDaTentativa(base, 0); got != base {
		t.Errorf("primeira tentativa: quero %v, tenho %v", base, got)
	}
	if got := tetoDaTentativa(base, 1); got != base*2 {
		t.Errorf("depois de uma escalada: quero %v, tenho %v", base*2, got)
	}
	// O total continua limitado pelo numero de tentativas.
	if got := tetoDaTentativa(base, 1); got > base*2 {
		t.Errorf("o teto nao pode crescer alem de uma tentativa extra: %v", got)
	}
}

// Teto desligado (zero ou negativo) continua desligado.
func TestTetoDesligadoNaoEMultiplicado(t *testing.T) {
	for _, base := range []float64{0, -1} {
		if got := tetoDaTentativa(base, 3); got != base {
			t.Errorf("base %v: quero %v, tenho %v", base, base, got)
		}
	}
}
