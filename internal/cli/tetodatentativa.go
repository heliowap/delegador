package cli

// tetoDaTentativa devolve o teto de custo que vale NESTA tentativa.
//
// O contador de custo do watchdog é acumulado por job, não por tentativa:
// ele soma o ledger inteiro mais os turnos em curso. Com um teto fixo, a
// primeira tentativa que gasta a maior parte do orçamento faz a escalada
// nascer morta — o modelo novo leva veto de custo antes do primeiro turno,
// e `MaxEscaladas: 1` vira promessa que o código não cumpre.
//
// Cada tentativa recebe o teto de novo. O total continua limitado, porque
// o número de tentativas é: teto × (1 + MaxEscaladas).
//
// A extensão é em dólares e igual para todo degrau, o que significa que um
// modelo mais caro recebe menos turnos pelo mesmo teto. É deliberado: o
// teto é o número de quem opera, e quem monta o roster já escolheu que
// degraus existem. Se isso se mostrar apertado, o caminho é o teto virar
// tokens — que é a unidade em que a rota já mede trabalho (§16.1) — e não
// um multiplicador por preço de modelo.
func tetoDaTentativa(base float64, escaladas int) float64 {
	if base <= 0 {
		return base
	}
	if escaladas < 0 {
		escaladas = 0
	}
	return base * float64(escaladas+1)
}
