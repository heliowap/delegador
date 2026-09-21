#!/usr/bin/env python3
"""Diz se os testes do plano continuam identicos no disco."""
import re, sys, pathlib
plano, n, wt = sys.argv[1], int(sys.argv[2]), pathlib.Path(sys.argv[3])
body = re.search(rf"\n### Task {n}: .*?(?=\n### Task \d+: |\n## Ordem)", pathlib.Path(plano).read_text(), re.S).group(0)
for blk in re.findall(r"```go\n(.*?)```", body, re.S):
    if "func Test" not in blk:
        continue
    caminho = blk.splitlines()[0].strip()[3:].strip()
    f = wt / caminho
    if not f.exists() or f.read_text().strip() != blk.strip():
        print("MODIFICADO"); sys.exit(0)
print("INTOCADO")
