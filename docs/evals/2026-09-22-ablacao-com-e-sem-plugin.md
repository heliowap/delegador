# O mesmo modelo, com e sem o plugin

**2026-09-22.** `cpa-claude-opus-5(high)`, três tarefas, seis execuções.

## Veredito

**O plugin não mudou o resultado de nenhuma das três.** Os dois braços
fizeram 3 de 3, e os dois passaram na sonda de mutação.

| braço | tarefa | veredito | turnos | tokens entrada | seg | arq. extra |
|---|---|---|---:|---:|---:|---:|
| sem plugin | fácil | verde | 10 | 199.328 | 128 | 0 |
| **com plugin** | fácil | verde | 18 | 393.412 | 195 | 0 |
| sem plugin | médio | verde | 16 | 670.528 | 167 | **1** |
| **com plugin** | médio | verde | 17 | 701.466 | 184 | 0 |
| sem plugin | difícil | verde | 34 | 2.477.085 | 507 | 0 |
| **com plugin** | difícil | verde | 34 | 2.525.092 | 548 | 0 |

Total: **3.346.941 tokens e 802 s** sem o plugin, **3.619.970 e 927 s** com
ele. O plugin custou **+8% de tokens e +16% de relógio**.

O sobrecusto não é uniforme: **+97% de tokens na tarefa fácil, +2% na
difícil**. Ele é quase todo custo fixo — os gates, a seleção de evidência e
a rota rodam igual num trabalho de dez turnos e num de trinta e quatro.
Quanto maior a tarefa, menos ele pesa.

## O que o plugin entregou que o controle não entregou

**A prova, automaticamente.** Os dois braços produziram correções reais —
apliquei a sonda de mutação à mão nos três resultados do controle, e os três
ficam vermelhos sem a correção. Mas **eu só soube fazer isso porque o plugin
define o padrão.** O controle devolve um teste verde; ele não diz nada sobre
o teste valer alguma coisa. A saída do plugin é uma afirmação com evidência
anexada: `teste: exit 0 | mutacao: exit 1 (esperado) | suite: exit 0 | lint:
exit 0`, mais `relato: 3 de 3 comandos conferem com o trace` nas três.

**Um desvio de escopo a menos.** Na tarefa média o controle escreveu um
arquivo além dos que o mantenedor tocou; o braço do plugin, não. Um caso —
não é tendência.

**Um piso quando dá errado.** O controle rodou 34 turnos na tarefa difícil
sem teto de custo, sem checagem de ociosidade e sem cascata. Deu certo. Um
run do controle que der errado não tem o que o interrompa.

## O que isto NÃO autoriza a concluir

**Três tarefas, um repositório, uma linguagem, um modelo.** E um modelo
forte: o mesmo `opus-5` em esforço `low` falhou a tarefa difícil duas vezes
em 2026-09-21 e só saiu com escalada. **O esforço foi a variável dominante,
muito acima da presença do plugin.**

E o braço do controle usou o mesmo modelo de propósito, para isolar as
camadas. Isso deixa de fora justamente a proposta principal do plugin —
fazer o trabalho sair de uma conta mais barata. Essa comparação é outra, e
ainda não foi feita.

## A leitura honesta

Com cota de fronteira sobrando e disposição para conferir o trabalho à mão,
**o `opus-5(high)` sozinho resolve estas três tarefas e custa 8% menos.**

O plugin se paga em duas situações, nenhuma delas medida aqui: quando o
executor **não** é o modelo caro, e quando ninguém vai conferir à mão. Fora
delas, ele é sobrecusto — e a medida desse sobrecusto, agora, é 8%.
