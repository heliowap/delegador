# Evals

Fixtures rotuladas que conferem se as perguntas de `internal/jev/questions.go`
caem do lado certo do limiar **no nosso domínio**. Elas não validam o modelo:
validam a calibragem das nossas perguntas.

```bash
go test -count=1 ./evals/
```

Precisa de `TYPESAFE_API_KEY`. Custa pouco — Jev cobra US$ 0,042 por milhão de
tokens de entrada e a saída é gratuita —, mas depende de rede, então não roda
no CI.

## As três seções

**`rota`** confere `dimensao_dominante`, `complexidade` e `volume`. Duas
fixtures são um **par cruzado** e existem para provar que os eixos estão
separados: a migração tem complexidade baixa com volume alto, e a janela de
concorrência tem complexidade alta com volume baixo. Se alguém colapsar os
eixos de novo, uma das duas quebra.

**`autonomia`** confere `tarefa_autocontida`.

**`estabilidade`** repete a **mesma pergunta sobre o mesmo estado** e reprova
por duas coisas distintas: cair do lado errado do limiar em alguma execução, e
amplitude acima do tolerado. Um gate que oscila é pior que um gate severo —
ele ensina a tentar de novo em vez de corrigir.

## O que a estabilidade cobre, e por quê assim

Os nove nouls que **reprovam** trabalho: `desenho_em_aberto`, `toca_sensivel`,
`criterio_de_pronto` e os seis do briefing. `defeito_unico` e `cruza_pacotes`
ficam de fora porque só avisam.

Cada caso manda o **estado real do gate**, com `tarefa` e `briefing` juntos, e
isso não é detalhe. O único caso de oscilação real encontrado até hoje só
aparecia com os dois no estado: medir a pergunta isolada mostrava 0,130 a
0,160, perfeitamente estável, e produziu a conclusão errada de que não havia
problema. Com o estado real, a mesma pergunta devolvia 0,540 a 0,610, em cima
do limiar. **Medir contra uma reconstrução do estado esconde o defeito.**

Os dois casos que cobrem os nove gates de uma vez mandam uma requisição só:
perguntas independentes sobre o mesmo estado não veem as respostas umas das
outras.

## Quando um eval falha

A pergunta é qual dos dois está errado — o limiar ou o texto da pergunta — e a
resposta não é "ajustar o rótulo". Ajustar gabarito depois de ver a resposta é
o modo mais fácil de transformar este diretório em teatro. Se o rótulo não se
sustenta, **retire-o** e diga por quê na `nota`, em vez de trocá-lo pelo que o
modelo respondeu.

Mudou o texto de uma pergunta? Incremente `jev.QuestionsVersion`, senão a
auditoria em `jev.jsonl` deixa de ser interpretável.
