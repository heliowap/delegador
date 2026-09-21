// precondition.go — a pré-condição entre turnos (spec §5.3 e §6.3). Sinais
// determinísticos em código, sem modelo: chamada idêntica repetida com o
// mesmo resultado, N turnos sem escrita bem-sucedida, teto de custo. E um
// sinal semântico via Jev: o noul sem_progresso, que só veta depois de
// cruzar o limiar em janelas consecutivas. Falha do Jev nunca veta — o
// laço sobrevive à rede cair.
//
// O sinal determinístico fora_do_escopo do spec §6.3 fica de fora de
// propósito: a pré-condição só recebe os turnos, nunca a política nem o
// escopo declarado. tools.Allow já nega escrita fora do escopo quando
// Policy.WritePrefixes casa com o briefing — a cobertura depende de quem
// liga o laço configurar os prefixos iguais ao escopo, não mais largos.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/tools"
)

// Asker é a parte do cliente Jev de que a pré-condição precisa.
// *jev.Client implementa; nil desliga só o sinal semântico.
type Asker interface {
	Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error)
}

// PreConfig são os limiares da pré-condição. Zero em RepeatThreshold,
// IdleTurns, ConsecutiveWindows ou CostCapUSD desliga o sinal
// correspondente.
type PreConfig struct {
	RepeatThreshold    int     // mesma chamada+resultado tantas vezes seguidas
	IdleTurns          int     // turnos seguidos sem escrita bem-sucedida
	ConsecutiveWindows int     // janelas seguidas com sem_progresso alto
	NoProgress         float64 // corte do noul sem_progresso
	CostCapUSD         float64 // custo acumulado máximo por job
}

// DefaultPreConfig traz os limiares de fábrica: repetição veta na terceira
// vez; dez turnos sem escrever é agente perdido; o semântico exige duas
// janelas consecutivas a 0.8; e o custo do job para em US$ 5 — teto
// folgado para uma correção delegada, apertado para um laço em fuga.
func DefaultPreConfig() PreConfig {
	return PreConfig{
		RepeatThreshold:    3,
		IdleTurns:          10,
		ConsecutiveWindows: 2,
		NoProgress:         0.8,
		CostCapUSD:         5.00,
	}
}

// NewPrecondition monta a pré-condição do laço. O estado das janelas
// consecutivas mora no closure — cada Precondition conta a própria
// sequência, sem estado global.
func NewPrecondition(cfg PreConfig, a Asker, costSoFar func() float64) Precondition {
	qs := map[string]jev.Question{}
	if q, ok := jev.WatchdogQuestions()["sem_progresso"]; ok {
		qs["sem_progresso"] = q
	}
	seen, consecutive := 0, 0

	return func(turns []Turn) *Veto {
		// Custo primeiro: não depende dos turnos e corta mesmo um laço que
		// ainda não teve tempo de mostrar os outros sintomas.
		if cfg.CostCapUSD > 0 && costSoFar != nil {
			if spent := costSoFar(); spent > cfg.CostCapUSD {
				return &Veto{Signal: "teto_de_custo", Probability: 1,
					Excerpt: fmt.Sprintf("custo acumulado US$ %.2f passa do teto de US$ %.2f",
						spent, cfg.CostCapUSD)}
			}
		}
		if v := repeatedCall(turns, cfg.RepeatThreshold); v != nil {
			return v
		}
		if v := noWrite(turns, cfg.IdleTurns); v != nil {
			return v
		}

		// Semântico: só com Asker, pergunta e turno novo. A janela mínima é
		// um turno — a pergunta compara o atual com os anteriores, e a
		// sequência de janelas é o que exige consecutividade.
		if a == nil || len(qs) == 0 || cfg.ConsecutiveWindows <= 0 {
			return nil
		}
		if len(turns) <= seen {
			return nil
		}
		seen = len(turns)

		state, err := windowState(turns, qs)
		if err != nil {
			return nil
		}
		res, err := a.Ask(context.Background(), state, qs)
		if err != nil {
			return nil // rede fora não é evidência de nada
		}
		p, ok := res.Answers.NoulOf("sem_progresso")
		if !ok {
			return nil // resposta sem o noul não informa; não conta janela
		}
		if p < cfg.NoProgress {
			consecutive = 0
			return nil
		}
		consecutive++
		if consecutive < cfg.ConsecutiveWindows {
			return nil
		}
		return &Veto{Signal: "sem_progresso", Probability: p,
			Excerpt: trecho(turns[len(turns)-1], 300)}
	}
}

// ---------- sinais determinísticos ----------

// signature identifica o turno pelo que ele pediu e pelo que voltou. Args
// com "_" ("_id", "_parse_error") são marcadores internos — fora da
// assinatura, senão o id único de cada chamada faria toda repetição
// parecer diferente. Marshal sobre map[string]string ordena as chaves,
// então a assinatura é estável.
func signature(t Turn) string {
	type call struct {
		Name string            `json:"name"`
		Args map[string]string `json:"args"`
	}
	calls := make([]call, 0, len(t.Message.ToolCalls))
	for _, c := range t.Message.ToolCalls {
		args := make(map[string]string, len(c.Args))
		for k, v := range c.Args {
			if !strings.HasPrefix(k, "_") {
				args[k] = v
			}
		}
		calls = append(calls, call{Name: c.Name, Args: args})
	}
	raw, _ := json.Marshal(struct {
		Calls   []call         `json:"calls"`
		Results []tools.Result `json:"results"`
	}{calls, t.Results})
	return string(raw)
}

