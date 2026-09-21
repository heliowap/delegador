import json, subprocess, sys, os, pathlib

SRC = "/tmp/swebench/expr"
casos = [l.split("\t") for l in open("/tmp/swebench/casos.tsv").read().strip().split("\n")]
cmds  = {l.split("\t")[0]: l.split("\t")[1:] for l in open("/tmp/swebench/cmds.tsv").read().strip().split("\n")}
termos = {l.split("\t")[0]: l.split("\t")[1] for l in open("/tmp/swebench/termos.tsv").read().strip().split("\n")}

def triagem(d, termo):
    """Triagem mecanica: grep do identificador que a PROPRIA issue nomeia,
    um hit por arquivo, em ordem de caminho, ate 12. Nenhuma linha deste
    resultado vem do commit de correcao — o termo sai do texto do relator."""
    cmd = ("grep -rn %r --include='*.go' . | grep -v '_test\\.go' | grep -v '^\\./test/' "
           "| grep -v '/internal/testify/' | grep -v '/internal/spew/'" % termo)
    out = subprocess.run(["bash","-c",cmd], cwd=d, capture_output=True, text=True).stdout
    vistos, linhas = set(), []
    for l in out.split("\n"):
        if ":" not in l: continue
        f = l.split(":",1)[0]
        if f in vistos: continue
        vistos.add(f); linhas.append(l.strip())
        if len(linhas) == 12: break
    return cmd, linhas

def sh(args, cwd=None):
    return subprocess.run(args, cwd=cwd, capture_output=True, text=True).stdout

for n, sha, pr, iss in casos:
    d = f"/tmp/swebench/c{n}"
    out = pathlib.Path(f"/tmp/swebench/prep/{n}"); out.mkdir(parents=True, exist_ok=True)
    issue = open(f"/tmp/swebench/issues/{n}-{iss}.md").read()
    titulo = issue.split("\n")[0].split(": ", 1)[1].strip()
    corpo  = issue.split("\n", 2)[2].strip()

    testes = [f for f in sh(["git","show","--name-only","--format=",sha], SRC).split()
              if f.endswith("_test.go")]
    diff = sh(["git","diff",f"{sha}^..{sha}","--"] + testes, SRC)
    # so as linhas ADICIONADAS pelo mantenedor, sem o ruido do formato diff
    add = "\n".join(l[1:] for l in diff.split("\n") if l.startswith("+") and not l.startswith("+++"))
    add = add[:4000]

    tc, sc, lc = [c.strip() for c in cmds[n]]
    erro = subprocess.run(["bash","-c",tc], cwd=d, capture_output=True, text=True)
    saida = (erro.stdout + erro.stderr)[:3000]
    assert erro.returncode != 0, f"caso {n} nao esta vermelho"

    tcmd, hits = triagem(d, termos[n])
    triado = "\n".join(hits)

    ev = [
      {"kind":"fonte","ref":f"github.com/expr-lang/expr/issues/{iss}","text":corpo[:4000]},
      {"kind":"erro","ref":tc,"text":saida},
      {"kind":"comando","ref":"verificacao","text":f"teste: {tc}\nsuite: {sc}\nlint: {lc}"},
      {"kind":"trecho","ref":", ".join(testes),"text":add},
      {"kind":"trecho","ref":hits[0].split(":")[0] + ":" + hits[0].split(":")[1] if hits else "",
       "text":"Triagem: onde o identificador %r citado na issue aparece no codigo "
              "(caminho:linha:conteudo). Um hit por arquivo, ordem de caminho.\n\n%s" % (termos[n], triado)},
    ]
    (out/"tarefa.txt").write_text(titulo + "\n")
    (out/"evidencia.jsonl").write_text("\n".join(json.dumps(e) for e in ev) + "\n")
    (out/"cmds.txt").write_text(f"{tc}\n{sc}\n{lc}\n")
    (out/"globs.txt").write_text("\n".join(sorted({os.path.basename(t) for t in testes})) + "\n")
    print(f"caso {n}: '{titulo[:60]}' | evid {sum(len(e['text']) for e in ev)} chars | vermelho ok")
