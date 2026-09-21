package route

import (
	"testing"
	"time"

	"github.com/heliowap/delegador/internal/roster"
)

// O mesmo modelo, em canais diferentes, com a MESMA qualidade. E o caso
// medido no proxy em 2026-09-21: glm-5.3-flash aparece em cinco canais,
// todos com tool calling, todos com o mesmo peso por tras.
func mesmoModeloEm(id string, c roster.Conta) roster.Model {
	m := m(id, 71.5, 41.8, 0.758, 0.0061, 0.15, 0.50)
	m.Conta = c
	return m
}

var (
	promo = roster.Conta{Nome: "devin-pro", Escassez: "promocao",
		Ate: time.Now().AddDate(0, 0, 9)}
	folga   = roster.Conta{Nome: "opencode-go", Escassez: "assinatura", Aperto: "baixo"}
	prePago = roster.Conta{Nome: "fireworks", Escassez: "pre_pago"}
	orq     = roster.Conta{Nome: "claude-max", Escassez: "assinatura",
		Aperto: "medio", Orquestrador: true}
)

// Qualidade identica: o que decide e de que pote sai.
func TestEntreCanaisDoMesmoModeloVenceAContaMaisFolgada(t *testing.T) {
	ms := []roster.Model{
		mesmoModeloEm("cpa-fw-glm", prePago),
		mesmoModeloEm("cpa-ocgo-glm", folga),
		mesmoModeloEm("devin/glm", promo),
	}
	e, err := Escolher(ms, Mecanica, 0)
	if err != nil {
		t.Fatal(err)
	}
	if e.Modelo.ID != "devin/glm" {
		t.Errorf("ID = %q, quero devin/glm: promocao se usa enquanto dura", e.Modelo.ID)
	}
}

// A conta do orquestrador vai por ultimo mesmo quando o modelo dela e o
// mais eficiente do conjunto: o que ela paga tambem e o contexto de quem
// esta conduzindo o trabalho.
func TestNaoGastaCotaDoOrquestradorTendoAlternativa(t *testing.T) {
	// opus e mais eficiente que o gemini por trabalho, mas esta na conta
	// do orquestrador.
	opus := m("cpa-claude-opus", 78.0, 50.8, 0.792, 0.4930, 5.50, 27.50)
	opus.Conta = orq
	gemini := m("cpa-ocgo-gemini", 69.2, 34.0, 0.730, 0.2365, 1.35, 6.75)
	gemini.Conta = folga

	if trabalhoPorTarefa(opus) >= trabalhoPorTarefa(gemini) {
		t.Fatal("a fixture perdeu o sentido: o do orquestrador tem de ser o mais eficiente")
	}
	e, err := Escolher([]roster.Model{opus, gemini}, Mecanica, 0)
	if err != nil {
		t.Fatal(err)
	}
	if e.Modelo.ID != "cpa-ocgo-gemini" {
		t.Errorf("ID = %q: havia alternativa fora da cota do orquestrador", e.Modelo.ID)
	}
}

// Dentro da MESMA conta, o trabalho volta a decidir: dois modelos do mesmo
// pote, o que termina com menos tokens deixa mais pote para o proximo job.
func TestDentroDaMesmaContaODesempateEOTrabalho(t *testing.T) {
	gemini := m("gemini", 69.2, 34.0, 0.730, 0.2365, 1.35, 6.75)
	fable := m("fable", 81.6, 53.4, 0.783, 0.7261, 5.00, 25.00)
	gemini.Conta, fable.Conta = folga, folga
	e, err := Escolher([]roster.Model{gemini, fable}, Mecanica, 0)
	if err != nil {
		t.Fatal(err)
	}
	if e.Modelo.ID != "fable" {
		t.Errorf("ID = %q, quero fable: mesma conta, menos tokens", e.Modelo.ID)
	}
}

// Corte de qualidade continua vindo antes da conta: promocao nao compra
// passagem para quem nao passa a barra.
func TestAContaNaoAtravessaOCorteDeQualidade(t *testing.T) {
	fraco := m("fraco", 10.0, 10.0, 0.10, 0.001, 0.10, 0.30)
	fraco.Conta = promo
	forte := m("forte", 81.6, 53.4, 0.783, 0.7261, 5.00, 25.00)
	forte.Conta = orq
	e, err := Escolher([]roster.Model{fraco, forte}, Mecanica, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if e.Modelo.ID != "forte" {
		t.Errorf("ID = %q: o corte de qualidade vem antes da conta", e.Modelo.ID)
	}
}

// Medido no proxy em 2026-09-21: glm-5.3-flash esta em cinco canais. Se
// cada canal ocupasse uma posicao na distribuicao, tres copias do MESMO
// modelo cairiam em lados opostos do mesmo corte — e o corte deixaria de
// significar qualidade. A posicao e por modelo; canal nao muda peso.
func TestCanaisDoMesmoModeloNaoDistorcemADistribuicao(t *testing.T) {
	comSlug := func(id string, c roster.Conta) roster.Model {
		x := mesmoModeloEm(id, c)
		x.Permaslug = "z-ai/glm-5.3-flash-20260826"
		return x
	}
	opus := m("opus", 78.0, 50.8, 0.792, 0.4930, 5.50, 27.50)
	opus.Permaslug, opus.Conta = "anthropic/opus-5", orq

	umCanal := []roster.Model{comSlug("glm-ocgo", folga), opus}
	cincoCanais := []roster.Model{
		comSlug("glm-ocgo", folga), comSlug("glm-fw", prePago),
		comSlug("glm-or", prePago), comSlug("glm-devin", promo),
		comSlug("glm-bt", folga), opus,
	}
	for _, p := range []float64{0.0, 0.25, 0.5, 0.75, 1.0} {
		a, err := EscolherCom(umCanal, Mecanica, p, Opcoes{})
		if err != nil {
			t.Fatal(err)
		}
		b, err := EscolherCom(cincoCanais, Mecanica, p, Opcoes{})
		if err != nil {
			t.Fatal(err)
		}
		mesmoModelo := (a.Modelo.Permaslug == b.Modelo.Permaslug)
		if !mesmoModelo {
			t.Errorf("percentil %.2f: com um canal escolheu %q, com cinco escolheu %q — "+
				"o numero de canais mudou a decisao de QUALIDADE", p, a.Modelo.ID, b.Modelo.ID)
		}
	}
}
