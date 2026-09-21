package jev

import "fmt"

// As perguntas do spec §6. IDs em portugues, como especificado; o ID nao e
// enviado ao modelo, entao cada pergunta carrega o significado inteiro no
// texto. Cada conjunto tem fixture rotulada correspondente em evals/.

// QuestionsVersion muda sempre que o texto de uma pergunta muda, para que a
// auditoria em jev.jsonl continue interpretavel depois de uma recalibragem.
const QuestionsVersion = "2"

// DelegabilityQuestions julga se a tarefa deve ser delegada (spec §6.1).
func DelegabilityQuestions() map[string]Question {
	return map[string]Question{
		"tipo_de_tarefa": Choice{
			Instructions: "Classifique o trabalho descrito em `tarefa.texto` pelo tipo de entrega que ele pede a um agente de codigo que trabalha sozinho num repositorio.",
			Criteria: map[string]string{
				"correcao_com_teste":      "Corrigir um comportamento errado num codigo que ja existe, com um teste que prove a correcao.",
				"revisao_somente_leitura": "Ler codigo e apontar problemas, sem alterar arquivo nenhum.",
				"investigacao":            "Descobrir a causa de um sintoma, ou levantar como algo funciona, sem que se saiba de antemao qual sera a correcao.",
				"nao_delegavel":           "Exige decidir o desenho, negociar requisito, tocar credencial, segredo, dado pessoal ou deploy, ou depende de contexto que nao esta escrito.",
			},
		},
		"defeito_unico": Noul{
			Instructions: "A tarefa em `tarefa.texto` trata de um unico defeito ou comportamento a corrigir.",
			Criteria: &NoulCriteria{
				True:  "Um defeito, ou dois defeitos que sao a mesma causa vista de dois lugares.",
				False: "Dois ou mais temas independentes, que poderiam virar correcoes separadas sem prejuizo.",
			},
		},
		"desenho_em_aberto": Noul{
			Instructions: "Executar a tarefa em `tarefa.texto` exige escolher entre desenhos possiveis, sem que a tarefa diga qual escolher.",
			Criteria: &NoulCriteria{
				True:  "Quem for executar precisa decidir arquitetura, contrato de API, formato de dado ou politica que a tarefa deixou em aberto.",
				False: "O que fazer esta determinado; restam apenas decisoes locais de implementacao.",
			},
		},
		"cruza_pacotes": Noul{
			Instructions: "A correcao descrita em `tarefa.texto` exige mudancas coordenadas em mais de um pacote ou modulo, considerando `repo.pacotes_atingidos`.",
			Criteria: &NoulCriteria{
				True:  "Alterar um pacote sozinho deixaria o sistema inconsistente ou quebrado.",
				False: "A mudanca cabe num pacote, mesmo que outros o consumam sem alteracao.",
			},
		},
		"toca_sensivel": Noul{
			Instructions: "A tarefa em `tarefa.texto` envolve credencial, segredo, dado pessoal, configuracao de deploy ou infraestrutura de producao.",
			Criteria: &NoulCriteria{
				True:  "Executa-la implica ler, escrever ou mover esse tipo de material, ou alterar o que vai para producao.",
				False: "Fica restrita a codigo de aplicacao, teste ou documentacao.",
			},
		},
		"criterio_de_pronto": Noul{
			Instructions: "Julgue `tarefa.texto` E `briefing.texto` COMO UM SO ARTEFATO: e isso que quem executa vai receber. Existe ali um criterio objetivo de termino? Se a tarefa nao diz mas o briefing diz, existe. Nao julgue os dois separadamente nem exija que o criterio esteja no texto da tarefa.",
			Criteria: &NoulCriteria{
				True:  "Ha um criterio objetivo e verificavel em qualquer um dos dois textos: um teste que precisa passar, um comando com saida esperada, um comportamento observavel. Pedir para escrever um teste que exponha o defeito e depois faze-lo passar E um criterio de termino: o verde do teste e a prova.",
				False: "O criterio e subjetivo, implicito, ou simplesmente ausente.",
			},
		},
	}
}

