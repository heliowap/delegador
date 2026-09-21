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
	// Veto de ORÇAMENTO ou de AMBIENTE não escala. O teto de custo e o de
	// turnos são NOSSOS: escalar depois de estourá-los é pagar mais caro
	// por um limite que nós mesmos pusemos. Recusa de permissão é o
	// ambiente barrando, e um modelo melhor bate na mesma parede.
	if out.Veto != nil && !Estagnacao(out.Veto.Signal) {
		return Decisao{Motivo: "veto: " + out.Veto.Signal}
	}

	// Veto de ESTAGNAÇÃO escala, e isto é uma revisão do invariante
	// original (§6.5, §14), feita em 2026-09-21 com medição.
	//
	// O texto antigo dizia: "nesses casos o modelo não falhou, o ambiente
	// falhou". Isso é verdade para teto de custo e recusa de permissão, e
	// é falso para `sem_escrita`, `comando_repetido` e `sem_progresso` —
	// nesses o watchdog MEDIU o executor deixando de progredir. Doze
	// turnos sem uma escrita bem-sucedida não é o ambiente falhando: é o
	// modelo empacado, que é exatamente a situação que a cascata existe
	// para resolver.
	//
	// O que se preservou do invariante: não se escala por suposição. A
	// estagnação não decide sozinha — ela apenas deixa de curto-circuitar,
	// e o caso cai na mesma verificação que julga qualquer entrega. Run
	// que estagnou DEPOIS de deixar tudo verde e provado não escala, como
	// não escalava antes.
	//
	// Medido contra oito execuções sobre bugs reais do expr-lang/expr: a
	// cascata não disparou nenhuma vez, e nenhum dos três fracassos foi
	// por verificação vermelha de um `final` — dois foram `sem_escrita` e
	// um foi teto de custo. Com o gatilho antigo, o desenho tinha um
	// mecanismo que a realidade não alcançava.
	estagnou := out.Veto != nil && Estagnacao(out.Veto.Signal)

	// Sem "final" e sem estagnação não há resultado entregue: "erro" é
	// falha de execução e "teto_de_turnos" é o laço encerrado sem resposta
	// — orçamento nosso, mesma categoria do teto de custo.
	if !estagnou && out.Stop != "final" {
		return Decisao{Motivo: fmt.Sprintf("parada %q sem resultado entregue", out.Stop)}
	}

	// Teto atingido: para e o caso vai para o humano com os dois diffs.
	if tentativa >= cfg.MaxEscaladas {
		return Decisao{Motivo: "teto de escaladas atingido; o caso vai para o humano"}
	}

	// A entrega existe: escala só o que a verificação provou falho.
	// Green() já reprova sonda que não falhou (ExpectFail com exit 0 não é
	// verde); o que ele não vê é sonda que não rodou — pulada ou ausente,
	// o Green segue e só MutationProved separa "o teste pega o defeito"
	// de "a entrega ficou sem prova".
	// Um veto de estagnação que chegou até aqui foi seguido de verificação:
	// o Motivo nomeia os dois, porque "verificacao vermelha" sozinho
	// esconderia por que o laço parou onde parou.
	porque := func(base string) string {
		if estagnou {
			return out.Veto.Signal + " e " + base
		}
		return base
	}

	switch {
	case !rep.Green():
		return Decisao{Escala: true, NovoPercentil: cfg.DegrauPercentil,
			Motivo: porque("verificacao vermelha")}
	case !rep.MutationProved:
		// Duas causas distintas: a sonda não rodou (sem TestCmd ou erro de
		// infra — inclui o passo ausente num Report montado à mão) ou rodou
		// e o teste ficou verde com a correção desfeita. Ambas escalam —
		// entrega sem prova não completa em silêncio e o teto de uma
		// escalada limita o custo — mas o Motivo nomeia a causa real.
		if s, ok := rep.Step("mutacao"); !ok || s.Skipped {
			return Decisao{Escala: true, NovoPercentil: cfg.DegrauPercentil,
				Motivo: porque("sonda de mutacao nao rodou; entrega sem prova")}
		}
		return Decisao{Escala: true, NovoPercentil: cfg.DegrauPercentil,
			Motivo: porque("mutacao nao provou nada")}
	}
	return Decisao{}
}

// sinaisDeEstagnacao sao os vetos em que o watchdog MEDIU o executor
// deixando de progredir — ao contrario do teto de custo, do teto de turnos
// e da recusa de permissão, que são limites nossos ou do ambiente.
//
// A lista é fechada de propósito: sinal novo não vira motivo de escalada
// por omissão. Quem acrescentar um veto ao watchdog decide aqui, de forma
// explícita, em qual das duas famílias ele cai.
var sinaisDeEstagnacao = map[string]bool{
	"sem_escrita":      true,
	"comando_repetido": true,
	"sem_progresso":    true,
}

// Estagnacao diz se este sinal de veto é medição de executor empacado.
func Estagnacao(sinal string) bool { return sinaisDeEstagnacao[sinal] }
