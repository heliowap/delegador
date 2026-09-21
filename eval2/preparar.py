#!/usr/bin/env python3
"""Prepara uma tarefa nua do plano v1 para o delegador executar.

Coloca os TESTES no disco a partir do plano e deixa a implementacao de fora.
Assim o juiz nao e o teste que o proprio modelo escreveu — que seria circular
— e sim o teste do plano, intocado.

uso: preparar.py <plano.md> <n> <worktree> <dir-saida>
"""
import json, pathlib, re, sys


def blocos(plano: str, n: int):
    m = re.search(rf"\n### Task {n}: .*?(?=\n### Task \d+: |\n## Ordem)", plano, re.S)
    if not m:
        sys.exit(f"tarefa {n} nao encontrada")
    body = m.group(0)
    testes, impls = [], []
    for blk in re.finditer(r"```go\n(.*?)```", body, re.S):
        code = blk.group(1)
        primeira = code.splitlines()[0].strip()
        if not primeira.startswith("// ") or not primeira.endswith(".go"):
            continue
        caminho = primeira[3:].strip()
        (testes if "func Test" in code else impls).append((caminho, code))
    return body, testes, impls


def interfaces(body: str) -> str:
    m = re.search(r"\*\*Interfaces:\*\*\n(.*?)(?=\n- \[ \]|\n\*\*)", body, re.S)
    return m.group(1).strip() if m else ""


def main():
    plano_path, n, worktree, saida = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4]
    plano = pathlib.Path(plano_path).read_text()
    body, testes, impls = blocos(plano, n)

    wt, out = pathlib.Path(worktree), pathlib.Path(saida)
    out.mkdir(parents=True, exist_ok=True)

    # 1. testes no disco, antes de o modelo comecar
    for caminho, code in testes:
        f = wt / caminho
        f.parent.mkdir(parents=True, exist_ok=True)
        f.write_text(code)
    (out / "testes.txt").write_text("\n".join(c for c, _ in testes) + "\n")

    alvos = [c for c, _ in impls]
    (out / "alvos.txt").write_text("\n".join(alvos) + "\n")

    # 2. evidencia: o contrato e a especificacao, nunca a implementacao
    ev = [
        {"kind": "fonte", "ref": f"{plano_path}#task-{n}",
         "text": "Contrato desta tarefa, do plano aprovado:\n\n" + interfaces(body)},
        {"kind": "trecho", "ref": f"{testes[0][0]}:1",
         "text": "Os testes ja estao escritos no disco e sao o criterio de aceite. "
                 "Nao os altere.\n\n" + testes[0][1][:3000]},
        {"kind": "comando", "text": "go test ./... && go vet ./..."},
    ]
    (out / "evidencia.jsonl").write_text("\n".join(json.dumps(e, ensure_ascii=False) for e in ev) + "\n")

    # 3. a tarefa
    tarefa = (
        f"Implemente a tarefa {n} do plano deste repositorio. Os arquivos de TESTE ja "
        f"estao no disco e definem o comportamento exigido: {', '.join(c for c, _ in testes)}. "
        f"Voce precisa escrever os arquivos de implementacao que fazem esses testes "
        f"passarem: {', '.join(alvos)}. Respeite exatamente as assinaturas do contrato. "
        f"NAO altere nenhum arquivo terminado em _test.go — eles sao o criterio de aceite, "
        f"e alterar qualquer um reprova a tarefa. Pronto quando 'go test ./...' passar e "
        f"'go vet ./...' sair limpo. Nao faca commit, nao faca push, nao acesse a rede."
    )
    (out / "tarefa.txt").write_text(tarefa)

    # Comando de teste especifico dos pacotes tocados. Generico demais
    # ("go test ./..." como teste E como suite) deixa a secao de Comandos do
    # briefing com a mesma linha repetida, e o gate comandos_copiaveis reprova
    # — com razao: isso nao e instrucao, e ruido.
    pacotes = sorted({str(pathlib.Path(c).parent) for c in alvos + [t for t, _ in testes]})
    (out / "cmds.txt").write_text(
        "go test " + " ".join("./" + p + "/" for p in pacotes) + "\n"
        "go test ./...\n"
        "go vet ./...\n")
    print(f"tarefa {n}: {len(testes)} teste(s) no disco, {len(alvos)} arquivo(s) a implementar")
    print("  testes:", ", ".join(c for c, _ in testes))
    print("  alvos: ", ", ".join(alvos))


if __name__ == "__main__":
    main()
