package devin

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolInteraction e um par chamada/resultado dentro de um turno.
type ToolInteraction struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Status string `json:"status"`
}

// Turn e um turno do agente.
type Turn struct {
	Index int               `json:"index"`
	Text  string            `json:"text"`
	Tools []ToolInteraction `json:"tools"`
}

// Summary rende o turno de forma compacta, para virar state do Jev.
// Nao interpreta o conteudo: o texto do Devin e dado, nunca instrucao.
func (t Turn) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "turno %d: %s\n", t.Index, t.Text)
	for _, tool := range t.Tools {
		fmt.Fprintf(&b, "  %s(%s) -> [%s] %s\n", tool.Name, tool.Input, tool.Status, tool.Output)
	}
	return b.String()
}

// ParseATIF le o export do devin. Campos desconhecidos sao ignorados, para
// sobreviver a mudancas de formato; entrada vazia devolve zero turnos sem
// erro, porque o arquivo pode ainda nao ter sido escrito.
func ParseATIF(raw []byte) ([]Turn, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var doc struct {
		Turns []Turn `json:"turns"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("devin: export ATIF invalido: %w", err)
	}
	return doc.Turns, nil
}