// repeatedCall veta quando os últimos turnos repetem a mesma assinatura —
// mesmas chamadas, mesmos resultados — tantas vezes seguidas quanto o
// limiar. Mudar de abordagem e falhar diferente não é travar.
func repeatedCall(turns []Turn, threshold int) *Veto {
	if threshold <= 0 || len(turns) < threshold {
		return nil
	}
	last := turns[len(turns)-1]
	if len(last.Message.ToolCalls) == 0 {
		return nil // turno sem chamada não é repetição de comando
	}
	sig := signature(last)
	run := 0
	for i := len(turns) - 1; i >= 0 && signature(turns[i]) == sig; i-- {
		run++
	}
	if run < threshold {
		return nil
	}
	return &Veto{Signal: "comando_repetido", Probability: 1,
		Excerpt: fmt.Sprintf("%d turnos seguidos com a mesma chamada e o mesmo resultado: %s",
			run, trecho(last, 300))}
}

// noWrite veta quando nenhum dos últimos N turnos teve write_file ou
// edit_file bem-sucedida. Tentativa negada ou falha não é escrita.
func noWrite(turns []Turn, idle int) *Veto {
	if idle <= 0 || len(turns) < idle {
		return nil
	}
	for _, t := range turns[len(turns)-idle:] {
		for i, c := range t.Message.ToolCalls {
			if (c.Name == "write_file" || c.Name == "edit_file") &&
				i < len(t.Results) && !t.Results[i].IsError {
				return nil
			}
		}
	}
	return &Veto{Signal: "sem_escrita", Probability: 1,
		Excerpt: fmt.Sprintf("%d turnos sem write_file/edit_file bem-sucedida", idle)}
}

// trecho resume o turno para o veto carregar a evidência: as chamadas e
// as saídas, cortadas no tamanho dado.
func trecho(t Turn, max int) string {
	var sb strings.Builder
	if s := strings.TrimSpace(t.Message.Content); s != "" {
		sb.WriteString(s)
		sb.WriteByte('\n')
	}
	for _, c := range t.Message.ToolCalls {
		args := make(map[string]string, len(c.Args))
		for k, v := range c.Args {
			if !strings.HasPrefix(k, "_") {
				args[k] = v
			}
		}
		raw, _ := json.Marshal(args)
		fmt.Fprintf(&sb, "%s %s\n", c.Name, raw)
	}
	for _, r := range t.Results {
		sb.WriteString(r.Output)
		sb.WriteByte('\n')
	}
	s := strings.TrimSpace(sb.String())
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// ---------- janela para o Jev ----------

// digestFieldCap é o teto de cada campo de texto na janela: a pergunta
// compara turnos, não audita saída verbatim — e um resultado de 64 KB não
// pode comer o orçamento inteiro do state.
const digestFieldCap = 4000

// callDigest e turnDigest são a visão do turno que vai no state, com os
// nomes que a pergunta cita (janela.turno_atual, janela.turnos_previos).
type callDigest struct {
	Nome string            `json:"nome"`
	Args map[string]string `json:"args"`
}

type resultDigest struct {
	Texto string `json:"texto"`
	Erro  bool   `json:"erro"`
}

type turnDigest struct {
	Index      int            `json:"index"`
	Texto      string         `json:"texto,omitempty"`
	Raciocinio string         `json:"raciocinio,omitempty"`
	Chamadas   []callDigest   `json:"chamadas,omitempty"`
	Resultados []resultDigest `json:"resultados,omitempty"`
}

type windowStateView struct {
	Janela struct {
		TurnoAtual    turnDigest   `json:"turno_atual"`
		TurnosPrevios []turnDigest `json:"turnos_previos"`
	} `json:"janela"`
}

func clip(s string) string {
	if len(s) <= digestFieldCap {
		return s
	}
	return fmt.Sprintf("%s…[%d bytes cortados]", s[:digestFieldCap], len(s)-digestFieldCap)
}

func digest(t Turn) turnDigest {
	// O raciocinio entra quando o endpoint o devolve: o watchdog julga o
	// que o modelo pensou, nao so o que ele fez — um turno que repete a
	// mesma cadeia de pensamento e sem_progresso mesmo sem chamada igual.
	d := turnDigest{Index: t.Index, Texto: clip(t.Message.Content),
		Raciocinio: clip(t.Message.ReasoningContent)}
	for _, c := range t.Message.ToolCalls {
		args := make(map[string]string, len(c.Args))
		for k, v := range c.Args {
			if strings.HasPrefix(k, "_") {
				continue
			}
			args[k] = clip(v)
		}
		d.Chamadas = append(d.Chamadas, callDigest{Nome: c.Name, Args: args})
	}
	for _, r := range t.Results {
		d.Resultados = append(d.Resultados, resultDigest{Texto: clip(r.Output), Erro: r.IsError})
	}
	return d
}

// windowState monta a janela dentro do orçamento do Jev: o turno mais
// recente é o turno_atual e os anteriores entram do mais novo para o mais
// velho até onde o teto de state deixar. Turno atual que sozinho estoura
// o orçamento devolve erro — a janela sem ele não existe.
func windowState(turns []Turn, qs map[string]jev.Question) (any, error) {
	budget, err := jev.StateBudget(qs, jev.DefaultLimits())
	if err != nil {
		return nil, err
	}

	var w windowStateView
	w.Janela.TurnoAtual = digest(turns[len(turns)-1])
	fits := func() bool {
		raw, _ := json.Marshal(w)
		return jev.EstimateTokens(string(raw)) <= budget
	}
	if !fits() {
		return nil, fmt.Errorf("turno atual nao cabe no orcamento de state (%d tokens)", budget)
	}
	for i := len(turns) - 2; i >= 0; i-- {
		prev := w.Janela.TurnosPrevios
		w.Janela.TurnosPrevios = append([]turnDigest{digest(turns[i])}, prev...)
		if !fits() {
			w.Janela.TurnosPrevios = prev
			break
		}
	}
	return w, nil
}