// BriefingQuestions confere o briefing montado (spec §6.2). Cada pergunta
// corresponde a um item do protocolo medido em uso real.
func BriefingQuestions() map[string]Question {
	return map[string]Question{
		"aponta_arquivo_linha": Noul{
			Instructions: "O briefing localiza o defeito citando arquivo e linha, e nao apenas descrevendo o sintoma.",
			Criteria: &NoulCriteria{
				True:  "Ha ao menos uma referencia no formato caminho/arquivo:linha apontando onde esta o problema.",
				False: "So ha descricao do sintoma, nome de funcao solto, ou nenhuma localizacao.",
			},
		},
		"cita_fonte_do_contrato": Noul{
			Instructions: "O briefing diz qual contrato foi violado e nomeia a fonte desse contrato.",
			Criteria: &NoulCriteria{
				True:  "Cita um ADR, especificacao, documento, issue ou comentario normativo como a fonte da regra que o codigo desrespeita.",
				False: "Afirma que algo esta errado sem dizer com base em que regra, ou apenas apela ao bom senso.",
			},
		},
		"pede_teste_antes_da_correcao": Noul{
			Instructions: "O briefing garante que o teste prove a correcao, exigindo ver o vermelho ANTES de escrever o codigo de producao. O que importa e a ordem, nao quem escreveu o teste: um briefing onde os testes ja estao no disco e que manda roda-los e confirmar a falha antes de implementar cumpre isso tanto quanto um que manda escrever o teste primeiro.",
			Criteria: &NoulCriteria{
				True:  "O briefing exige confirmar o vermelho antes de tocar no codigo de producao, em qualquer uma das duas formas: mandando escrever o teste primeiro e rodar, ou apontando testes que ja existem e mandando roda-los para ver a falha antes de implementar.",
				False: "Nao ha exigencia de ver o vermelho antes: pede so a correcao, pede teste sem pedir a falha, deixa a ordem em aberto, ou admite escrever o teste depois do codigo.",
			},
		},
		"comandos_copiaveis": Noul{
			Instructions: "O briefing traz os comandos exatos de teste e de lint, prontos para copiar e colar, incluindo o interpretador ou caminho de ambiente quando necessario.",
			Criteria: &NoulCriteria{
				True:  "Os comandos estao escritos por extenso e podem ser colados num shell sem adivinhacao.",
				False: "Diz apenas 'rode os testes', ou descreve o comando sem escreve-lo, ou omite como acessar o ambiente.",
			},
		},
		"limites_explicitos": Noul{
			Instructions: "O briefing lista o que nao pode ser feito: commit, push, acesso a rede, pastas proibidas, arquivos que nao podem mudar.",
			Criteria: &NoulCriteria{
				True:  "Ha uma lista explicita de proibicoes ou de escopo permitido.",
				False: "Nao ha restricao escrita, ou ela fica subentendida.",
			},
		},
		"pede_relatorio": Noul{
			Instructions: "O briefing pede um relatorio final com o teste escrito, a mudanca por arquivo e a saida dos comandos executados.",
			Criteria: &NoulCriteria{
				True:  "Pede explicitamente esses tres elementos, ou equivalentes que permitam conferir o trabalho sem reler o codigo.",
				False: "Nao pede relatorio, ou pede so um resumo em prosa.",
			},
		},
	}
}

