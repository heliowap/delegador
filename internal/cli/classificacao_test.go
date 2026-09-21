package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/route"
)

// O ponto do arquivo e guardar o que a rota DESCARTA na hora de decidir:
// confianca e distribuicao. Se eles nao sobreviverem ao JSON, o arquivo nao
// serve para recalibrar nada.
func TestClassificacaoGuardaConfiancaEDistribuicao(t *testing.T) {
	var ans jev.Answers
	if err := json.Unmarshal([]byte(`{
		"dimensao_dominante":{"type":"choice","choice":"raciocinio","confidence":0.42,
			"probabilities":{"raciocinio":0.5,"mecanica":0.4,"agentica":0.1}},
		"complexidade":{"type":"score","score":2.1,"confidence":0.55,"probabilities":[0.1,0.2,0.4,0.3]},
		"sutileza":{"type":"score","score":3.0,"confidence":0.9,"probabilities":[0,0,0,1]}
	}`), &ans); err != nil {
		t.Fatal(err)
	}

	p := filepath.Join(t.TempDir(), "classificacao.json")
	var errb bytes.Buffer
	gravaClassificacao(p, route.Classificacao{
		Dimensao: route.Raciocinio, Percentil: 0.7, Volume: 1.4,
		ConfiancaDimensao: 0.42, ConfiancaComplexidade: 0.55, Bruto: ans}, &errb)
	if errb.Len() > 0 {
		t.Fatalf("stderr: %s", errb.String())
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var lido classificacaoEmDisco
	if err := json.Unmarshal(b, &lido); err != nil {
		t.Fatal(err)
	}
	if lido.ConfiancaDimensao != 0.42 {
		t.Errorf("confianca da dimensao nao sobreviveu: %v", lido.ConfiancaDimensao)
	}
	ch, ok := lido.Respostas.ChoiceOf("dimensao_dominante")
	if !ok || ch.Confidence != 0.42 || ch.Probabilities["mecanica"] != 0.4 {
		t.Errorf("a distribuicao da choice nao sobreviveu: %+v", ch)
	}
	sa, ok := lido.Respostas.ScoreOf("complexidade")
	if !ok || sa.Confidence != 0.55 || len(sa.Probabilities) != 4 {
		t.Errorf("a distribuicao do score nao sobreviveu: %+v", sa)
	}
	// Os atomos ainda nao decidem nada, e e justamente por isso que
	// precisam estar no disco: sem eles guardados nao ha como testar um
	// peso novo sem reexecutar os jobs.
	if _, ok := lido.Respostas.ScoreOf("sutileza"); !ok {
		t.Error("o atomo sutileza sumiu; sem os atomos o arquivo nao recalibra nada")
	}
	if lido.QuestionsVersion == "" {
		t.Error("sem a versao das perguntas, comparar jobs de epocas diferentes mente")
	}
}
