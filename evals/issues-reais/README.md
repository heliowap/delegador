# Harness: issues reais do expr-lang/expr

Reproduz o eval de 2026-09-21 (`docs/evals/2026-09-21-escalada-issues-reais.md`).

```bash
cd /tmp && rm -rf swebench && mkdir swebench && cd swebench
cp ~/VSCode/delegador/evals/issues-reais/* .
git clone --filter=blob:none https://github.com/expr-lang/expr.git expr
bash montar.sh          # repositorios no commit PAI de cada correcao
python3 prep.py         # tarefa, evidencia, triagem; falha se algum caso nao estiver vermelho
./rodar.sh 1 2 3 4 5 6  # plan + run, um caso por vez
./conferir.sh           # compara o diff do modelo com o do mantenedor
```

`montar.sh` monta cada caso como um repositorio NOVO, sem remote e sem o
object store do upstream: o modelo nao consegue achar o commit que corrige.
Sobre a arvore do commit pai ele aplica so os arquivos de teste da correcao.

`prep.py` faz `assert` de que o caso esta vermelho antes de gerar a evidencia.
Se um caso ficar verde — porque o upstream mudou, ou porque o teste passou a
depender de outra coisa — o harness para, em vez de medir um oraculo vazio.

A triagem de `prep.py` e um grep do identificador que a PROPRIA issue nomeia;
`termos.tsv` registra, para cada caso, o termo e a citacao de onde ele saiu.
Isso existe para o gate `aponta_arquivo_linha` ter o que julgar sem que quem
monta o eval localize o defeito — localizar e parte da resposta.

`placar-2026-09-21.tsv` e o resultado daquele dia. A primeira linha esta com
as colunas deslocadas: o driver tinha um bug de parsing de `escalou`,
corrigido depois. O custo do caso 1 e US$ 1,69.
