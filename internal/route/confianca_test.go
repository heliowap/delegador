package route

import (
	"strings"
	"testing"

	"github.com/heliowap/delegador/internal/roster"
)

// A confianca da Choice chegava parseada e era descartada — um grep por
// `.Confidence` fora de internal/jev nao devolvia nada em 2026-09-21. Uma
// dimensao escolhida sem conviccao significa que o indice sobre o qual o
// corte vai operar e chute, e cortar por um eixo chutado e pior que nao
// cortar: o candidato passa a ter de estar acima da linha nos tres.
func TestDimensaoSemConfiancaExigeOsTresEixos(t *testing.T) {
	// especialista e o melhor do roster em coding e o pior nos outros dois.
	ms := []roster.Model{
		m("especialista", 90.0, 20.0, 0.50, 0.010, 0.10, 0.30),
		m("solido-a", 70.0, 50.0, 0.78, 0.020, 0.10, 0.30),
		m("solido-b", 72.0, 52.0, 0.79, 0.030, 0.10, 0.30),
	}
	// Com confianca alta na dimensao mecanica, o especialista ganha.
	firme, err := EscolherCom(ms, Mecanica, 0.4, Opcoes{ConfiancaDimensao: 0.95})
	if err != nil {
		t.Fatal(err)
	}
	if firme.Modelo.ID != "especialista" {
		t.Errorf("com dimensao confiavel quero especialista, tenho %q", firme.Modelo.ID)
	}
	// Com confianca baixa, ele cai: nao passa o corte em raciocinio nem em agentica.
	frouxa, err := EscolherCom(ms, Mecanica, 0.4, Opcoes{ConfiancaDimensao: 0.3})
	if err != nil {
		t.Fatal(err)
	}
	if frouxa.Modelo.ID == "especialista" {
		t.Error("dimensao no chute nao pode eleger quem so e bom naquele eixo")
	}
	if !strings.Contains(frouxa.Motivo, "tres indices") {
		t.Errorf("o relatorio precisa dizer por que o corte mudou: %q", frouxa.Motivo)
	}
}

// Ninguem passa em tudo nao trava o trabalho: vale o corte da dimensao, que
// e o comportamento de sempre.
func TestNinguemPassaEmTudoCaiNoCorteDaDimensao(t *testing.T) {
	ms := []roster.Model{
		m("a", 90.0, 20.0, 0.50, 0.010, 0.10, 0.30),
		m("b", 20.0, 90.0, 0.50, 0.020, 0.10, 0.30),
	}
	e, err := EscolherCom(ms, Mecanica, 1.0, Opcoes{ConfiancaDimensao: 0.2})
	if err != nil {
		t.Fatal(err)
	}
	if e.Modelo.ID != "a" {
		t.Errorf("quero o melhor em coding, tenho %q", e.Modelo.ID)
	}
}

// Confianca nao informada (zero) preserva o comportamento antigo.
func TestConfiancaZeroNaoMudaNada(t *testing.T) {
	ms := candidatos()
	a, _ := Escolher(ms, Mecanica, 0.25)
	b, _ := EscolherCom(ms, Mecanica, 0.25, Opcoes{ConfiancaDimensao: 0})
	if a.Modelo.ID != b.Modelo.ID {
		t.Errorf("sem confianca informada a rota mudou: %q vs %q", a.Modelo.ID, b.Modelo.ID)
	}
}
