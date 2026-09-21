#!/usr/bin/env python3
"""Tokens por tarefa MEDIDOS, a partir dos ledgers do executor.

O roster carrega uma estimativa derivada do benchmark de terceiro
(`Benchmark.TokensPorTarefa`). Este script produz o numero do nosso
harness, para a estimativa poder ser conferida em vez de acreditada.

Uso: python3 medir-tekens.py <dir de jobs>
     (padrao: $XDG_STATE_HOME/delegador/jobs)

Separa as tentativas de um job escalado pela assinatura de preco do
ledger: cada modelo tem a sua, e a escalada troca de modelo no meio.
"""
import collections
import glob
import json
import os
import re
import sys

jobs = sys.argv[1] if len(sys.argv) > 1 else os.path.join(
    os.environ.get("XDG_STATE_HOME", os.path.expanduser("~/.local/state")),
    "delegador", "jobs")


def issue(d):
    try:
        m = re.search(r"issues/(\d+)", open(f"{d}/briefing.md").read())
        return "#" + m.group(1) if m else "?"
    except OSError:
        return "?"


linhas = []
for d in sorted(glob.glob(os.path.join(jobs, "job-*"))):
    ledger = f"{d}/executor.jsonl"
    if not os.path.exists(ledger):
        continue
    res = open(f"{d}/result.txt").read() if os.path.exists(f"{d}/result.txt") else ""
    verde = res.startswith("veredito: verde")
    sinal = re.search(r"CANCELADO pelo watchdog: (\w+)", res)

    por = collections.OrderedDict()
    for l in open(ledger):
        e = json.loads(l)
        k = (e["usd_per_mtok_input"], e["usd_per_mtok_output"])
        a = por.setdefault(k, {"turnos": 0, "tin": 0, "tout": 0, "usd": 0.0})
        a["turnos"] += 1
        a["tin"] += e["input_tokens"]
        a["tout"] += e["output_tokens"]
        a["usd"] += e["input_tokens"] * k[0] / 1e6 + e["output_tokens"] * k[1] / 1e6

    for i, (k, a) in enumerate(por.items()):
        ultima = i == len(por) - 1
        # So a ULTIMA tentativa tem o desfecho do job; as anteriores
        # escalaram, e um modelo que escalou nao "terminou" a tarefa.
        if not ultima:
            desfecho = "escalada"
        elif verde:
            desfecho = "verde"
        elif sinal:
            desfecho = "veto " + sinal.group(1)
        else:
            desfecho = "vermelho"
        linhas.append((issue(d), k, a, desfecho))

print(f"{'issue':7}{'preco in/out':>16}{'turnos':>7}{'tok_in':>12}{'tok_out':>9}{'US$':>9}  desfecho")
for iss, k, a, desf in linhas:
    print(f"{iss:7}{f'{k[0]}/{k[1]}':>16}{a['turnos']:7}{a['tin']:12,}"
          f"{a['tout']:9,}{a['usd']:9.2f}  {desf}")

print("\nTOKENS POR TAREFA, so onde a tentativa TERMINOU verde.")
print("Tentativa que nao terminou gasta menos por nao ter terminado: media-la")
print("junto inverteria o sinal que se quer medir.")
ag = collections.defaultdict(list)
for iss, k, a, desf in linhas:
    if desf == "verde":
        ag[k].append(a["tin"] + a["tout"])
for k, v in sorted(ag.items()):
    v.sort()
    print(f"  preco {k[0]}/{k[1]}: n={len(v)}  mediana={v[len(v)//2]:,}  "
          f"min={v[0]:,}  max={v[-1]:,}")

tin = sum(a["tin"] for _, _, a, _ in linhas)
tout = sum(a["tout"] for _, _, a, _ in linhas)
if tin:
    print(f"\nProporcao saida/entrada: {tout / tin * 100:.2f}%  "
          f"({tout:,} / {tin:,}) — e a premissa de Benchmark.TokensPorTarefa")
