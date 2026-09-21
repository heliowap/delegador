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
}

// OrcamentoBase e o orcamento de fabrica, ancorado no volume nivel 1.
// agent.DefaultPreConfig traz o mesmo teto de custo como fallback de quem
// roda sem plan; os dois numeros devem andar juntos.
func OrcamentoBase() Orcamento { return Orcamento{Turnos: 30, TetoUSD: 5.00} }

// Limites protegem contra resposta degenerada do modelo: nem 0 turnos, nem
// um laco que roda ate a conta acabar.
const (
	turnosMin  = 6
	turnosMax  = 200
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

	return Orcamento{
		Turnos:  clampInt(turnos, turnosMin, turnosMax),
		TetoUSD: clampFloat(teto, tetoUSDMin, tetoUSDMax),
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
