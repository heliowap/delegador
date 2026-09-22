// Command sem-plugin e o BRACO DE CONTROLE do eval: o mesmo modelo, no
// mesmo laco, com as mesmas ferramentas e a mesma camada de permissao —
// e sem nenhuma das camadas de julgamento do delegador.
//
// Fica de fora: o gate de delegabilidade, o gate de briefing, a selecao de
// evidencia, a rota, o watchdog, a compactacao, a sonda de mutacao, a
// conferencia de veracidade e a cascata. O prompt e a tarefa e a evidencia
// como alguem as colaria, nao o briefing montado.
//
// Fica DENTRO a camada de permissao. Seguranca nao e a variavel sob teste:
// comparar um braco que pode `curl` com outro que nao pode mediria outra
// coisa. O laco tambem e o mesmo, para que a diferenca medida seja de
// JULGAMENTO e nao de implementacao de laco.
//
// O juiz e externo aos dois bracos: quem roda o teste do mantenedor e o
// harness, depois, igual para os dois.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/gate"
	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

func main() {
	var (
		modelo   = flag.String("modelo", "", "id do modelo no proxy")
		worktree = flag.String("worktree", "", "worktree da tarefa")
		tarefa   = flag.String("tarefa", "", "texto da tarefa")
		evidPath = flag.String("evidencia", "", "JSONL de evidencias")
		cmds     = flag.String("comandos", "", "comandos liberados, separados por ;")
		maxTurns = flag.Int("max-turns", 40, "teto de turnos")
		saida    = flag.String("saida", "", "arquivo para o resumo em JSON")
	)
	flag.Parse()
	if *modelo == "" || *worktree == "" || *tarefa == "" {
		fmt.Fprintln(os.Stderr, "uso: sem-plugin --modelo M --worktree W --tarefa T [--evidencia E] [--comandos 'a;b']")
		os.Exit(2)
	}

	var permitidos []string
	for _, c := range strings.Split(*cmds, ";") {
		if c = strings.TrimSpace(c); c != "" {
			permitidos = append(permitidos, c)
		}
	}
	policy := tools.Policy{Worktree: *worktree, WritePrefixes: []string{""}, AllowCommands: permitidos}

	prompt, err := promptNu(*tarefa, *evidPath, permitidos)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sem-plugin: %v\n", err)
		os.Exit(1)
	}

	base := os.Getenv("DELEGADOR_BASE_URL")
	if base == "" {
		base = "http://127.0.0.1:8317/v1"
	}
	c := llm.New(llm.Options{BaseURL: base, APIKey: os.Getenv("DELEGADOR_API_KEY")})

	// pre = nil: sem watchdog. E o ponto do braco de controle.
	out, runErr := agent.Run(context.Background(), c, &tools.Registry{},
		agent.Config{Model: *modelo, MaxTurns: *maxTurns, Policy: policy}, prompt, nil)

	var tin, tout int
	for _, t := range out.Turns {
		tin += t.Usage.PromptTokens
		tout += t.Usage.CompletionTokens
	}
	res := map[string]any{
		"modelo": *modelo, "turnos": len(out.Turns), "parada": out.Stop,
		"tokens_entrada": tin, "tokens_saida": tout, "final": out.Final,
	}
	if runErr != nil {
		res["erro"] = runErr.Error()
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	if *saida != "" {
		_ = os.WriteFile(*saida, append(b, '\n'), 0o644)
	}
	fmt.Println(string(b))
}

// promptNu monta o que uma pessoa colaria: a tarefa, a evidencia inteira e
// os comandos. Sem gate, sem selecao, sem secoes — a evidencia vai toda,
// porque escolher o que entra ja e trabalho do plugin.
func promptNu(tarefa, evidPath string, cmds []string) (string, error) {
	var b strings.Builder
	b.WriteString("Voce vai corrigir um defeito neste repositorio.\n\n")
	b.WriteString("# Tarefa\n\n" + tarefa + "\n")

	if evidPath != "" {
		f, err := os.Open(evidPath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		items, err := gate.ParseEvidence(f)
		if err != nil {
			return "", err
		}
		b.WriteString("\n# Contexto\n")
		for _, e := range items {
			b.WriteString("\n")
			if e.Ref != "" {
				fmt.Fprintf(&b, "`%s`:\n", e.Ref)
			}
			fmt.Fprintf(&b, "```\n%s\n```\n", e.Text)
		}
	}
	if len(cmds) > 0 {
		b.WriteString("\n# Comandos disponiveis\n\n```bash\n" + strings.Join(cmds, "\n") + "\n```\n")
	}
	b.WriteString("\nOs testes ja estao no repositorio e sao o criterio: nao os altere. " +
		"Nao faca commit nem push. Quando terminar, responda com o que voce mudou.\n")
	return b.String(), nil
}
