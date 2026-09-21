package roster

import "time"

// Conta e o pote de onde um modelo e pago. Existe porque o mesmo modelo
// aparece em varios canais do proxy — glm-5.3-flash esta em cinco — com a
// MESMA qualidade e escassez completamente diferente: um vem de cota de
// assinatura que renova, outro de saldo em dolar que nao volta, outro de
// uma promocao com data para acabar.
//
// Dolar nao e o eixo. O que o operador esta administrando e cota, e a cota
// mais disputada e a do orquestrador: cada token que o executor gasta nela
// sai da mesma janela em que a sessao que delegou esta rodando.
type Conta struct {
	Nome string
	// Escassez: "promocao" (gratis ate uma data), "assinatura" (cota que
	// renova) ou "pre_pago" (saldo em dolar, nao renova).
	Escassez string
	// Aperto e quanto a cota de assinatura aperta na pratica: "baixo",
	// "medio" ou "alto". E observacao de uso, nao numero de fornecedor —
	// uma franquia semanal que acaba em dois dias e "alto" mesmo sendo
	// nominalmente generosa.
	Aperto string
	// Orquestrador marca a conta em que a sessao que delega esta rodando.
	// Ela vai por ultimo sempre: gastar cota do orquestrador com execucao
	// e tirar contexto de quem esta conduzindo o trabalho.
	Orquestrador bool
	// Ate e quando a promocao expira. Promocao vencida perde a preferencia
	// — o preco dela mudou e quem declara precisa dizer para quanto.
	Ate time.Time
	// Prioridade explicita, quando o operador quer mandar na ordem. Zero
	// significa "derive de escassez, aperto e papel".
	Prioridade int
}

// Ordens derivadas. Menor vence. Os intervalos sao espacados para caber
// prioridade explicita entre dois degraus sem renumerar os outros.
const (
	OrdemPromocao         = 10
	OrdemAssinaturaFolga  = 20
	OrdemPrePago          = 30
	OrdemAssinaturaMedia  = 40
	OrdemAssinaturaAperto = 50
	OrdemNaoDeclarada     = 60
	OrdemOrquestrador     = 90
)

// Ordem e a preferencia desta conta. Menor vence.
//
// A sequencia responde a uma pergunta so: o que se gasta primeiro e o que
// se repoe mais facil? Promocao nao custa nada e tem prazo, entao usa-se
// enquanto dura. Assinatura folgada renova sozinha. Saldo pre-pago e real
// e nao volta. Assinatura apertada e o que falta antes do fim da semana. E
// a conta do orquestrador vai por ultimo, sempre.
//
// Conta nao declarada nao vira preferida: fica entre a assinatura apertada
// e o orquestrador. Omitir a conta de um modelo nao pode ser um atalho para
// ele ganhar a rota.
func (c Conta) Ordem() int {
	if c.Prioridade > 0 {
		return c.Prioridade
	}
	if c.Orquestrador {
		return OrdemOrquestrador
	}
	switch c.Escassez {
	case "promocao":
		if c.Vencida(time.Now()) {
			return OrdemNaoDeclarada
		}
		return OrdemPromocao
	case "pre_pago":
		return OrdemPrePago
	case "assinatura":
		switch c.Aperto {
		case "baixo":
			return OrdemAssinaturaFolga
		case "alto":
			return OrdemAssinaturaAperto
		default:
			return OrdemAssinaturaMedia
		}
	}
	return OrdemNaoDeclarada
}

// Vencida diz se a promocao ja passou da data. Sem data, nao vence: quem
// declarou promocao sem prazo assumiu o risco de reve-la a mao.
func (c Conta) Vencida(agora time.Time) bool {
	return c.Escassez == "promocao" && !c.Ate.IsZero() && agora.After(c.Ate)
}
