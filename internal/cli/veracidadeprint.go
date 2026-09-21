package cli

import (
	"fmt"
	"io"

	"github.com/heliowap/delegador/internal/veracidade"
)

// escreveVeracidade imprime a conferencia do relatorio contra o trace.
//
// So imprime quando ha o que dizer, e o que dizer e o que DESVIA: comando
// citado sem execucao e afirmacao contradita pela saida real. Um relatorio
// inteiro sustentado nao vira parede de texto — vira uma linha.
func escreveVeracidade(w io.Writer, vs []veracidade.Veredito) {
	if len(vs) == 0 {
		return
	}
	var suspeitos, incertos, ok int
	for _, v := range vs {
		switch {
		case v.Suspeito():
			suspeitos++
		case v.Estado == veracidade.Incerto:
			incertos++
		case v.Estado == veracidade.Sustentado:
			ok++
		}
	}
	if suspeitos == 0 && incertos == 0 {
		if ok > 0 {
			fmt.Fprintf(w, "relato:   %d de %d comandos conferem com o trace\n", ok, len(vs))
		}
		return
	}
	fmt.Fprintln(w, "relato:   o relatorio do executor DIVERGE do que o trace registra")
	for _, v := range vs {
		switch v.Estado {
		case veracidade.SemExecucao:
			fmt.Fprintf(w, "  %s — citado no relatorio, nenhuma execucao no trace\n", v.Comando)
		case veracidade.Contradito:
			fmt.Fprintf(w, "  %s — o relatorio contradiz a saida real (confianca %.2f)\n",
				v.Comando, v.Confianca)
		case veracidade.Incerto:
			fmt.Fprintf(w, "  %s — conferencia inconclusiva (confianca %.2f)\n", v.Comando, v.Confianca)
		}
	}
}
