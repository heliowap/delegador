// writeprobe.go — a escrita do roster: regrava o bloco sondado de um modelo
// no arquivo YAML. Cirúrgico, linha a linha, porque o arquivo é curadoria
// humana cheia de comentários e notas que um reparse + reserialização
// apagaria.
package roster

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// camposSondado são as chaves que a sondagem escreve sob `sondado:` —
// inclusive `em`, a data desta sondagem, que prevalece sobre o
// as_of_sondagem do arquivo para ESTE modelo (ver parse).
var camposSondado = []string{"tool_call", "reasoning_content", "tokens_base", "latencia_s", "em"}

// WriteProbe grava no arquivo path o resultado da sondagem p do modelo id.
// Substitui os campos existentes sob `sondado:` e acrescenta os que faltam;
// modelo sem bloco `sondado:` ganha um no fim do seu bloco. Nada fora do
// modelo tocado muda — em particular o as_of_sondagem do arquivo, que é a
// data da varredura original e não pode fingir frescura alheia.
func WriteProbe(path string, id string, p Probe) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("roster: lendo %s: %w", path, err)
	}
	linhas := strings.Split(string(raw), "\n")

	inicio, fim, err := blocoDoModelo(linhas, id)
	if err != nil {
		return err
	}

	valores := map[string]string{
		"tool_call":         strconv.FormatBool(p.ToolCall),
		"reasoning_content": strconv.FormatBool(p.ReasoningContent),
		"tokens_base":       strconv.Itoa(p.TokensBase),
		"latencia_s":        strconv.FormatFloat(p.LatenciaS, 'f', -1, 64),
		"em":                `"` + p.Em.Format("2006-01-02") + `"`,
	}

	// Acha o `sondado:` do modelo e os filhos dele: linhas mais indentadas
	// que a chave, até o próximo campo no nível do modelo ou o fim do bloco.
	sondado, fimSondado, indentFilho := -1, -1, 6
	for i := inicio + 1; i < fim; i++ {
		indent, chave, ok := campoYAML(linhas[i])
		if !ok {
			continue // em branco ou comentário: neutro
		}
		if sondado < 0 {
			if indent <= 4 && chave == "sondado" {
				sondado = i
			}
			continue
		}
		if indent <= 4 {
			break // próximo campo do modelo: a seção sondado acabou
		}
		fimSondado = i + 1
		if v, tem := valores[chave]; tem {
			linhas[i] = strings.Repeat(" ", indent) + chave + ": " + v
			indentFilho = indent
			delete(valores, chave)
		}
	}
	if sondado >= 0 && fimSondado < 0 {
		fimSondado = sondado + 1 // `sondado:` sem filhos
	}

	var novas []string
	for _, k := range camposSondado {
		if v, falta := valores[k]; falta {
			novas = append(novas, strings.Repeat(" ", indentFilho)+k+": "+v)
		}
	}
	switch {
	case sondado < 0:
		// Sem bloco sondado: nasce um no fim do bloco do modelo.
		novas = append([]string{"    sondado:"}, novas...)
		linhas = insere(linhas, fim, novas)
	case len(novas) > 0:
		linhas = insere(linhas, fimSondado, novas)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(linhas, "\n")), 0o644); err != nil {
		return fmt.Errorf("roster: gravando %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("roster: renomeando %s: %w", path, err)
	}
	return nil
}

// blocoDoModelo devolve o intervalo [inicio, fim) das linhas do modelo id:
// inicio é a linha `- ` que abre o modelo (com ou sem o id nela), fim é a
// linha que abre o modelo seguinte ou o fim do arquivo.
func blocoDoModelo(linhas []string, id string) (int, int, error) {
	inicio, achou := -1, false
	for i, l := range linhas {
		indent, chave, ok := campoYAML(l)
		if !ok || indent > 4 {
			continue
		}
		if ehAbertura(l) {
			if achou {
				return inicio, i, nil // o próximo `- ` fecha o bloco
			}
			inicio = i
		}
		// O id pode vir na própria linha `- id: x` (campoYAML já tirou o
		// "- ") ou numa linha seguinte do bloco.
		if inicio >= 0 && !achou && chave == "id" && valorDe(l) == id {
			achou = true
		}
	}
	if achou {
		return inicio, len(linhas), nil
	}
	return 0, 0, fmt.Errorf("roster: modelo %q não encontrado", id)
}

// ehAbertura diz se a linha abre um item da lista de modelos (`- ` em
// indent de item, não de filho de mapa).
func ehAbertura(l string) bool {
	sem := tiraComentario(l)
	indent := len(sem) - len(strings.TrimLeft(sem, " "))
	return indent <= 4 && strings.HasPrefix(strings.TrimSpace(sem), "- ")
}

// campoYAML devolve o indent e a chave de uma linha `chave: ...` ou
// `- chave: ...`, e false para linha em branco ou comentário — mesma
// leitura rasa do parse.
func campoYAML(l string) (int, string, bool) {
	sem := tiraComentario(l)
	if strings.TrimSpace(sem) == "" {
		return 0, "", false
	}
	indent := len(sem) - len(strings.TrimLeft(sem, " "))
	campo := strings.TrimSpace(sem)
	if strings.HasPrefix(campo, "- ") {
		campo = strings.TrimSpace(campo[2:])
	}
	k, _, _ := strings.Cut(campo, ":")
	return indent, strings.TrimSpace(k), true
}

// valorDe devolve o valor escalar da linha `chave: valor`, sem aspas.
func valorDe(l string) string {
	campo := strings.TrimSpace(tiraComentario(l))
	if strings.HasPrefix(campo, "- ") {
		campo = strings.TrimSpace(campo[2:])
	}
	_, v, _ := strings.Cut(campo, ":")
	return valorEscalar(strings.TrimSpace(v))
}

func insere(linhas []string, pos int, novas []string) []string {
	out := make([]string, 0, len(linhas)+len(novas))
	out = append(out, linhas[:pos]...)
	out = append(out, novas...)
	return append(out, linhas[pos:]...)
}
