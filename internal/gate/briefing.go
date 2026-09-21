package gate

import (
	"fmt"
	"strings"
)

// BriefingLimits sao os dados operacionais que o briefing precisa carregar.
type BriefingLimits struct {
	TestCmd         string
	SuiteCmd        string
	LintCmd         string
	VenvPath        string
	NodeModulesPath string
	Forbidden       []string

	// TestesJaEscritos muda a ordem de trabalho. O template sempre mandava
	// "escreva primeiro o teste", e quando os testes ja estao no disco isso
	// contradiz o texto da tarefa — medido em 2026-09-21: o modelo obedeceu
	// ao template, escreveu um teste novo e nunca chegou a implementacao.
	// A disciplina do vermelho fica nos dois casos; o que muda e quem
	// escreve o teste.
	TestesJaEscritos bool
	// ArquivosDeTeste sao os caminhos a citar quando TestesJaEscritos.
	ArquivosDeTeste []string
}

// BuildBriefing monta o briefing a partir do template fixo e das evidencias
// que sobreviveram. Evidencia nunca e reescrita: so entra ou nao entra.
func BuildBriefing(task string, kept []Evidence, l BriefingLimits) string {
	var b strings.Builder

	b.WriteString("# Tarefa\n\n")
	b.WriteString(task)
	b.WriteString("\n\n")

	if refs := refsOf(kept); len(refs) > 0 {
		fmt.Fprintf(&b, "Localizacao: %s\n\n", strings.Join(refs, ", "))
	}

	writeSection(&b, "## Contrato violado", kept, "fonte")
	writeSection(&b, "## Onde esta o defeito", kept, "trecho")
	writeSection(&b, "## Erro observado", kept, "erro")
	writeSection(&b, "## Comandos ja executados", kept, "comando")

	// Kind fora dos quatro conhecidos nao some: a selecao ja disse que o
	// item e necessario, e contar no EvidenceOut sem entrar no briefing
	// perderia dado e mentiria a conta. Caindo aqui ele fica visivel.
	known := map[string]bool{"fonte": true, "trecho": true, "erro": true, "comando": true}
	var rest []Evidence
	for _, e := range kept {
		if !known[e.Kind] {
			rest = append(rest, e)
		}
	}
	if len(rest) > 0 {
		b.WriteString("## Outras evidencias (kind nao reconhecido)\n\n")
		for _, e := range rest {
			if e.Ref != "" {
				fmt.Fprintf(&b, "`%s`:\n", e.Ref)
			}
			fmt.Fprintf(&b, "- kind `%s`:\n```\n%s\n```\n\n", e.Kind, e.Text)
		}
	}

	b.WriteString("## Ordem de trabalho\n\n")
	if l.TestesJaEscritos {
		if len(l.ArquivosDeTeste) > 0 {
			fmt.Fprintf(&b, "Os testes **ja estao escritos** em %s. Eles sao o criterio de aceite: "+
				"nao os altere, nao escreva testes novos.\n\n", strings.Join(l.ArquivosDeTeste, ", "))
		} else {
			b.WriteString("Os testes **ja estao escritos** e sao o criterio de aceite: " +
				"nao os altere, nao escreva testes novos.\n\n")
		}
		b.WriteString("1. Leia os testes para entender o contrato exigido.\n")
		b.WriteString("2. Rode-os e **confirme o vermelho** antes de escrever a implementacao. ")
		b.WriteString("Cole a saida da falha no relatorio.\n")
		b.WriteString("3. Escreva a implementacao que os faz passar.\n")
		b.WriteString("4. Rode de novo e confirme o verde.\n\n")
	} else {
		b.WriteString("1. Escreva primeiro o teste que expoe este defeito.\n")
		b.WriteString("2. Rode o teste e **confirme o vermelho** antes de tocar no codigo de producao. ")
		b.WriteString("Cole a saida da falha no relatorio.\n")
		b.WriteString("3. So entao corrija.\n")
		b.WriteString("4. Rode de novo e confirme o verde.\n\n")
	}

	b.WriteString("## Comandos\n\n```bash\n")
	if l.VenvPath != "" {
		fmt.Fprintf(&b, "# interpretador desta tarefa (a worktree nao tem ambiente proprio)\n%s/bin/python -V\n", l.VenvPath)
	}
	if l.NodeModulesPath != "" {
		fmt.Fprintf(&b, "# dependencias: %s\n", l.NodeModulesPath)
	}
	if l.TestCmd != "" {
		fmt.Fprintf(&b, "%s\n", l.TestCmd)
	}
	if l.SuiteCmd != "" {
		fmt.Fprintf(&b, "%s\n", l.SuiteCmd)
	}
	if l.LintCmd != "" {
		fmt.Fprintf(&b, "%s\n", l.LintCmd)
	}
	b.WriteString("```\n\n")

	b.WriteString("## Limites\n\n")
	b.WriteString("- Nao faca commit e nao faca push.\n")
	b.WriteString("- Nao acesse a rede.\n")
	b.WriteString("- Trabalhe apenas nesta worktree.\n")
	for _, f := range l.Forbidden {
		fmt.Fprintf(&b, "- Nao altere nada em `%s`.\n", f)
	}
	b.WriteString("\n")

	b.WriteString("## Relatorio final\n\n")
	b.WriteString("Ao terminar, responda com: ")
	if l.TestesJaEscritos {
		// Pedir "o teste que voce escreveu" aqui contradiz o "nao escreva
		// testes novos" da ordem de trabalho — o template falava com duas
		// vozes, e o gate sentia a contradicao.
		b.WriteString("quais testes passaram a valer, ")
	} else {
		b.WriteString("o teste que voce escreveu, ")
	}
	b.WriteString("a mudanca feita arquivo por arquivo, e a saida literal dos comandos que voce executou, ")
	b.WriteString("incluindo a do vermelho inicial.\n")

	return b.String()
}

func refsOf(items []Evidence) []string {
	var refs []string
	for _, e := range items {
		if e.Ref != "" {
			refs = append(refs, e.Ref)
		}
	}
	return refs
}

func writeSection(b *strings.Builder, title string, items []Evidence, kind string) {
	var sel []Evidence
	for _, e := range items {
		if e.Kind == kind {
			sel = append(sel, e)
		}
	}
	if len(sel) == 0 {
		return
	}
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, e := range sel {
		if e.Ref != "" {
			fmt.Fprintf(b, "`%s`:\n", e.Ref)
		}
		b.WriteString("```\n")
		b.WriteString(e.Text)
		if !strings.HasSuffix(e.Text, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
	}
}
