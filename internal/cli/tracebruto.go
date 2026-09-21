package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/heliowap/delegador/internal/agent"
)

// maxTrechoBruto corta cada campo do trace bruto. O arquivo existe para
// auditar o VETO — quais ferramentas foram chamadas, e se o resultado foi
// erro — e nao para guardar o conteudo dos arquivos lidos.
const maxTrechoBruto = 2000

// turnoBruto e uma linha do turns.jsonl.
type turnoBruto struct {
	Turno      int              `json:"turno"`
	Fala       string           `json:"fala,omitempty"`
	Chamadas   []chamadaBruta   `json:"chamadas,omitempty"`
	Resultados []resultadoBruto `json:"resultados,omitempty"`
}

type chamadaBruta struct {
	Nome string            `json:"nome"`
	Args map[string]string `json:"args,omitempty"`
}

type resultadoBruto struct {
	Erro  bool   `json:"erro"`
	Saida string `json:"saida,omitempty"`
}

// gravaTraceBruto persiste o historico de turnos ANTES da compactacao.
//
// Medido em 2026-09-21 nas issues expr-lang/expr#836 e #685: os dois runs
// pararam por veto `sem_escrita` depois de 10 e 12 turnos, e o unico
// registro que sobrou foi o trace compactado — dois turnos em cada. A
// compactacao seleciona o que importa para a ENTREGA, e um run vetado nao
// tem entrega: ela descarta exatamente o que responderia a pergunta do
// operador, que e se o modelo estava lendo para entender ou tentando
// escrever e falhando. `sem_escrita` nao distingue os dois: o contador so
// zera com escrita bem-sucedida.
func gravaTraceBruto(caminho string, turns []agent.Turn, stderr io.Writer) {
	f, err := os.Create(caminho)
	if err != nil {
		fmt.Fprintf(stderr, "run: trace bruto nao gravado: %v\n", err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, t := range turns {
		tb := turnoBruto{Turno: t.Index, Fala: corta(t.Message.Content)}
		for _, c := range t.Message.ToolCalls {
			args := make(map[string]string, len(c.Args))
			for k, v := range c.Args {
				args[k] = corta(v)
			}
			tb.Chamadas = append(tb.Chamadas, chamadaBruta{Nome: c.Name, Args: args})
		}
		for _, r := range t.Results {
			tb.Resultados = append(tb.Resultados, resultadoBruto{Erro: r.IsError, Saida: corta(r.Output)})
		}
		if err := enc.Encode(tb); err != nil {
			fmt.Fprintf(stderr, "run: trace bruto truncado: %v\n", err)
			return
		}
	}
}

func corta(s string) string {
	if len(s) <= maxTrechoBruto {
		return s
	}
	return s[:maxTrechoBruto] + fmt.Sprintf("\n[...%d bytes cortados]", len(s)-maxTrechoBruto)
}
