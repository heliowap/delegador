package route

import "math"

// Orcamento sao os tetos de um laco: quantos turnos e quanto dinheiro.
//
// Existe porque complexidade e volume nao sao o mesmo eixo. A complexidade
// escolhe QUAL modelo, pelo percentil de corte no roster; o volume escolhe
// QUANTO ele pode gastar chegando la. Uma migracao mecanica em trinta
// arquivos e trivial e cara; uma linha sutil de concorrencia e dificil e
// barata. Rotear as duas pelo mesmo numero errava as duas.
type Orcamento struct {
	Turnos  int
	TetoUSD float64
	// TurnosOciosos e quantos turnos seguidos sem escrita sao tolerados antes
	// do veto. Escala com o volume pelo mesmo motivo que os outros dois:
	// autorar dois arquivos a partir de um contrato exige ler bastante antes
	// da primeira escrita, e matar no turno 10 de 60 e falso positivo.
	TurnosOciosos int
}

// OrcamentoBase e o orcamento de fabrica, ancorado no volume nivel 1.
// agent.DefaultPreConfig traz o mesmo teto de custo como fallback de quem
// roda sem plan; os dois numeros devem andar juntos.
func OrcamentoBase() Orcamento {
	return Orcamento{Turnos: 30, TetoUSD: 5.00, TurnosOciosos: 10}
}

// Limites protegem contra resposta degenerada do modelo: nem 0 turnos, nem
// um laco que roda ate a conta acabar.
const (
	turnosMin  = 6
	turnosMax  = 200
	ociososMin = 10
	ociososMax = 60
	tetoUSDMin = 0.10
	tetoUSDMax = 50.00
)

// OrcamentoPara escala o orcamento base pelo volume medido. A ancora e o
// nivel 1 (poucos pontos que andam juntos): ali o orcamento e exatamente o
// base. Cada nivel acima dobra, cada nivel abaixo divide por dois — a escala
// e continua, porque o Score devolve posicao ponderada e nao degrau.
func OrcamentoPara(volume float64, base Orcamento) Orcamento {
	if volume < 0 {
		volume = 0
	}
	fator := math.Pow(2, volume-1)

	turnos := int(math.Round(float64(base.Turnos) * fator))
	teto := base.TetoUSD * fator

	// Volume so AUMENTA a tolerancia de ociosidade, nunca reduz. O custo de
	// preparacao e praticamente constante — ler o contrato, ler os testes e
	// rodar o teste para ver o vermelho, que o proprio briefing prescreve e
	// que e exec, nao write. O que cresce com o volume e o trabalho total,
	// nao o preambulo.
	//
	// Medido em 2026-09-21: numa tarefa de volume 0.04 a escala para baixo
	// deu 5 turnos ociosos, e o modelo foi vetado depois de ler o stub, ler
	// o teste e confirmar o vermelho. Tres acoes legitimas, nenhuma escrita.
	ociosos := base.TurnosOciosos
	if escalado := int(math.Round(float64(base.TurnosOciosos) * fator)); escalado > ociosos {
		ociosos = escalado
	}
	return Orcamento{
		Turnos:        clampInt(turnos, turnosMin, turnosMax),
		TetoUSD:       clampFloat(teto, tetoUSDMin, tetoUSDMax),
		TurnosOciosos: clampInt(ociosos, ociososMin, ociososMax),
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
