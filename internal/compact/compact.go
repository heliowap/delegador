// Package compact comprime o trace do executor para o relatorio. O
// mecanismo e delecao, nunca reescrita — dois nouls do Jev por interacao
// (spec §6.5, volta): a chamada importa para a conferencia? o resultado
// precisa ficar palavra por palavra? O que sobrevive, sobrevive inteiro.
package compact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/tools"
)

// KeepThreshold e o corte dos nouls de compactacao: abaixo dele a interacao
// sai do trace. 0.5 e o mesmo corte da selecao de evidencia (gate) — o noul
// e uma probabilidade e abaixo de meio o "nao" e mais provavel que o "sim".
// Resposta sem o noul conta como abaixo do corte, como em SelectEvidence.
const KeepThreshold = 0.5

// resultadoStateCap e o teto do resultado enviado ao Jev no state: o noul
// julga se a saida precisa ficar verbatim, nao a confere — e saidas de
// ferramenta ja chegam limitadas a 64 KB pelo tools.Registry, entao sem
// corte um unico resultado comeria o orcamento do state inteiro. Mesmo
// precedente do digestFieldCap do watchdog (agent/precondition.go).
const resultadoStateCap = 4000

// Asker e a parte do cliente Jev de que a compactacao precisa.
// *jev.Client implementa.
type Asker interface {
	Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error)
}

// Turns pergunta ao Jev, por interacao (chamada + resultado), se a chamada
// e necessaria a conferencia e se o resultado precisa ficar verbatim.
// Chamada dispensavel tira a interacao do turno — e o turno inteiro quando
// nao sobra nada nele; chamada mantida com resultado dispensavel deixa o
// turno com um marcador de truncamento no lugar da saida, porque truncar
// sem declarar esconderia a perda. Nada e reescrito.
func Turns(ctx context.Context, a Asker, tarefa string, turns []agent.Turn) ([]agent.Turn, jev.Usage, error) {
	var total jev.Usage
	var kept []agent.Turn

	for _, t := range turns {
		// Turno sem chamada de ferramenta e a fala do assistente (a
		// resposta final, por exemplo): nao ha interacao a julgar, fica.
		if len(t.Message.ToolCalls) == 0 {
			kept = append(kept, t)
			continue
		}

		var calls []tools.Call
		var results []tools.Result
		for i, call := range t.Message.ToolCalls {
			var res tools.Result
			hasRes := i < len(t.Results)
			if hasRes {
				res = t.Results[i]
			}

			state := map[string]any{
				"tarefa": map[string]any{"texto": tarefa},
				"interacao": map[string]any{
					"id":        call.Args["_id"],
					"chamada":   callDesc(call),
					"resultado": clipResultado(res.Output),
				},
			}
			ans, err := a.Ask(ctx, state, jev.CompactionQuestions())
			if err != nil {
				return nil, total, fmt.Errorf("compactacao: %w", err)
			}
			total.InputTokens += ans.Usage.InputTokens
			total.OutputTokens += ans.Usage.OutputTokens

			chamada, ok := ans.Answers.NoulOf("chamada_necessaria")
			if !ok || chamada < KeepThreshold {
				continue // delecao: a interacao sai do trace
			}
			verbatim, ok := ans.Answers.NoulOf("resultado_necessario_verbatim")
			if hasRes && res.Output != "" && (!ok || verbatim < KeepThreshold) {
				res.Output = truncMarker(res.Output)
			}
			calls = append(calls, call)
			if hasRes {
				results = append(results, res)
			}
		}

		// Sem chamada nenhuma, restou so a fala — e turno sem fala e sem
		// chamada nao tem o que conferir: sai junto com a interacao.
		t.Message.ToolCalls = calls
		t.Results = results
		if len(calls) > 0 || t.Message.Content != "" {
			kept = append(kept, t)
		}
	}
	return kept, total, nil
}

// AfirmaVerde mede o quanto o relatorio do executor afirma que os testes
// passaram. A comparacao com o exit code capturado acontece no render —
// codigo contra codigo nao precisa de modelo.
func AfirmaVerde(ctx context.Context, a Asker, relatorio string) (float64, jev.Usage, error) {
	state := map[string]any{"relatorio": map[string]any{"texto": relatorio}}
	res, err := a.Ask(ctx, state, jev.ReportQuestion())
	if err != nil {
		return 0, res.Usage, fmt.Errorf("relatorio: %w", err)
	}
	p, ok := res.Answers.NoulOf("relatorio_afirma_verde")
	if !ok {
		return 0, res.Usage, fmt.Errorf("relatorio: resposta sem relatorio_afirma_verde")
	}
	return p, res.Usage, nil
}

// callDesc escreve a chamada como uma linha legivel: nome + argumentos em
// JSON, sem as chaves internas "_*" — o _id viaja em Args mas e marcador
// nosso, nao parametro da ferramenta.
func callDesc(c tools.Call) string {
	args := make(map[string]string, len(c.Args))
	for k, v := range c.Args {
		if strings.HasPrefix(k, "_") {
			continue
		}
		args[k] = v
	}
	raw, _ := json.Marshal(args)
	return c.Name + " " + string(raw)
}

// truncMarker substitui a saida descartada declarando a perda: o marcador
// precisa conter "truncado" para que o trace nunca esconda o que saiu.
func truncMarker(out string) string {
	return fmt.Sprintf("[resultado truncado pela compactacao: %d bytes descartados]", len(out))
}

// clipResultado corta o resultado que vai no state no teto, declarando o
// corte — so o state e clipado; o trace mantem a saida inteira ou o
// marcador de truncamento, nada no meio.
func clipResultado(s string) string {
	if len(s) <= resultadoStateCap {
		return s
	}
	return fmt.Sprintf("%s…[%d bytes cortados]", s[:resultadoStateCap], len(s)-resultadoStateCap)
}
