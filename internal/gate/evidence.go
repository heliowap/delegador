package gate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Evidence e um item de contexto verbatim vindo do orquestrador.
type Evidence struct {
	Kind string `json:"kind"` // trecho | erro | comando | fonte
	Ref  string `json:"ref"`  // arquivo:linha, quando houver
	Text string `json:"text"`
	Kept bool   `json:"kept"`
}

// ParseEvidence le o JSONL de evidencias.
func ParseEvidence(r io.Reader) ([]Evidence, error) {
	var out []Evidence
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e Evidence
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("evidencia linha %d: %w", line, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}
