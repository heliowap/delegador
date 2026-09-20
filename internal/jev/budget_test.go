package jev

import (
	"math"
	"path/filepath"
	"testing"
)

func TestLedgerAccumulatesTokensAndCost(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "jev.jsonl")}

	if err := l.Record("gate_delegabilidade", Usage{InputTokens: 1_000_000}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := l.Record("watchdog", Usage{InputTokens: 500_000}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	tokens, usd, err := l.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	if tokens != 1_500_000 {
		t.Errorf("tokens = %d, quero 1500000", tokens)
	}
	// 1,5M x $0,042/M = $0,063
	if math.Abs(usd-0.063) > 1e-9 {
		t.Errorf("usd = %v, quero 0.063", usd)
	}
}

func TestLedgerTotalOnMissingFileIsZero(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "nao-existe.jsonl")}
	tokens, usd, err := l.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	if tokens != 0 || usd != 0 {
		t.Errorf("quero zero, tenho %d/%v", tokens, usd)
	}
}
