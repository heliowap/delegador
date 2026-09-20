// Package agent implementa o laço do executor: pede completion, executa as
// chamadas de ferramenta através da permissão e devolve os resultados ao
// modelo, até resposta final, teto de turnos ou veto. A recusa de uma
// ferramenta é dado na conversa, nunca motivo para parar — é o modelo quem
// lê o motivo e tenta outro caminho.
package agent

import (
	"context"
	"strings"

	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

// Config é o que o laço precisa saber do job: o modelo, o teto de turnos e a
// política de permissão aplicada a cada chamada de ferramenta.
type Config struct {
	Model    string
	MaxTurns int
	Policy   tools.Policy
}

// Turn registra uma rodada completa: a mensagem do assistente e o resultado
// de cada chamada de ferramenta que ela pediu, na mesma ordem. Results é
// vazio quando o assistente respondeu sem pedir ferramentas.
type Turn struct {
	Index   int
	Message llm.Message
	Results []tools.Result
	Usage   llm.Usage
}

// Outcome é o que a cascata recebe do laço: o histórico de turnos, por que
// parou e a resposta final. Stop vale "final", "teto_de_turnos", "veto" ou
// "erro"; Veto só vem preenchido quando Stop é "veto" — saber por que parou
// importa tanto quanto saber que parou.
type Outcome struct {
	Turns []Turn
	Stop  string
	Final string
	Usage llm.Usage
	Veto  *Veto
}

// Veto é a interrupção pedida por uma Precondition: o sinal disparado, o
// trecho que o evidencia e a confiança da detecção.
type Veto struct {
	Signal      string
	Excerpt     string
	Probability float64
}

// Precondition é avaliada depois de cada turno registrado e antes do próximo
// pedido ao modelo. Devolver nil deixa o laço seguir; um *Veto o corta.
type Precondition func(turns []Turn) *Veto

// systemLine é o briefing mínimo: quem fala com o modelo é um executor com
// ferramentas, não um assistente de conversa.
const systemLine = "Voce e o executor do delegador: use as ferramentas para cumprir a tarefa na worktree e responda ao final."

// Run roda o laço. Erro de Go só sai quando Complete falha — o Outcome volta
// junto com Stop "erro" para a cascata enxergar os turnos que já aconteceram.
func Run(ctx context.Context, c *llm.Client, reg *tools.Registry, cfg Config, prompt string, pre Precondition) (Outcome, error) {
	msgs := []llm.Message{
		{Role: "system", Content: systemLine},
		{Role: "user", Content: prompt},
	}
	var out Outcome

	for {
		resp, err := c.Complete(ctx, cfg.Model, msgs, reg.Schemas())
		if err != nil {
			out.Stop = "erro"
			return out, err
		}
		msgs = append(msgs, resp.Message)

		turn := Turn{Index: len(out.Turns), Message: resp.Message, Usage: resp.Usage}
		for _, call := range resp.Message.ToolCalls {
			res := runCall(ctx, reg, call, cfg.Policy)
			turn.Results = append(turn.Results, res)
			msgs = append(msgs, llm.Message{
				Role:       "tool",
				ToolCallID: call.Args["_id"],
				Content:    res.Output,
			})
		}

		// O turno entra no Outcome antes da pre-condição: vetar sem registrar
		// perderia a evidência do que causou o veto.
		out.Turns = append(out.Turns, turn)
		out.Usage.PromptTokens += resp.Usage.PromptTokens
		out.Usage.CompletionTokens += resp.Usage.CompletionTokens
		out.Final = resp.Message.Content

		if pre != nil {
			if v := pre(out.Turns); v != nil {
				out.Stop = "veto"
				out.Veto = v
				return out, nil
			}
		}
		if len(resp.Message.ToolCalls) == 0 {
			out.Stop = "final"
			return out, nil
		}
		if len(out.Turns) >= cfg.MaxTurns {
			out.Stop = "teto_de_turnos"
			return out, nil
		}
	}
}

// runCall executa uma chamada e devolve seu Result. O id fica fora dos
// argumentos — é marcador interno, não parâmetro da ferramenta. Argumentos
// malformados não passam pelo Registry: o erro de parse já é a resposta que
// o modelo precisa para se corrigir.
func runCall(ctx context.Context, reg *tools.Registry, call tools.Call, p tools.Policy) tools.Result {
	if msg, bad := call.Args["_parse_error"]; bad {
		return tools.Result{Output: msg, IsError: true}
	}
	args := make(map[string]string, len(call.Args))
	for k, v := range call.Args {
		if strings.HasPrefix(k, "_") {
			continue
		}
		args[k] = v
	}
	return reg.Run(ctx, tools.Call{Name: call.Name, Args: args}, p)
}
