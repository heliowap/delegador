#!/usr/bin/env python3
"""Qual eixo da rota separa as tarefas que separaram os modelos?

O corte de percentil da rota opera sobre `complexidade`. Este script
confere, contra os classificacao.json ja gravados, se esse eixo de fato
distingue tarefas de dificuldade diferente — e compara com os atomos
(alcance, acoplamento, sutileza) e com volume.

sinal = amplitude das medianas ENTRE tarefas
ruido = maior desvio-padrao DENTRO de uma tarefa (o mesmo texto, n execucoes)

Razao alta significa que o eixo mede a tarefa, e nao o acaso da chamada.

uso: eixos.py [placar.tsv] [dir de jobs]
"""
import collections
import json
import os
import statistics
import sys

placar = sys.argv[1] if len(sys.argv) > 1 else "run/placar.tsv"
jobs = sys.argv[2] if len(sys.argv) > 2 else "run/state/delegador/jobs"

por = collections.defaultdict(list)
tarefas = []
for l in open(placar).read().strip().split("\n")[1:]:
    c = l.split("\t")
    if len(c) < 11 or not c[10].startswith("job-"):
        continue
    p = os.path.join(jobs, c[10], "classificacao.json")
    try:
        d = json.load(open(p))
    except OSError:
        continue
    if c[1] not in tarefas:
        tarefas.append(c[1])
    for eixo, v in d["respostas"].items():
        if "score" in v:
            por[(eixo, c[1])].append(v["score"])

eixos = sorted({e for e, _ in por})
print(f"{'eixo':14}{'sinal':>7}{'ruido':>8}{'razao':>8}   medianas por tarefa")
for eixo in eixos:
    meds, ruidos, ns = [], [], []
    for t in tarefas:
        v = por.get((eixo, t), [])
        if not v:
            continue
        meds.append(statistics.median(v))
        ruidos.append(statistics.pstdev(v) if len(v) > 1 else 0.0)
        ns.append(len(v))
    if len(meds) < 2:
        continue
    sinal = max(meds) - min(meds)
    ruido = max(ruidos)
    razao = sinal / ruido if ruido else float("inf")
    corpo = "/".join(f"{m:.2f}" for m in meds)
    print(f"{eixo:14}{sinal:7.2f}{ruido:8.2f}{razao:8.1f}   {corpo}  n={ns}")
