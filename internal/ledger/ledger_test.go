package ledger

import (
	"math"
	"path/filepath"
	"testing"
)

// Jev e executor vao em arquivos separados porque sao ordens de grandeza
// diferentes: Jev cobra $0.042/M so na entrada; executor cobra os dois lados.
func TestSomaEntradaESaidaComPrecosDistintos(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "executor.jsonl")}
	if err := l.Record("turno", 1_000_000, 200_000, 0.15, 0.50); err != nil {
		t.Fatalf("Record: %v", err)
	}
	usd, err := l.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	// 1M x 0.15 + 0.2M x 0.50 = 0.15 + 0.10 = 0.25
	if math.Abs(usd-0.25) > 1e-9 {
		t.Errorf("usd = %v, quero 0.25", usd)
	}
}

func TestArquivoAusenteEhZeroNaoErro(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "nao-existe.jsonl")}
	if usd, err := l.Total(); err != nil || usd != 0 {
		t.Errorf("quero 0 sem erro, tenho %v / %v", usd, err)
	}
}