// RouteQuestions escolhem modo de permissao e esforco (spec §6.4).
// dangerous nao aparece: nunca e escolhido automaticamente.
func RouteQuestions() map[string]Question {
	return map[string]Question{
		"permissao": Choice{
			Instructions: "Escolha o nivel minimo de autonomia que permite concluir a tarefa descrita em `tarefa.texto` num agente de codigo rodando sem ninguem para responder perguntas.",
			Criteria: map[string]string{
				"auto":         "Basta ler arquivos e responder. Nenhuma escrita em disco e nenhum comando que altere estado.",
				"accept-edits": "Precisa editar arquivos do repositorio, mas nao precisa executar comando nenhum para concluir.",
				"smart":        "Precisa executar comandos, tipicamente rodar teste ou lint, alem de editar arquivos.",
			},
		},
		"dimensao_dominante": Choice{
			Instructions: "Identifique qual capacidade a MUDANCA pedida em `tarefa.texto` mais exige de quem for executa-la. Todo briefing deste sistema traz a mesma rotina — escrever o teste, confirmar o vermelho, corrigir, confirmar o verde — e os mesmos comandos de teste e lint. Essa rotina e constante, aparece em toda tarefa e nao distingue nenhuma: ignore-a por completo e julgue apenas a natureza da alteracao de codigo que esta sendo pedida.",
			Criteria: map[string]string{
				"mecanica":   "Transformar codigo de forma local e ja determinada: corrigir um comparador, renomear um identificador, ajustar um formato, transcrever o trecho que o briefing descreve.",
				"raciocinio": "O que falta saber esta NO CODIGO-FONTE e nos documentos, e sai lendo com atencao suficiente — ainda que a causa comece desconhecida. Entra aqui deduzir uma janela de concorrencia entre duas operacoes, reconstruir um fluxo de dados, achar a premissa errada de um algoritmo: tudo isso esta escrito em algum arquivo, mesmo que espalhado, e executar serve para confirmar a conclusao, nao para chegar nela.",
				"agentica":   "O que falta saber NAO ESTA em arquivo nenhum: so passa a existir quando algo roda. Em que etapa um script para, o que um servico responde, qual saida um comando produz nesta maquina — nenhuma leitura de codigo revela isso, e cada execucao muda o passo seguinte. Extensao sozinha NAO e agentica: uma migracao mecanica repetida em dezenas de pontos continua mecanica, e o custo dela aparece na complexidade, nao aqui. Nao basta a tarefa pedir teste e lint: toda tarefa deste sistema pede, e isso nao distingue nenhuma.",
			},
		},
		"volume": Score{
			Instructions: "Avalie QUANTO TRABALHO a tarefa em `tarefa.texto` representa: em quantos pontos distintos e preciso mexer, e quantos ciclos de editar-e-conferir ela exige. Isto nao e dificuldade — uma mudanca trivial repetida em trinta arquivos e facil e volumosa, e uma unica linha sutil e dificil e pequena. Julgue so o tamanho. Ignore a rotina de escrever teste, confirmar vermelho e confirmar verde: toda tarefa deste sistema a tem, e ela nao distingue nenhuma.",
			Criteria: []string{
				"Um ponto so: uma funcao, uma constante, uma condicao. Quem executa abre um arquivo, muda o que precisa e acabou.",
				"Poucos pontos que andam juntos: uma funcao e seus dois ou tres chamadores, ou uma mudanca mais o ajuste que ela obriga no mesmo pacote.",
				"Uma duzia de pontos espalhados por varios pacotes, cada um pequeno, com conferencia entre eles para nao quebrar o caminho.",
				"Dezenas de pontos, ou uma sequencia longa em que cada etapa precisa ser verificada antes da proxima comecar; terminar exige muitas idas e vindas, mesmo que cada uma seja simples.",
			},
		},
		"complexidade": Score{
			Instructions: "Avalie quanto raciocinio a tarefa em `tarefa.texto` exige de quem for executa-la, considerando o que precisa ser entendido antes de escrever a primeira linha.",
			Criteria: []string{
				"A mudanca e local e mecanica: corrigir um comparador, um nome errado, um off-by-one, um valor de constante. Quem le a tarefa ja sabe o que digitar.",
				"A mudanca exige ler uma funcao e seus chamadores para entender o contrato antes de alterar, mas o defeito e visivel quando se olha o lugar certo.",
				"A mudanca exige reconstruir o comportamento a partir de varios arquivos, entender um fluxo de dados ou um estado que atravessa camadas, e decidir onde exatamente intervir.",
				"A mudanca exige entender uma interacao nao obvia entre partes do sistema, como concorrencia, cache, ordem de eventos ou compatibilidade retroativa, onde a correcao ingenua introduz outro defeito.",
			},
		},
	}
}

// AutonomyQuestion mede se o briefing deixa a execucao determinada (spec §8).
// A fronteira vem de medicao, nao de intuicao: 16 das 18 tarefas do plano v1
// eram 72-91% codigo literal, e o swe-2 executou oito com fidelidade e morreu
// na primeira que exigia montagem. Tarefa autocontida grande cabe num modelo
// barato; tarefa pequena que exige decisao, nao.
func AutonomyQuestion() map[string]Question {
	return map[string]Question{
		"tarefa_autocontida": Noul{
			Instructions: "O briefing em `briefing.texto` fecha as DECISOES da tarefa: quem executa sabe qual e o comportamento correto e onde intervir, e o que resta e escrever o codigo que realiza isso. Escrever o teste e a correcao e execucao esperada, nao decisao em aberto — nenhum briefing deste sistema entrega o codigo pronto, e isso sozinho nao torna a tarefa aberta.",
			Criteria: &NoulCriteria{
				True:  "O briefing localiza o defeito e diz qual e o comportamento correto, de forma que so ha um jeito razoavel de corrigir. Quem executa escreve o teste e a correcao sem precisar escolher entre desenhos, nem descobrir qual deveria ser o resultado certo.",
				False: "O comportamento correto ou o caminho ficam em aberto: o briefing pede um objetivo sem dizer como chegar, oferece alternativas sem decidir entre elas, ou exige investigar para descobrir a causa antes de saber o que mudar.",
			},
		},
	}
}

