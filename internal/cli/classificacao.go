package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/route"
)

// classificacaoEmDisco e o que fica em classificacao.json: o que a rota
// DECIDIU e o que ela recebeu para decidir.
//
// Guardar as respostas cruas muda o que da para calibrar. Os oito jobs de
// 2026-09-21 rodaram descartando confianca e distribuicao, entao a unica
// forma de testar outro peso era reexecutar — e o spec chegou a declarar
// PisoTauMinimo nao calibravel por fixture justamente por isso. Com o bruto
// no disco e o desfecho de cada job rotulado, mudar um peso vira uma conta
// sobre o que ja rodou.
type classificacaoEmDisco struct {
	Em                    time.Time   `json:"em"`
	QuestionsVersion      string      `json:"questions_version"`
	Dimensao              string      `json:"dimensao"`
	Percentil             float64     `json:"percentil"`
	Volume                float64     `json:"volume"`
	ConfiancaDimensao     float64     `json:"confianca_dimensao"`
	ConfiancaComplexidade float64     `json:"confianca_complexidade"`
	Respostas             jev.Answers `json:"respostas"`
}

func gravaClassificacao(caminho string, cls route.Classificacao, stderr io.Writer) {
	b, err := json.MarshalIndent(classificacaoEmDisco{
		Em: time.Now(), QuestionsVersion: jev.QuestionsVersion,
		Dimensao: string(cls.Dimensao), Percentil: cls.Percentil, Volume: cls.Volume,
		ConfiancaDimensao: cls.ConfiancaDimensao, ConfiancaComplexidade: cls.ConfiancaComplexidade,
		Respostas: cls.Bruto,
	}, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "plan: classificacao nao serializou: %v\n", err)
		return
	}
	if err := os.WriteFile(caminho, append(b, '\n'), 0o644); err != nil {
		fmt.Fprintf(stderr, "plan: classificacao.json nao gravado: %v\n", err)
	}
}
