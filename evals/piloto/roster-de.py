#!/usr/bin/env python3
"""Gera um roster de UM modelo so, para forcar a rota a ele.

O piloto mede o modelo, nao a rota: cada braco roda com um roster que tem
exatamente um candidato, e a escolha deixa de ser variavel.

uso: roster-de.py <id do modelo> <preco_in> <preco_out> > roster.yaml
"""
import sys, datetime

mid, pin, pout = sys.argv[1], sys.argv[2], sys.argv[3]
hoje = datetime.date.today().isoformat()
print(f'''versao: 1
as_of_sondagem: "{hoje}"

contas:
  piloto:
    escassez: assinatura
    aperto: baixo

modelos:
  - id: {mid}
    papel: barato
    conta: piloto
    permaslug: null
    mapeamento: ausente
    sondado:
      tool_call: true
      reasoning_content: false
      tokens_base: 0
      latencia_s: 0.0
      em: "{hoje}"
    benchmark: null
    humano:
      custo_usd_por_mtok: {pin}
      custo_saida_usd_por_mtok: {pout}
      habilitado: true
''')
