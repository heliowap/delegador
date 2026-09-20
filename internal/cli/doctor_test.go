package cli

import (
	"context"
	"errors"
	"testing"
)

func TestCheckReportsMissingDevin(t *testing.T) {
	lookPath := func(string) (string, error) { return "", errors.New("not found") }
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, nil }

	_, err := Check(context.Background(), lookPath, run)
	if err == nil {
		t.Fatal("quero erro quando o devin nao esta no PATH")
	}
}

func TestCheckDetectsMissingFlags(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/local/bin/devin", nil }
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("devin 3000.10.31 (b98cc431)\n"), nil
		}
		// help sem --export, para provar que a deteccao funciona
		return []byte("--prompt-file --permission-mode --respect-workspace-trust --model -p"), nil
	}

	rep, err := Check(context.Background(), lookPath, run)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.DevinVersion != "3000.10.31" {
		t.Errorf("DevinVersion = %q, quero 3000.10.31", rep.DevinVersion)
	}
	if len(rep.MissingFlags) != 1 || rep.MissingFlags[0] != "--export" {
		t.Errorf("MissingFlags = %v, quero [--export]", rep.MissingFlags)
	}
}

func TestCheckAllFlagsPresent(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/local/bin/devin", nil }
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("devin 3000.10.31 (b98cc431)\n"), nil
		}
		return []byte("--prompt-file --permission-mode --respect-workspace-trust --model --export -p -c"), nil
	}

	rep, err := Check(context.Background(), lookPath, run)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(rep.MissingFlags) != 0 {
		t.Errorf("MissingFlags = %v, quero vazio", rep.MissingFlags)
	}
}
