package roster

import (
	"strings"
	"testing"
)

const rosterComContas = `
contas:
  opencode-go:
    escassez: assinatura
    aperto: baixo
  devin-pro:
    escassez: promocao
    ate: "2026-09-30"
  claude-max:
    escassez: assinatura
    aperto: medio
    orquestrador: true

as_of_sondagem: "2026-09-20"

modelos:
  - id: cpa-ocgo-glm-5.3-flash
    papel: barato
    conta: opencode-go
    sondado:
      tool_call: true
    humano:
      custo_usd_por_mtok: 0.0
      habilitado: true
  - id: devin/swe-2
    papel: barato
    conta: devin-pro
    sondado:
      tool_call: true
    humano:
      custo_usd_por_mtok: 0.0
      habilitado: true
  - id: cpa-claude-opus-5(low)
    papel: forte
    conta: claude-max
    sondado:
      tool_call: true
    humano:
      custo_usd_por_mtok: 5.0
      habilitado: true
`

func TestContasResolvemPorNome(t *testing.T) {
	ms, err := parse([]byte(rosterComContas))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ms) != 3 {
		t.Fatalf("quero 3 modelos, tenho %d", len(ms))
	}
	porID := map[string]Model{}
	for _, m := range ms {
		porID[m.ID] = m
	}
	if c := porID["cpa-ocgo-glm-5.3-flash"].Conta; c.Nome != "opencode-go" || c.Aperto != "baixo" {
		t.Errorf("conta do glm: %+v", c)
	}
	if c := porID["devin/swe-2"].Conta; c.Escassez != "promocao" || c.Ate.IsZero() {
		t.Errorf("conta do swe-2: %+v", c)
	}
	if c := porID["cpa-claude-opus-5(low)"].Conta; !c.Orquestrador {
		t.Errorf("a conta do orquestrador precisa vir marcada: %+v", c)
	}
	// E a ordem que a rota vai usar.
	if a, b := porID["devin/swe-2"].Conta.Ordem(), porID["cpa-ocgo-glm-5.3-flash"].Conta.Ordem(); a >= b {
		t.Errorf("promocao (%d) deveria vir antes de assinatura folgada (%d)", a, b)
	}
	if a, b := porID["cpa-ocgo-glm-5.3-flash"].Conta.Ordem(), porID["cpa-claude-opus-5(low)"].Conta.Ordem(); a >= b {
		t.Errorf("assinatura folgada (%d) deveria vir antes do orquestrador (%d)", a, b)
	}
}

// Typo em nome de conta muda a preferencia da rota. Tem de gritar.
func TestContaDesconhecidaEErro(t *testing.T) {
	ruim := strings.Replace(rosterComContas, "conta: opencode-go", "conta: opencodego", 1)
	_, err := parse([]byte(ruim))
	if err == nil {
		t.Fatal("quero erro para conta inexistente")
	}
	if !strings.Contains(err.Error(), "opencodego") {
		t.Errorf("o erro precisa nomear o typo: %v", err)
	}
}

// Roster sem bloco `contas:` continua carregando: conta omitida e zero, e
// zero nao vira preferida.
func TestRosterSemContasContinuaValendo(t *testing.T) {
	semContas := rosterComContas[strings.Index(rosterComContas, "as_of_sondagem"):]
	semContas = strings.ReplaceAll(semContas, "    conta: opencode-go\n", "")
	semContas = strings.ReplaceAll(semContas, "    conta: devin-pro\n", "")
	semContas = strings.ReplaceAll(semContas, "    conta: claude-max\n", "")
	ms, err := parse([]byte(semContas))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ms) != 3 {
		t.Fatalf("quero 3 modelos, tenho %d", len(ms))
	}
	if ms[0].Conta.Ordem() != OrdemNaoDeclarada {
		t.Errorf("conta omitida = %d, quero %d", ms[0].Conta.Ordem(), OrdemNaoDeclarada)
	}
}
