// Um "devin" falso e roteirizado. Escreve o export ATIF turno a turno,
// como o devin real faz, para que o watchdog possa ser testado sem rede.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

type tool struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Status string `json:"status"`
}

type turn struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
	Tools []tool `json:"tools"`
}

type doc struct {
	Turns []turn `json:"turns"`
}

func main() {
	var promptFile, export, model, permMode, trust string
	var print bool
	flag.StringVar(&promptFile, "prompt-file", "", "")
	flag.StringVar(&export, "export", "", "")
	flag.StringVar(&model, "model", "", "")
	flag.StringVar(&permMode, "permission-mode", "", "")
	flag.StringVar(&trust, "respect-workspace-trust", "", "")
	flag.BoolVar(&print, "p", false, "")
	flag.Parse()

	if os.Getenv("FAKEDEVIN_ECHO_ARGS") != "" {
		fmt.Println(os.Args[1:])
	}

	scenario := os.Getenv("FAKEDEVIN_SCENARIO")
	if scenario == "" {
		scenario = "healthy"
	}
	step := 50 * time.Millisecond
	if s := os.Getenv("FAKEDEVIN_STEP_MS"); s != "" {
		var ms int
		fmt.Sscanf(s, "%d", &ms)
		step = time.Duration(ms) * time.Millisecond
	}

	d := doc{}
	emit := func(tr turn) {
		d.Turns = append(d.Turns, tr)
		if export != "" {
			raw, _ := json.MarshalIndent(d, "", "  ")
			tmp := export + ".tmp"
			_ = os.WriteFile(tmp, raw, 0o644)
			_ = os.Rename(tmp, export) // escrita atomica, como o devin real
		}
		time.Sleep(step)
	}

	switch scenario {
	case "healthy":
		emit(turn{0, "lendo o arquivo", []tool{{"t0", "read", "pkg/svc/rota.go", "func Rota() {...}", "ok"}}})
		emit(turn{1, "escrevendo o teste", []tool{{"t1", "write", "pkg/svc/rota_test.go", "escrito", "ok"}}})
		emit(turn{2, "confirmando o vermelho", []tool{{"t2", "exec", "go test ./pkg/svc/", "FAIL: TestRota", "ok"}}})
		emit(turn{3, "corrigindo", []tool{{"t3", "edit", "pkg/svc/rota.go", "1 hunk", "ok"}}})
		emit(turn{4, "confirmando o verde", []tool{{"t4", "exec", "go test ./pkg/svc/", "ok  pkg/svc  0.4s", "ok"}}})
		fmt.Println("Relatorio: teste escrito, vermelho confirmado, correcao aplicada, verde confirmado.")
	case "repeat-fail":
		emit(turn{0, "rodando o teste", []tool{{"t0", "exec", "go test ./pkg/svc/", "FAIL: cannot find module", "erro"}}})
		for i := 1; i <= 6; i++ {
			emit(turn{i, "tentando de novo", []tool{{fmt.Sprintf("t%d", i), "exec", "go test ./pkg/svc/", "FAIL: cannot find module", "erro"}}})
		}
	case "permission-block":
		emit(turn{0, "lendo", []tool{{"t0", "read", "pkg/svc/rota.go", "func Rota() {...}", "ok"}}})
		emit(turn{1, "aguardando confirmacao para rodar o teste", []tool{{"t1", "exec", "go test ./...", "aguardando confirmacao do usuario", "pendente"}}})
		select {} // trava de proposito
	case "out-of-scope":
		emit(turn{0, "lendo", []tool{{"t0", "read", "pkg/svc/rota.go", "...", "ok"}}})
		emit(turn{1, "editando fora do escopo", []tool{{"t1", "edit", "infra/deploy.yaml", "1 hunk", "ok"}}})
		emit(turn{2, "editando fora do escopo", []tool{{"t2", "edit", "infra/secrets.tf", "1 hunk", "ok"}}})
	case "no-progress":
		for i := 0; i < 8; i++ {
			emit(turn{i, "explorando o repositorio", []tool{{fmt.Sprintf("t%d", i), "exec", "ls -la", "total 48", "ok"}}})
		}
	default:
		fmt.Fprintf(os.Stderr, "fakedevin: cenario desconhecido %q\n", scenario)
		os.Exit(64)
	}
}
