package roster

import (
	"path/filepath"
	"testing"
	"time"
)

// Saida custa de 3 a 5 vezes a entrada. Um numero unico aplicado aos dois
// lados subestimava a conta na mesma proporcao — e o projeto promete ledger
// auditavel, nao aproximado.
func TestRosterRealDeclaraOsDoisPrecos(t *testing.T) {
	ms, err := Load(filepath.Join("..", "..", "config", "roster.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, m := range ms {
		if m.CustoUSDPorMTok == nil {
			t.Errorf("%s: custo de entrada nao preenchido — fica inelegivel", m.ID)
			continue
		}
		if m.CustoSaidaUSDPorMTok == nil {
			t.Errorf("%s: custo de saida ausente; o ledger vai subestimar", m.ID)
			continue
		}
		ent, sai := *m.CustoUSDPorMTok, *m.CustoSaidaUSDPorMTok
		if ent == 0 && sai == 0 {
			continue // gratuito e declarado, nao omitido
		}
		if sai < ent {
			t.Errorf("%s: saida (%.2f) menor que entrada (%.2f); conferir a fonte", m.ID, sai, ent)
		}
	}
}

// Todos elegiveis: o sistema precisa rodar de checkout limpo.
func TestRosterRealTemModelosElegiveis(t *testing.T) {
	ms, err := Load(filepath.Join("..", "..", "config", "roster.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ok, motivos := Elegiveis(ms, 365*24*time.Hour, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	if len(ok) == 0 {
		t.Fatalf("nenhum modelo elegivel; o sistema nao roda de fabrica. motivos: %v", motivos)
	}
}
