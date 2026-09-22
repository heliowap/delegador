#!/usr/bin/env python3
"""Transforma o placar do piloto em numeros que o roster pode carregar.

O que sai daqui NAO e ranking: tres tarefas, uma execucao por celula. E uma
TRIAGEM — separa "nao faz" de "faz", e da o tamanho em tokens de quem faz.
Ranking entre modelos proximos precisa de n maior, e de tarefas que nao
foram escolhidas por mim.
"""
import collections
import sys

placar = sys.argv[1] if len(sys.argv) > 1 else "/tmp/piloto/run/placar.tsv"
linhas = []
for l in open(placar).read().strip().split("\n")[1:]:
    c = l.split("\t")
    if len(c) < 11 or not c[4].isdigit():
        continue
    linhas.append({
        "modelo": c[0], "tarefa": c[1], "veredito": c[2], "parada": c[3],
        "turnos": int(c[4]), "tin": int(c[5]), "tout": int(c[6]), "seg": int(c[7]),
        "teste": int(c[8]), "extra": int(c[9]),
    })

ordem = ["facil", "medio", "dificil"]
por = collections.defaultdict(dict)
for r in linhas:
    por[r["modelo"]][r["tarefa"]] = r

print(f"{'modelo':12}{'facil':>10}{'medio':>10}{'dificil':>10}{'verdes':>8}"
      f"{'tok/tarefa':>12}{'s/tarefa':>10}{'fidelidade':>12}")
for mod, cells in sorted(por.items(), key=lambda kv: -sum(
        1 for t in kv[1].values() if t["veredito"] == "VERDE")):
    marcas = []
    for t in ordem:
        r = cells.get(t)
        if not r:
            marcas.append("-")
        elif r["veredito"] == "VERDE":
            marcas.append("ok")
        else:
            marcas.append(r["parada"][:8])
    verdes = [r for r in cells.values() if r["veredito"] == "VERDE"]
    tok = sorted(r["tin"] + r["tout"] for r in verdes)
    seg = sorted(r["seg"] for r in verdes)
    med = lambda v: v[len(v) // 2] if v else 0
    # Fidelidade e sobre TODAS as execucoes, nao so as verdes: tocar no
    # oraculo ou escrever fora do escopo e desvio mesmo quando da certo.
    desvios = sum(1 for r in cells.values() if r["teste"] > 0 or r["extra"] > 0)
    fid = f"{len(cells)-desvios}/{len(cells)}"
    print(f"{mod:12}{marcas[0]:>10}{marcas[1]:>10}{marcas[2]:>10}"
          f"{len(verdes):>5}/{len(cells):<2}{med(tok):>12,}{med(seg):>10}{fid:>12}")

print("\nalterou_teste / arquivo_extra por celula (desvio de escopo):")
for mod, cells in sorted(por.items()):
    ruins = [f"{t}(teste={r['teste']},extra={r['extra']})"
             for t, r in cells.items() if r["teste"] or r["extra"]]
    if ruins:
        print(f"  {mod:12} {' '.join(ruins)}")
