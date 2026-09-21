package compact

import (
	"context"
	"testing"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/llm"
	"github.com/heliowap/delegador/internal/tools"
)

type contaAsker struct{ n int }

func (c *contaAsker) Ask(context.Context, any, map[string]jev.Question) (jev.Result, error) {
	c.n++
	return jev.Result{}, nil
}

func turnoCom(idx int, cmds ...string) agent.Turn {
	t := agent.Turn{Index: idx}
	for _, c := range cmds {
		t.Message.ToolCalls = append(t.Message.ToolCalls,
			tools.Call{Name: "exec", Args: map[string]string{"command": c}})
		t.Results = append(t.Results, tools.Result{Output: "saida de " + c})
	}
	return t
}

// A rodada de requests no fim do run era inteiramente evitavel: o watchdog
// ja passou por cada turno durante o laco. Com as marcas colhidas la, a
// compactacao nao fala com a rede.
func TestCompactacaoComMarcasNaoPergunta(t *testing.T) {
	turns := []agent.Turn{turnoCom(0, "ls", "go build ./..."), turnoCom(1, "go test ./...")}
	m := Marcas{}
	m.Registrar(agent.Marca{Turno: 0, Chamada: 0, Necessaria: 0.1, Verbatim: 0.1})  // exploracao: sai
	m.Registrar(agent.Marca{Turno: 0, Chamada: 1, Necessaria: 0.9, Verbatim: 0.1})  // fica, saida truncada
	m.Registrar(agent.Marca{Turno: 1, Chamada: 0, Necessaria: 0.95, Verbatim: 0.9}) // fica inteira

	a := &contaAsker{}
	kept, _, err := TurnsCom(context.Background(), a, "tarefa", turns, m)
	if err != nil {
		t.Fatal(err)
	}
	if a.n != 0 {
		t.Errorf("quero zero requests com tudo marcado, tenho %d", a.n)
	}
	if len(kept) != 2 {
		t.Fatalf("quero os dois turnos, tenho %d", len(kept))
	}
	if n := len(kept[0].Message.ToolCalls); n != 1 {
		t.Errorf("turno 0: quero uma chamada apos a delecao, tenho %d", n)
	}
	if got := kept[0].Results[0].Output; got == "saida de go build ./..." {
		t.Error("turno 0: verbatim baixo deveria truncar a saida")
	}
	if got := kept[1].Results[0].Output; got != "saida de go test ./..." {
		t.Errorf("turno 1: verbatim alto preserva a saida, tenho %q", got)
	}
}

// Sem marca e sem Asker nao ha julgamento. Apagar o que ninguem julgou
// esconderia prova por omissao: o nao-julgado fica.
func TestSemMarcaESemAskerMantem(t *testing.T) {
	turns := []agent.Turn{turnoCom(0, "go test ./...")}
	kept, _, err := TurnsCom(context.Background(), nil, "tarefa", turns, Marcas{})
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || len(kept[0].Message.ToolCalls) != 1 {
		t.Errorf("interacao nao julgada foi apagada: %+v", kept)
	}
	if kept[0].Results[0].Output != "saida de go test ./..." {
		t.Error("interacao nao julgada teve a saida truncada")
	}
}

var _ = llm.Message{}
