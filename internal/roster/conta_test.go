package roster

import (
	"testing"
	"time"
)

// A ordem responde a uma pergunta so: o que se gasta primeiro e o que se
// repoe mais facil?
func TestOrdemDasContas(t *testing.T) {
	promo := Conta{Nome: "devin-pro", Escassez: "promocao", Ate: time.Now().AddDate(0, 0, 9)}
	folga := Conta{Nome: "opencode-go", Escassez: "assinatura", Aperto: "baixo"}
	pago := Conta{Nome: "fireworks", Escassez: "pre_pago"}
	media := Conta{Nome: "antigravity", Escassez: "assinatura", Aperto: "medio"}
	aperto := Conta{Nome: "codex-pro", Escassez: "assinatura", Aperto: "alto"}
	muda := Conta{Nome: "claude-max", Escassez: "assinatura", Aperto: "medio", Orquestrador: true}

	emOrdem := []Conta{promo, folga, pago, media, aperto, muda}
	for i := 1; i < len(emOrdem); i++ {
		if emOrdem[i-1].Ordem() >= emOrdem[i].Ordem() {
			t.Errorf("%s (%d) deveria vir antes de %s (%d)",
				emOrdem[i-1].Nome, emOrdem[i-1].Ordem(), emOrdem[i].Nome, emOrdem[i].Ordem())
		}
	}
}

// A conta do orquestrador vai por ultimo mesmo quando a cota dela e a mais
// folgada do conjunto: o que ela paga nao e so o executor, e o contexto de
// quem esta conduzindo o trabalho.
func TestOrquestradorPorUltimoMesmoComFolga(t *testing.T) {
	folgado := Conta{Escassez: "assinatura", Aperto: "baixo", Orquestrador: true}
	apertado := Conta{Escassez: "assinatura", Aperto: "alto"}
	if folgado.Ordem() <= apertado.Ordem() {
		t.Errorf("orquestrador %d nao pode vir antes de assinatura apertada %d",
			folgado.Ordem(), apertado.Ordem())
	}
}

// Promocao vencida perde a preferencia: o preco mudou e quem declara
// precisa dizer para quanto.
func TestPromocaoVencidaPerdeAPreferencia(t *testing.T) {
	c := Conta{Escassez: "promocao", Ate: time.Now().AddDate(0, 0, -1)}
	if c.Ordem() == OrdemPromocao {
		t.Error("promocao vencida continuou na frente")
	}
	if !c.Vencida(time.Now()) {
		t.Error("Vencida deveria acusar")
	}
}

// Conta nao declarada nao pode ser atalho para ganhar a rota.
func TestContaNaoDeclaradaNaoVenceNinguem(t *testing.T) {
	muda := Conta{}
	for _, c := range []Conta{
		{Escassez: "promocao"},
		{Escassez: "assinatura", Aperto: "baixo"},
		{Escassez: "pre_pago"},
		{Escassez: "assinatura", Aperto: "alto"},
	} {
		if muda.Ordem() <= c.Ordem() {
			t.Errorf("conta omitida (%d) venceu %q (%d)", muda.Ordem(), c.Escassez, c.Ordem())
		}
	}
}

// Prioridade explicita manda: quem opera conhece a propria cota melhor que
// qualquer regra que eu derive.
func TestPrioridadeExplicitaManda(t *testing.T) {
	c := Conta{Escassez: "assinatura", Aperto: "alto", Prioridade: 5}
	if c.Ordem() != 5 {
		t.Errorf("Ordem = %d, quero 5", c.Ordem())
	}
}
