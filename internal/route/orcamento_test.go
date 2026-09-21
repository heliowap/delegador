package route

import "testing"

func base() Orcamento { return Orcamento{Turnos: 30, TetoUSD: 5.00} }

// O nivel 1 e a ancora: poucos pontos que andam juntos recebem o base.
func TestVolumeUmRecebeOBase(t *testing.T) {
	got := OrcamentoPara(1.0, base())
	if got.Turnos != 30 || got.TetoUSD != 5.00 {
		t.Errorf("got = %+v, quero o base intacto", got)
	}
}

func TestVolumeCresceOOrcamento(t *testing.T) {
	casos := []struct {
		volume float64
		turnos int
	}{{0, 15}, {1, 30}, {2, 60}, {3, 120}}
	for _, c := range casos {
		if got := OrcamentoPara(c.volume, base()); got.Turnos != c.turnos {
			t.Errorf("volume %.0f -> %d turnos, quero %d", c.volume, got.Turnos, c.turnos)
		}
	}
}

// Score devolve posicao ponderada, nao degrau: 1.5 fica entre 1 e 2.
func TestEscalaEhContinua(t *testing.T) {
	meio := OrcamentoPara(1.5, base())
	um, dois := OrcamentoPara(1, base()), OrcamentoPara(2, base())
	if meio.Turnos <= um.Turnos || meio.Turnos >= dois.Turnos {
		t.Errorf("1.5 -> %d, deveria ficar entre %d e %d", meio.Turnos, um.Turnos, dois.Turnos)
	}
}

// Resposta degenerada nao pode virar laco infinito nem laco que nao comeca.
func TestLimitesProtegemDeRespostaAbsurda(t *testing.T) {
	if got := OrcamentoPara(-5, base()); got.Turnos < turnosMin {
		t.Errorf("volume negativo -> %d turnos, minimo e %d", got.Turnos, turnosMin)
	}
	if got := OrcamentoPara(50, base()); got.Turnos > turnosMax {
		t.Errorf("volume absurdo -> %d turnos, maximo e %d", got.Turnos, turnosMax)
	}
	if got := OrcamentoPara(50, base()); got.TetoUSD > tetoUSDMax {
		t.Errorf("teto = %v, maximo e %v", got.TetoUSD, tetoUSDMax)
	}
}

// Volume e dificuldade sao eixos distintos: o orcamento so olha o primeiro.
func TestOrcamentoNaoOlhaDificuldade(t *testing.T) {
	trivialEVolumoso := OrcamentoPara(3, base())
	dificilEPequeno := OrcamentoPara(0, base())
	if trivialEVolumoso.Turnos <= dificilEPequeno.Turnos {
		t.Error("trinta arquivos triviais precisam de mais turnos que uma linha sutil")
	}
}
