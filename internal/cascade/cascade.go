// Package cascade decide quando uma tarefa sobe um degrau de modelo. A
// regra do spec (§6.5, §14): escalada existe só por falha provada pela
// verificação de um resultado entregue — nunca por veto (recusa de
// permissão, teto de custo), erro de execução ou teto de turnos. Nesses
// casos o modelo não falhou: o ambiente falhou, e escalar é pagar caro
// por um erro que não é do executor.
package cascade

import (
	"fmt"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/verify"
)

// Decisao é o veredito da cascata: se escala, por quê, e o degrau a
// aplicar no corte de percentil.
//
// NovoPercentil é um DELTA, não um corte absoluto: Avaliar não recebe o
// percentil atual do job, então devolve o degrau que o chamador soma a
// j.Percentil — o run aplica min(1.0, j.Percentil+d.NovoPercentil). Vale
// zero quando não escala.
type Decisao struct {
	Escala        bool
	Motivo        string
	NovoPercentil float64
}

// Config é a política da cascata.
type Config struct {
	MaxEscaladas    int     // teto de escaladas por tarefa
	DegrauPercentil float64 // quanto o corte sobe a cada escalada
}

// DefaultConfig é o padrão do spec: no máximo uma escalada por tarefa.
// O degrau de 0.25 sobe o corte um quarto da distribuição — salto
// suficiente para trocar de patamar de modelo sem pular direto ao topo.
func DefaultConfig() Config {
	return Config{MaxEscaladas: 1, DegrauPercentil: 0.25}
}

// Avaliar decide se a tarefa escala. tentativa é quantas escaladas a
// tarefa já sofreu. A ordem das checagens mantém o Motivo fiel à causa
// real: veto e parada sem entrega relatam a própria causa mesmo quando o
// teto de escaladas também estaria atingido.
//
// O chamador deve passar um Report de um verify.Run que retornou sem
// erro — uma verificação que falhou por infra não chega aqui.
func Avaliar(out agent.Outcome, rep verify.Report, tentativa int, cfg Config) Decisao {
	// Veto é o watchdog cortando o laço: nada foi entregue para verificar
	// e o sinal (recusa de permissão, teto de custo) é falha de ambiente,
	// não do modelo.
	if out.Veto != nil {
		return Decisao{Motivo: "veto: " + out.Veto.Signal}
	}

	// Sem "final" não há resultado entregue: "erro" é falha de execução e
	// "teto_de_turnos" é o laço encerrado sem resposta final — a mesma
	// categoria do veto, run terminado sem entrega, e portanto fora do
	// invariante "escalada só por falha de verificação". O Motivo diz qual
	// parada foi para o relatório explicar por que não subiu.
	if out.Stop != "final" {
		return Decisao{Motivo: fmt.Sprintf("parada %q sem resultado entregue", out.Stop)}
	}

	// Teto atingido: para e o caso vai para o humano com os dois diffs.
	if tentativa >= cfg.MaxEscaladas {
		return Decisao{Motivo: "teto de escaladas atingido; o caso vai para o humano"}
	}

	// A entrega existe: escala só o que a verificação provou falho.
	// Green() sozinha não basta — o passo de mutação tem ExpectFail, logo
	// é neutro no Green e um teste verde com mutação que não provou nada
	// continua "verde". MutationProved é o fato que separa "o teste pega
	// o defeito" de "o teste passa até com a correção desfeita".
	switch {
	case !rep.Green():
		return Decisao{Escala: true, NovoPercentil: cfg.DegrauPercentil,
			Motivo: "verificacao vermelha"}
	case !rep.MutationProved:
		// Duas causas distintas: a sonda não rodou (sem TestCmd ou erro de
		// infra — inclui o passo ausente num Report montado à mão) ou rodou
		// e o teste ficou verde com a correção desfeita. Ambas escalam —
		// entrega sem prova não completa em silêncio e o teto de uma
		// escalada limita o custo — mas o Motivo nomeia a causa real.
		if s, ok := rep.Step("mutacao"); !ok || s.Skipped {
			return Decisao{Escala: true, NovoPercentil: cfg.DegrauPercentil,
				Motivo: "sonda de mutacao nao rodou; entrega sem prova"}
		}
		return Decisao{Escala: true, NovoPercentil: cfg.DegrauPercentil,
			Motivo: "mutacao nao provou nada"}
	}
	return Decisao{}
}
