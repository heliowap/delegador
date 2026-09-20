package jev

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"time"
)

// InputUSDPerMillion e o preco publicado do jev-latest. Saida e gratuita.
const InputUSDPerMillion = 0.042

// entry e uma linha de jev.jsonl.
type entry struct {
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Version  string    `json:"questions_version"`
	InTokens int       `json:"input_tokens"`
}

// Ledger contabiliza o gasto com Jev de um job. A economia precisa ser
// auditavel, nao prometida.
type Ledger struct{ Path string }

// Record acrescenta uma chamada ao ledger.
func (l *Ledger) Record(kind string, u Usage) error {
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	raw, err := json.Marshal(entry{
		At:       time.Now(),
		Kind:     kind,
		Version:  QuestionsVersion,
		InTokens: u.InputTokens,
	})
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	return err
}

// Total soma tokens e custo. Arquivo ausente significa zero, nao erro.
func (l *Ledger) Total() (int, float64, error) {
	f, err := os.Open(l.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	defer f.Close()

	var tokens int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue // linha corrompida nao invalida a conta inteira
		}
		tokens += e.InTokens
	}
	if err := sc.Err(); err != nil {
		return 0, 0, err
	}
	return tokens, float64(tokens) / 1_000_000 * InputUSDPerMillion, nil
}