// WatchdogQuestions julgam um turno em andamento (spec §6.3). O que e
// detectavel por codigo (comando repetido, arquivo fora do escopo) nao esta
// aqui de proposito; bloqueio_de_permissao tambem saiu: a permissao virou
// allowlist em codigo e o estado que ela detectava nao existe mais.
func WatchdogQuestions() map[string]Question {
	return map[string]Question{
		"sem_progresso": Noul{
			Instructions: "Comparando o turno mais recente em `janela.turno_atual` com os anteriores em `janela.turnos_previos`, o agente deixou de progredir na tarefa.",
			Criteria: &NoulCriteria{
				True:  "O turno repete o que ja foi feito, reexplora o que ja foi visto, ou tenta a mesma coisa de novo sem mudar nada relevante na tentativa.",
				False: "O turno acrescenta informacao nova, avanca para uma etapa seguinte, ou muda de abordagem depois de um resultado.",
			},
		},
	}
}

// EvidenceQuestion seleciona evidencia para o briefing (spec §6.5, ida).
func EvidenceQuestion() map[string]Question {
	return map[string]Question{
		"evidencia_necessaria": Noul{
			Instructions: "Quem for executar a tarefa descrita em `tarefa.texto`, sem acesso a conversa que a originou, precisa do conteudo em `item.texto` para fazer o trabalho certo.",
			Criteria: &NoulCriteria{
				True:  "Sem este conteudo, quem executa teria que adivinhar: ele localiza o defeito, define o contrato violado, mostra o erro exato, ou diz como rodar o teste.",
				False: "E contexto de fundo, repeticao do que ja esta dito em outro item, ou detalhe que nao muda nada na execucao.",
			},
		},
	}
}

// CompactionQuestions compactam o trace por delecao (spec §6.5, volta).
// Mecanismo emprestado de fast-jev-compaction: nunca reescrever, so apagar.
func CompactionQuestions() map[string]Question {
	return map[string]Question{
		"chamada_necessaria": Noul{
			Instructions: "Quem for conferir se o trabalho descrito em `tarefa.texto` foi bem feito precisa saber que a acao em `interacao.chamada` aconteceu.",
			Criteria: &NoulCriteria{
				True:  "A acao faz parte da prova: escreveu o teste, rodou o teste, alterou o codigo, ou mostrou o estado que justificou a decisao seguinte.",
				False: "E exploracao, navegacao ou tentativa abandonada, cuja ausencia nao muda a conferencia.",
			},
		},
		"resultado_necessario_verbatim": Noul{
			Instructions: "O conteudo em `interacao.resultado` precisa ser preservado palavra por palavra para que a conferencia do trabalho continue possivel.",
			Criteria: &NoulCriteria{
				True:  "Contem o dado exato que sera conferido: saida de teste, mensagem de erro, diff, valor retornado.",
				False: "E confirmacao generica, listagem de diretorio, ou saida volumosa cujo unico conteudo util e ter dado certo.",
			},
		},
	}
}

// ReportQuestion confere o que o relatorio afirma (spec §6.6). O outro lado
// da comparacao e o exit code capturado, que e codigo, nao modelo.
func ReportQuestion() map[string]Question {
	return map[string]Question{
		"relatorio_afirma_verde": Noul{
			Instructions: "O relatorio em `relatorio.texto` afirma que os testes passaram depois da correcao.",
			Criteria: &NoulCriteria{
				True:  "Afirma que a suite ficou verde, que os testes passam, ou que a correcao foi verificada com sucesso.",
				False: "Relata falha, relata que nao conseguiu rodar, ou nao diz nada sobre o resultado dos testes.",
			},
		},
	}
}

