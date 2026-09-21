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

## O que foi medido, e onde fica registrado

Fixture não é só rótulo: cada caso de `autonomia` carrega o valor que o Jev
de fato respondeu, em `medido_2026_09_21`, e o bloco `_calibragem` no topo do
`fixtures.json` guarda o resumo — método, as duas classes, o vão entre elas e
o limiar escolhido, com a justificativa.

Isso existe para que um número de limiar nunca seja um número solto. Quem
abrir `route.LimiarAutocontida` daqui a seis meses e quiser mexer vai
encontrar que ele saiu de sete fixtures, mediana de três execuções cada,
classe `true` em 0,900–0,940 e `false` em 0,050–0,350 — e que o valor é o
ponto médio do vão, descentrado para cima se precisar, porque tratar tarefa
ambígua como fechada custa um run inteiro e o erro contrário custa centavos.

O mesmo bloco registra o que **não** foi calibrado, e por quê. O
`PisoTauMinimo` é percentil dentro do roster, não corte sobre resposta do
Jev: não existe rótulo dizendo qual percentil de tau basta, e fabricar uma
fixture que fingisse calibrá-lo daria ao número uma autoridade que ele não
tem. O que o sustenta é um teste sobre o roster real, e o campo
`o_que_sustenta` diz isso em voz alta.

Medição sem procedência vira folclore em três meses. O bloco é a procedência.

## Quando um eval falha

A pergunta é qual dos dois está errado — o limiar ou o texto da pergunta — e a
resposta não é "ajustar o rótulo". Ajustar gabarito depois de ver a resposta é
o modo mais fácil de transformar este diretório em teatro. Se o rótulo não se
sustenta, **retire-o** e diga por quê na `nota`, em vez de trocá-lo pelo que o
modelo respondeu.

Mudou o texto de uma pergunta? Incremente `jev.QuestionsVersion`, senão a
auditoria em `jev.jsonl` deixa de ser interpretável.
