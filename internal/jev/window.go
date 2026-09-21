package jev

import (
	"encoding/json"
	"fmt"
)

// Limits sao os tetos do Jev por requisicao, em tokens.
type Limits struct {
	Total            int // state + todas as perguntas
	StatePlusLongest int // state + a maior pergunta
}

// DefaultLimits traz os valores publicados para jev-latest.
func DefaultLimits() Limits { return Limits{Total: 64_000, StatePlusLongest: 32_000} }

// envelopeSlack reserva espaco para o JSON em volta do conteudo.
const envelopeSlack = 512

// EstimateTokens estima tokens contando caracteres. E aproximacao, nao
// tokenizacao: erra para mais em texto denso e para menos em ASCII simples.
// Por isso todo orcamento aqui deixa folga.
func EstimateTokens(s string) int { return (len(s) + 3) / 4 }

func questionTokens(q Question) int {
	raw, err := json.Marshal(q)
	if err != nil {
		return 0
	}
	return EstimateTokens(string(raw))
}

// StateBudget devolve quantos tokens sobram para o state, respeitando os dois
// tetos ao mesmo tempo: o total e o de state mais a maior pergunta.
func StateBudget(qs map[string]Question, l Limits) (int, error) {
	var sum, longest int
	for _, q := range qs {
		n := questionTokens(q)
		sum += n
		if n > longest {
			longest = n
		}
	}
	byTotal := l.Total - sum - envelopeSlack
	byLongest := l.StatePlusLongest - longest - envelopeSlack

	budget := byTotal
	if byLongest < budget {
		budget = byLongest
	}
	if budget <= 0 {
		return 0, fmt.Errorf("jev: perguntas ocupam %d tokens e nao deixam espaco para o state (tetos %d/%d)",
			sum, l.Total, l.StatePlusLongest)
	}
	return budget, nil
}

// Chunk e um grupo de itens que cabe num orcamento.
type Chunk[T any] struct{ Items []T }

// SplitByBudget agrupa itens em chunks que cabem no orcamento, preservando a
// ordem. Item que sozinho nao cabe e erro: truncar aqui esconderia perda.
func SplitByBudget[T any](items []T, size func(T) int, budget int) ([]Chunk[T], error) {
	var chunks []Chunk[T]
	var cur Chunk[T]
	var used int

	for i, it := range items {
		n := size(it)
		if n > budget {
			return nil, fmt.Errorf("jev: item %d ocupa %d tokens e o orcamento e %d", i, n, budget)
		}
		if used+n > budget && len(cur.Items) > 0 {
			chunks = append(chunks, cur)
			cur, used = Chunk[T]{}, 0
		}
		cur.Items = append(cur.Items, it)
		used += n
	}
	if len(cur.Items) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks, nil
}
