package route

import "testing"

func base() Orcamento { return Orcamento{Turnos: 30, TetoUSD: 5.00, TurnosOciosos: 10} }

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

// A tolerancia de ociosidade escala junto: medido em 2026-09-21, o glm gastou
// 10 turnos lendo o contrato e os testes antes da primeira escrita numa tarefa
// de volume 2.0, e o limiar fixo de 10 a matou no turno 10 de 60 concedidos.
// O preambulo que o briefing prescreve — ler contrato, ler teste, confirmar
// o vermelho — nao e escrita. Punir isso e punir o fluxo que se exige.
func TestOciosidadeNuncaFicaAbaixoDoPreambulo(t *testing.T) {
	for _, v := range []float64{0, 0.04, 0.5, 1.0} {
		got := OrcamentoPara(v, base())
		if got.TurnosOciosos < base().TurnosOciosos {
			t.Errorf("volume %.2f -> %d ociosos; volume nao pode reduzir abaixo do base %d",
				v, got.TurnosOciosos, base().TurnosOciosos)
		}
	}
}

func TestOciosidadeEscalaComOVolume(t *testing.T) {
	peq := OrcamentoPara(1, base())
	gra := OrcamentoPara(3, base())
	if peq.TurnosOciosos >= gra.TurnosOciosos {
		t.Errorf("ociosos: volume 0 -> %d, volume 2 -> %d; deveria crescer",
			peq.TurnosOciosos, gra.TurnosOciosos)
	}
	if gra.TurnosOciosos < 15 {
		t.Errorf("volume 2 -> %d turnos ociosos; autorar exige ler antes de escrever",
			gra.TurnosOciosos)
	}
}

func TestOciosidadeRespeitaLimites(t *testing.T) {
	if got := OrcamentoPara(-5, base()); got.TurnosOciosos < ociososMin {
		t.Errorf("ociosos = %d, minimo %d", got.TurnosOciosos, ociososMin)
	}
	if got := OrcamentoPara(50, base()); got.TurnosOciosos > ociososMax {
		t.Errorf("ociosos = %d, maximo %d", got.TurnosOciosos, ociososMax)
	}
}

// O piso de ociosidade e o numero que mais errou no projeto ate agora.
// Medido em 2026-09-21: com 10, catorze de dezessete celulas do piloto
// morreram por `sem_escrita`, quase todas exatamente no turno 10 ou 13 — e
// as mesmas celulas viraram verdes com 25. O laco estava matando modelos
// capazes a dez turnos do fim.
func TestPisoDeOciosidadeSustentaOPreambuloDeUmRepoGrande(t *testing.T) {
	// A menor tolerancia que o sistema emite, em qualquer volume, precisa
	// sustentar um preambulo de leitura de repositorio desconhecido.
	const preambuloObservado = 20
	for _, v := range []float64{-1, 0, 0.5, 1, 2, 4} {
		if got := OrcamentoPara(v, base()).TurnosOciosos; got < preambuloObservado {
			t.Errorf("volume %.2f -> %d ociosos; o piloto mediu ate %d turnos ate a "+
				"primeira escrita num repositorio real", v, got, preambuloObservado)
		}
	}
}
