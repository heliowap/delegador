package devin

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Effort e o nivel de esforco pedido, traduzido em sufixo de modelo.
type Effort string

const (
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
	EffortMax    Effort = "max"
)

// Model e uma entrada de `devin models list`.
type Model struct {
	UID           string
	Label         string
	Family        string
	ContextTokens int
	Free          bool
	InputUSDPerM  float64
}

var (
	familyRe = regexp.MustCompile(`^(\S.*)\s+\(([a-z0-9][a-z0-9.\-]*)\)\s*$`)
	modelRe  = regexp.MustCompile(`^\s{2,}(\S+)\s{2,}(.+?)\s*\[([^\]]*)\]\s*$`)
	ctxRe    = regexp.MustCompile(`([\d,]+)\s*(K|M)?\s*context`)
	priceRe  = regexp.MustCompile(`\$([\d.]+)\s*/\s*1M\s+Input`)
)

// ParseModelList le a saida de `devin models list`. Linhas que nao casam com
// o formato esperado sao ignoradas em silencio: familias novas nao podem
// derrubar o plugin.
func ParseModelList(r io.Reader) ([]Model, error) {
	var models []Model
	var family string

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "Available models") || strings.HasPrefix(line, "Pass a family") {
			continue
		}
		if m := familyRe.FindStringSubmatch(line); m != nil {
			family = m[2]
			continue
		}
		m := modelRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		attrs := m[3]
		ctx := ctxRe.FindStringSubmatch(attrs)
		if ctx == nil {
			continue // formato desconhecido: ignora
		}
		tokens, err := parseContext(ctx[1], ctx[2])
		if err != nil {
			continue
		}
		mod := Model{
			UID:           m[1],
			Label:         strings.TrimSpace(m[2]),
			Family:        family,
			ContextTokens: tokens,
			Free:          strings.Contains(attrs, "Free"),
		}
		if p := priceRe.FindStringSubmatch(attrs); p != nil {
			mod.InputUSDPerM, _ = strconv.ParseFloat(p[1], 64)
		}
		models = append(models, mod)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("lendo models list: %w", err)
	}
	return models, nil
}

func parseContext(num, unit string) (int, error) {
	n, err := strconv.Atoi(strings.ReplaceAll(num, ",", ""))
	if err != nil {
		return 0, err
	}
	switch unit {
	case "K":
		return n * 1000, nil
	case "M":
		return n * 1_000_000, nil
	default:
		return n, nil
	}
}

// ErrNoModel indica que nenhum modelo atende ao esforco pedido.
var ErrNoModel = errors.New("nenhum modelo disponivel para o esforco pedido")

// SelectModel escolhe o modelo para o esforco pedido: gratuito primeiro,
// depois o mais barato por token de entrada, desempatando pelo maior contexto.
// Nenhum UID aparece hardcoded — a escolha vem sempre da lista viva.
func SelectModel(models []Model, want Effort) (Model, error) {
	var cands []Model
	for _, m := range models {
		if strings.HasSuffix(m.UID, "-"+string(want)) {
			cands = append(cands, m)
		}
	}
	if len(cands) == 0 {
		// Familia sem sufixo de esforco: aceita qualquer uma, para nao travar.
		cands = models
	}
	if len(cands) == 0 {
		return Model{}, ErrNoModel
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.Free != b.Free {
			return a.Free
		}
		if a.InputUSDPerM != b.InputUSDPerM {
			return a.InputUSDPerM < b.InputUSDPerM
		}
		return a.ContextTokens > b.ContextTokens
	})
	return cands[0], nil
}
