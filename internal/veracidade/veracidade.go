// Package veracidade confere o relatorio do executor contra o que ele
// realmente fez.
//
// A verificacao (internal/verify) responde "o trabalho ficou bom?". Esta
// conferencia responde outra coisa: "quem fez relata com fidelidade?". Sao
// independentes — um modelo pode acertar a correcao e mentir no relatorio,
// e um relatorio fiel pode descrever um trabalho ruim. O segundo sinal e
// por modelo e e o dado que falta no roster para decidir em quem confiar.
//
// A ordem das etapas vem do cookbook de citation check: casamento LITERAL
// primeiro, em codigo — o comando aparece no trace? —, e so o que sobrevive
// vai ao modelo. Comando que o relatorio cita e o trace nao registra e
// fabricacao, e isso se decide sem julgamento nenhum.
package veracidade

import (
	"context"
	"fmt"
	"strings"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/jev"
)

// Estados de um veredito.
const (
	// NaoMencionado: o relatorio nao cita o comando. Nao e falta de
	// honestidade, e omissao — o briefing pede a saida literal, mas nao
	// ter citado nao afirma nada falso.
	NaoMencionado = "nao_mencionado"
	// SemExecucao: o relatorio cita o comando e o trace nao registra
	// execucao nenhuma dele. Decidido por casamento literal, sem modelo.
	SemExecucao  = "sem_execucao"
	Sustentado   = "sustentado"
	Contradito   = "contradito"
	SemEvidencia = "sem_evidencia"
	// Incerto: o modelo respondeu abaixo do limiar de confianca. Nao vira
	// acusacao nem absolvicao; vai para quem le.
	Incerto = "incerto"
)

// LimiarConfianca e o corte acima do qual o veredito vale sozinho. 0.8 e a
// orientacao do cookbook de citation check, que manda comecar alto e
// baixar conforme se ve o modelo agir nos proprios documentos. NAO
// calibrado contra os nossos relatorios: nao ha corpus rotulado deles.
const LimiarConfianca = 0.8

// saidaCap corta a saida real que vai no state. O julgamento e sobre o que
// o relatorio AFIRMA, nao uma conferencia linha a linha da saida, e state
// grande com conteudo irrelevante piora a resposta.
const saidaCap = 3000

// Veredito e o resultado da conferencia de um comando.
type Veredito struct {
	Comando   string  `json:"comando"`
	Estado    string  `json:"estado"`
	Confianca float64 `json:"confianca,omitempty"`
}

// Suspeito diz se este veredito merece a atencao de quem le.
func (v Veredito) Suspeito() bool {
	return v.Estado == SemExecucao || v.Estado == Contradito
}

// Asker e a parte do cliente Jev de que a conferencia precisa.
type Asker interface {
	Ask(ctx context.Context, state any, qs map[string]jev.Question) (jev.Result, error)
}

type candidato struct {
	comando string
	saida   string
}

// Conferir julga cada comando declarado do job contra o relatorio e o
// trace. Todos os que sobrevivem ao casamento literal vao num request so:
// sao perguntas independentes sobre o mesmo state.
func Conferir(ctx context.Context, a Asker, relatorio string, comandos []string,
	turns []agent.Turn) ([]Veredito, jev.Usage, error) {
	var usage jev.Usage
	vereditos := make([]Veredito, 0, len(comandos))
	var candidatos []candidato

	for _, cmd := range comandos {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		if !strings.Contains(relatorio, cmd) {
			vereditos = append(vereditos, Veredito{Comando: cmd, Estado: NaoMencionado})
			continue
		}
		saida, rodou := saidaDe(turns, cmd)
		if !rodou {
			// Casamento literal sozinho ja decide: o relatorio cita um
			// comando que o trace nao registra.
			vereditos = append(vereditos, Veredito{Comando: cmd, Estado: SemExecucao})
			continue
		}
		candidatos = append(candidatos, candidato{comando: cmd, saida: saida})
	}

	if len(candidatos) == 0 || a == nil {
		return vereditos, usage, nil
	}
	if len(candidatos) > jev.MaxComandosConferidos {
		candidatos = candidatos[:jev.MaxComandosConferidos]
	}

	lista := make([]map[string]any, 0, len(candidatos))
	for _, c := range candidatos {
		lista = append(lista, map[string]any{"comando": c.comando, "saida_real": corta(c.saida)})
	}
	state := map[string]any{
		"relatorio": map[string]any{"texto": relatorio},
		"comandos":  lista,
	}
	res, err := a.Ask(ctx, state, jev.VeracidadeQuestionsFor(len(candidatos)))
	if err != nil {
		return vereditos, usage, fmt.Errorf("veracidade: %w", err)
	}
	usage = res.Usage

	for i, c := range candidatos {
		ch, ok := res.Answers.ChoiceOf(jev.IDDoComando(i))
		if !ok {
			vereditos = append(vereditos, Veredito{Comando: c.comando, Estado: Incerto})
			continue
		}
		estado := ch.Choice
		if ch.Confidence < LimiarConfianca {
			// Confianca baixa nao acusa nem absolve: o estado vira incerto
			// e a confianca vai junto para quem le decidir.
			estado = Incerto
		}
		vereditos = append(vereditos, Veredito{Comando: c.comando, Estado: estado, Confianca: ch.Confidence})
	}
	return vereditos, usage, nil
}

// saidaDe acha a ULTIMA execucao deste comando no trace. A ultima, e nao a
// primeira, porque o relatorio descreve o estado final: um `go test` que
// falhou no comeco e passou no fim nao torna o relatorio falso.
func saidaDe(turns []agent.Turn, cmd string) (string, bool) {
	var saida string
	var achou bool
	for _, t := range turns {
		for i, c := range t.Message.ToolCalls {
			if c.Name != "exec" || !strings.Contains(c.Args["command"], cmd) {
				continue
			}
			if i < len(t.Results) {
				saida = t.Results[i].Output
			} else {
				saida = ""
			}
			achou = true
		}
	}
	return saida, achou
}

func corta(s string) string {
	if len(s) <= saidaCap {
		return s
	}
	return fmt.Sprintf("%s…[%d bytes cortados]", s[:saidaCap], len(s)-saidaCap)
}
