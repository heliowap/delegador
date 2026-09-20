#!/usr/bin/env python3
"""Remove os blocos de implementacao de uma tarefa, preservando testes e Interfaces.

Serve ao braco de capacidade do eval: o modelo recebe o contrato e os testes,
e precisa AUTORAR a implementacao. Os testes do plano sao o juiz.

uso: strip-task.py <plano.md> <numero-da-tarefa> > tarefa-nua.md
"""
import re, sys

def main():
    plan, n = sys.argv[1], sys.argv[2]
    txt = open(plan).read()
    m = re.search(rf"\n### Task {n}: .*?(?=\n### Task \d+: |\n## Ordem de execução)", txt, re.S)
    if not m:
        sys.exit(f"tarefa {n} nao encontrada")
    body = m.group(0)

    out, i, removed = [], 0, 0
    # Mantem blocos que contenham funcao de teste, comando de shell ou go.mod.
    for blk in re.finditer(r"```(\w*)\n(.*?)```", body, re.S):
        lang, code = blk.group(1), blk.group(2)
        keep = (lang != "go") or ("func Test" in code) or ("module " in code)
        out.append(body[i:blk.start()])
        if keep:
            out.append(blk.group(0))
        else:
            removed += 1
            out.append("```go\n// IMPLEMENTACAO REMOVIDA DE PROPOSITO.\n"
                       "// Escreva voce mesmo o codigo que faz os testes acima passarem,\n"
                       "// respeitando exatamente as assinaturas do bloco Interfaces.\n```")
        i = blk.end()
    out.append(body[i:])
    sys.stderr.write(f"tarefa {n}: {removed} blocos de implementacao removidos\n")
    print("".join(out))

if __name__ == "__main__":
    main()
