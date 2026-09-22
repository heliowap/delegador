# Ablação: o mesmo modelo, com e sem as camadas de julgamento

Responde à pergunta que o projeto inteiro pressupõe: **o plugin melhora o
resultado, ou só cobra por ele?**

O braço de controle (`evals/sem-plugin`) usa o MESMO laço, as MESMAS
ferramentas e a MESMA camada de permissão. Fica de fora só o julgamento:
gate de delegabilidade, gate de briefing, seleção de evidência, rota,
watchdog, compactação, sonda de mutação, conferência de veracidade e
cascata. O prompt é a tarefa e a evidência como alguém as colaria.

A permissão fica dentro de propósito. Comparar um braço que pode `curl`
com outro que não pode mediria outra coisa.

O juiz é externo e idêntico aos dois: o harness roda o teste do mantenedor
e a suíte depois que cada braço termina.

```bash
cd /tmp/ablacao && MODELO='cpa-claude-opus-5(high)' ./rodar.sh
```