// FindingQuestions triam achados de revisao (spec §6.7). A presenca de
// arquivo:linha e conferida por regex no codigo, nao aqui.
func FindingQuestions() map[string]Question {
	return map[string]Question{
		"tem_cenario_reproduzivel": Noul{
			Instructions: "O achado em `achado.texto` descreve uma situacao concreta em que o defeito aparece, e nao apenas uma opiniao sobre como o codigo deveria ser.",
			Criteria: &NoulCriteria{
				True:  "Descreve entrada, estado ou sequencia de acoes sob a qual o comportamento errado acontece, de forma que alguem poderia tentar reproduzir.",
				False: "Aponta estilo, nomenclatura, preferencia de estrutura, ou afirma que algo e arriscado sem dizer quando quebra.",
			},
		},
		"severidade": Score{
			Instructions: "Avalie a consequencia do problema descrito em `achado.texto` caso ele seja real e chegue a producao.",
			Criteria: []string{
				"Nao muda o comportamento do sistema: e legibilidade, nomenclatura ou preferencia de estilo.",
				"Degrada manutencao ou desempenho de forma perceptivel, mas o sistema continua correto para o usuario.",
				"Produz resultado errado, perda de dado ou falha visivel em algum caminho de execucao que usuarios percorrem.",
				"Abre brecha de seguranca, corrompe dado de forma silenciosa, ou derruba o sistema para todos os usuarios.",
			},
		},
	}
}

// MaxChamadasPorTurno e o teto de interacoes que CompactionQuestionsFor
// cobre num turno. Cada chamada custa duas perguntas, e perguntas comem o
// orcamento de state (StateBudget): passar disso estreitaria a janela que o
// watchdog usa para julgar progresso. Turno mais largo que isso cai no
// caminho antigo, uma pergunta por interacao.
const MaxChamadasPorTurno = 8

// CompactionQuestionsFor monta as perguntas de compactacao das n primeiras
// chamadas do turno atual, para irem no MESMO request do watchdog.
//
// As duas etapas julgavam o mesmo material em requests separados: o
// watchdog mandava a janela a cada turno, a compactacao mandava interacao
// por interacao no fim. Medido em 2026-09-21 num job de 22 turnos, eram 21
// requests de watchdog e 29 de compactacao — 50 idas seriais a rede num run
// de 220 segundos. Perguntas independentes sobre o mesmo state vao juntas e
// sao avaliadas em paralelo, entao unir as duas nao custa tempo de
// resposta, e o que era serial vira um request por turno.
//
// Os ids carregam a posicao porque o codigo precisa dela para casar a
// resposta com a chamada; o modelo nunca os ve — o que localiza a interacao
// e o caminho citado nas instrucoes.
func CompactionQuestionsFor(n int) map[string]Question {
	if n > MaxChamadasPorTurno {
		n = MaxChamadasPorTurno
	}
	qs := make(map[string]Question, 2*n)
	for i := 0; i < n; i++ {
		qs[fmt.Sprintf("chamada_%d_necessaria", i)] = Noul{
			Instructions: fmt.Sprintf(
				"Quem for conferir se o trabalho descrito em `tarefa.texto` foi bem feito precisa "+
					"saber que a acao em `janela.turno_atual.chamadas[%d]` aconteceu.", i),
			Criteria: &NoulCriteria{
				True: "A acao faz parte da prova: escreveu o teste, rodou o teste, alterou o codigo, " +
					"ou mostrou o estado que justificou a decisao seguinte.",
				False: "E exploracao, navegacao ou tentativa abandonada, cuja ausencia nao muda a conferencia.",
			},
		}
		qs[fmt.Sprintf("resultado_%d_verbatim", i)] = Noul{
			Instructions: fmt.Sprintf(
				"O conteudo em `janela.turno_atual.resultados[%d]` precisa ser preservado palavra por "+
					"palavra para que a conferencia do trabalho continue possivel.", i),
			Criteria: &NoulCriteria{
				True:  "Contem o dado exato que sera conferido: saida de teste, mensagem de erro, diff, valor retornado.",
				False: "E confirmacao generica, listagem de diretorio, ou saida volumosa cujo unico conteudo util e ter dado certo.",
			},
		}
	}
	return qs
}

// IDsDaChamada devolve os ids das duas perguntas da i-esima chamada.
func IDsDaChamada(i int) (necessaria, verbatim string) {
	return fmt.Sprintf("chamada_%d_necessaria", i), fmt.Sprintf("resultado_%d_verbatim", i)
}
