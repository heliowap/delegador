package cli

import "fmt"

// ExitInterrompido e o codigo de saida quando o laco parou por VETO do
// watchdog — teto de custo, ociosidade, comando repetido — e nao por
// verificacao vermelha.
//
// Medido em 2026-09-21 na issue expr-lang/expr#823: o modelo deixou os
// quatro passos verdes, a sonda de mutacao provou o teste, e so entao o
// custo passou do teto. O run devolveu 1, indistinguivel de uma entrega
// que a verificacao reprovou, e quem lia o placar contava a tarefa como
// falha do modelo. Sao coisas diferentes: uma se resolve escalando, a
// outra se resolve levantando o teto ou aceitando o diff.
//
// 2 ja e ExitUsage e 3 ja e ExitRejected; por isso 4.
const ExitInterrompido = 4

// codigoDeSaida separa entrega reprovada de laco cortado pelo ambiente.
func codigoDeSaida(concluido, vetado bool) int {
	switch {
	case concluido:
		return 0
	case vetado:
		return ExitInterrompido
	default:
		return 1
	}
}

// avisoVetoComVerde devolve a linha que reconcilia cabecalho e corpo quando
// o veto chegou DEPOIS de a entrega ficar boa. Sem ela o relatorio diz
// "CANCELADO pelo watchdog" no topo e "veredito: verde" tres linhas abaixo,
// e o leitor reconcilia sozinho. Vazio quando nao ha o que reconciliar.
func avisoVetoComVerde(vetado, verde, provado bool, sinal string) string {
	if !vetado || !verde || !provado {
		return ""
	}
	return fmt.Sprintf("atencao:  a worktree esta verde e a sonda de mutacao provou o teste; "+
		"o veto (%s) veio depois da entrega.\n"+
		"          falta so o relatorio final do modelo: retome o job para obte-lo, "+
		"ou leia o diff e decida.\n", sinal)
}
