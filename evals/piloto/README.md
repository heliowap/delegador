# Piloto de capacidade — seis modelos, três degraus

Mede, no NOSSO harness, o que os modelos disponíveis dão conta de fazer.
Curto de propósito: três tarefas, uma execução por célula. Não é ranking —
é triagem, e separa "não faz" de "faz".

```bash
cd /tmp && rm -rf piloto && mkdir piloto && cd piloto
cp ~/VSCode/delegador/evals/piloto/* .
# precisa do harness de issues reais montado antes (evals/issues-reais)
bash montar.sh                    # uma copia do repositorio por celula
OCIOSOS=25 ./rodar.sh             # plan + run, cascata desligada
python3 apurar.py                 # placar -> numeros
```

`roster-de.py` gera um roster de UM modelo, para a rota deixar de ser
variável: o piloto mede o modelo, não a escolha.

`rodar.sh` roda com `--max-escaladas 0`. Se a cascata resgatasse, o
resultado da célula seria do resgate, não do modelo.

`OCIOSOS` existe porque a primeira passada mediu o watchdog em vez dos
modelos — ver o relatório. `placar-2026-09-21-ociosos10.tsv` é essa
passada, guardada porque a diferença entre as duas é o achado.

As tarefas são três das seis issues do `evals/issues-reais`, escolhidas
pela dificuldade JÁ MEDIDA com o opus-5 naquele eval, não por intuição:
fácil (#857, 5 turnos), média (#888, 8-10 turnos), difícil (#836, falhou
duas vezes e precisou de escalada).
