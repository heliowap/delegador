package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunUnknownSubcommandExits2(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := Run(context.Background(), []string{"frobnicate"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("exit code = %d, quero 2", code)
	}
	if !strings.Contains(errBuf.String(), "frobnicate") {
		t.Errorf("stderr nao nomeia o subcomando desconhecido: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "doctor") {
		t.Errorf("stderr nao lista os subcomandos disponiveis: %q", errBuf.String())
	}
}

func TestRunNoArgsPrintsUsage(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := Run(context.Background(), nil, &out, &errBuf); code != 2 {
		t.Fatalf("exit code = %d, quero 2", code)
	}
}
