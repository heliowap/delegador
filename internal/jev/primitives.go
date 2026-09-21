package jev

import (
	"encoding/json"
	"strconv"
)

// Question e uma pergunta do System One. Interface selada: so os tres
// primitivos do TypeSafe a implementam.
type Question interface{ isQuestion() }

// NoulCriteria descreve o que conta como sim e como nao.
type NoulCriteria struct {
	True  string `json:"true"`
	False string `json:"false"`
}

// Noul pergunta sim ou nao e devolve probabilidade de sim.
type Noul struct {
	Instructions string        `json:"instructions"`
	Criteria     *NoulCriteria `json:"criteria,omitempty"`
}

// Choice escolhe uma entre opcoes nomeadas.
type Choice struct {
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// Score posiciona numa escala ordenada de 2 a 10 niveis, do menor ao maior.
type Score struct {
	Instructions string   `json:"instructions"`
	Criteria     []string `json:"criteria"`
}

func (Noul) isQuestion()   {}
func (Choice) isQuestion() {}
func (Score) isQuestion()  {}

// O wire format do System One exige o discriminador "type" em cada
// pergunta. Os MarshalJSON abaixo o injetam na serializacao.

type noulWire struct {
	Type         string        `json:"type"`
	Instructions string        `json:"instructions"`
	Criteria     *NoulCriteria `json:"criteria,omitempty"`
}

func (n Noul) MarshalJSON() ([]byte, error) {
	return json.Marshal(noulWire{Type: "noul", Instructions: n.Instructions, Criteria: n.Criteria})
}

type choiceWire struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

func (ch Choice) MarshalJSON() ([]byte, error) {
	return json.Marshal(choiceWire{Type: "choice", Instructions: ch.Instructions, Criteria: ch.Criteria})
}

type scoreWire struct {
	Type         string   `json:"type"`
	Instructions string   `json:"instructions"`
	Criteria     []string `json:"criteria"`
}

func (s Score) MarshalJSON() ([]byte, error) {
	return json.Marshal(scoreWire{Type: "score", Instructions: s.Instructions, Criteria: s.Criteria})
}

// ChoiceAnswer e a resposta de uma Choice.
type ChoiceAnswer struct {
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// ScoreAnswer e a resposta de um Score.
type ScoreAnswer struct {
	Score         float64           `json:"score"`
	Confidence    float64           `json:"confidence"`
	Probabilities []float64         `json:"probabilities"`
	Legend        map[string]string `json:"legend"`
}

type rawAnswer struct {
	Type          string            `json:"type"`
	Noul          float64           `json:"noul"`
	Choice        string            `json:"choice"`
	Score         float64           `json:"score"`
	Confidence    float64           `json:"confidence"`
	Probabilities any               `json:"probabilities"`
	Legend        map[string]string `json:"legend"`
}

// Answers sao as respostas indexadas pelos IDs das perguntas.
type Answers map[string]rawAnswer

// NoulOf devolve a probabilidade de sim do noul com este id.
func (a Answers) NoulOf(id string) (float64, bool) {
	r, ok := a[id]
	if !ok || r.Type != "noul" {
		return 0, false
	}
	return r.Noul, true
}

// ChoiceOf devolve a resposta da choice com este id.
func (a Answers) ChoiceOf(id string) (ChoiceAnswer, bool) {
	r, ok := a[id]
	if !ok || r.Type != "choice" {
		return ChoiceAnswer{}, false
	}
	probs := map[string]float64{}
	if m, ok := r.Probabilities.(map[string]any); ok {
		for k, v := range m {
			if f, ok := v.(float64); ok {
				probs[k] = f
			}
		}
	}
	return ChoiceAnswer{Choice: r.Choice, Confidence: r.Confidence, Probabilities: probs}, true
}

// ScoreOf devolve a resposta do score com este id.
func (a Answers) ScoreOf(id string) (ScoreAnswer, bool) {
	r, ok := a[id]
	if !ok || r.Type != "score" {
		return ScoreAnswer{}, false
	}
	return ScoreAnswer{Score: r.Score, Confidence: r.Confidence,
		Probabilities: distribuicao(r.Probabilities), Legend: r.Legend}, true
}

// distribuicao le a massa por nivel de um Score nas DUAS formas que o
// servico usa: lista posicional e objeto com o indice do nivel na chave.
//
// Medido em 2026-09-21 contra a API real: ela devolve o objeto
// (`{"0":0.01,"1":0.58,...}`), e a versao anterior so aceitava a lista —
// o type assertion falhava calado e Probabilities vinha nil em producao,
// enquanto os testes passavam porque a fixture usava lista. Nada quebrava
// porque nada consumia a distribuicao ainda; guardar a distribuicao para
// recalibrar e justamente o que passa a consumi-la.
func distribuicao(v any) []float64 {
	switch p := v.(type) {
	case []any:
		out := make([]float64, 0, len(p))
		for _, x := range p {
			f, _ := x.(float64)
			out = append(out, f)
		}
		return out
	case map[string]any:
		// Chave e o indice do nivel: o tamanho sai da maior chave, para
		// que um nivel de massa zero omitido nao desloque os outros.
		maior := -1
		vals := make(map[int]float64, len(p))
		for k, x := range p {
			i, err := strconv.Atoi(k)
			if err != nil {
				return nil
			}
			f, _ := x.(float64)
			vals[i] = f
			if i > maior {
				maior = i
			}
		}
		if maior < 0 {
			return nil
		}
		out := make([]float64, maior+1)
		for i, f := range vals {
			out[i] = f
		}
		return out
	}
	return nil
}
