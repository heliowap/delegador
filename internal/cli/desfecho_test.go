package cli

import "testing"

// Medido em 2026-09-21 na issue expr-lang/expr#823.
func TestVetoNaoEVerificacaoVermelha(t *testing.T) {
	casos := []struct {
		nome      string
		concluido bool
		vetado    bool
		quero     int
	}{
		{"verde e relatado", true, false, 0},
		{"entrega reprovada pela verificacao", false, false, 1},
		{"laco cortado pelo watchdog", false, true, ExitInterrompido},
	}
	for _, c := range casos {
		if got := codigoDeSaida(c.concluido, c.vetado); got != c.quero {
			t.Errorf("%s: quero %d, tenho %d", c.nome, c.quero, got)
		}
	}
}

func TestAvisoSoQuandoOVetoChegouDepoisDaEntrega(t *testing.T) {
	if s := avisoVetoComVerde(true, true, true, "teto_de_custo"); s == "" {
		t.Error("veto com verde provado precisa da linha que reconcilia o relatorio")
	}
	for _, c := range []struct {
		nome                   string
		vetado, verde, provado bool
	}{
		{"sem veto", false, true, true},
		{"veto com verificacao vermelha", true, false, true},
		{"veto com verde sem prova de mutacao", true, true, false},
	} {
		if s := avisoVetoComVerde(c.vetado, c.verde, c.provado, "teto_de_custo"); s != "" {
			t.Errorf("%s: nao ha o que reconciliar, mas saiu %q", c.nome, s)
		}
	}
}
