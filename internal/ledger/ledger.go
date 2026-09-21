// Package ledger contabiliza o gasto do executor, gemeo do jev.Ledger:
// JSONL com um registro por linha, gravacao que so guarda e leitura que
// soma. Arquivo separado porque as ordens de grandeza sao diferentes —
// Jev cobra so a entrada a $0.042/M; o executor cobra os dois lados
// (spec §9).
package ledger

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"time"
)

// entry e uma linha de executor.jsonl.
type entry struct {
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	InTokens   int       `json:"input_tokens"`
	OutTokens  int       `json:"output_tokens"`
	USDInMTok  float64   `json:"usd_per_mtok_input"`
	USDOutMTok float64   `json:"usd_per_mtok_output"`
}

// Ledger contabiliza o gasto do executor de um job. A economia precisa ser
// auditavel, nao prometida.
type Ledger struct{ Path string }

// Record acrescenta uma chamada ao ledger. Nao calcula custo: guarda os
// tokens que a API reportou e os precos por milhao vigentes na hora —
// preco gravado junto e o que permite auditar a conta depois.
func (l *Ledger) Record(kind string, inTok, outTok int, usdPerMTokIn, usdPerMTokOut float64) error {
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	raw, err := json.Marshal(entry{
		At:         time.Now(),
		Kind:       kind,
		InTokens:   inTok,
		OutTokens:  outTok,
		USDInMTok:  usdPerMTokIn,
		USDOutMTok: usdPerMTokOut,
	})
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	return err
}

// Total soma o custo de todos os registros: tokens de entrada vezes o
// preco de entrada mais tokens de saida vezes o preco de saida, por
// registro. Arquivo ausente significa zero, nao erro.
func (l *Ledger) Total() (usd float64, err error) {
	f, err := os.Open(l.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue // linha corrompida nao invalida a conta inteira
		}
		usd += float64(e.InTokens)/1e6*e.USDInMTok +
			float64(e.OutTokens)/1e6*e.USDOutMTok
	}
	return usd, sc.Err()
}
