package testsupport

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// StartFakeJev sobe um servidor httptest que responde o wire format do
// System One: toda pergunta recebe uma resposta "saudavel", exceto os ids de
// noul sobrescritos em overrides. Devolve a URL base para TYPESAFE_BASE_URL.
//
// O discriminador "type" viaja em cada pergunta (o MarshalJSON do jev o
// injeta), entao o falso responde pelo tipo declarado, sem adivinhar formato.
func StartFakeJev(t *testing.T, overrides map[string]float64) string {
	t.Helper()

	healthy := map[string]float64{
		"defeito_unico": 0.95, "desenho_em_aberto": 0.05, "cruza_pacotes": 0.1,
		"toca_sensivel": 0.02, "criterio_de_pronto": 0.93,
		"aponta_arquivo_linha": 0.97, "cita_fonte_do_contrato": 0.9,
		"pede_teste_antes_da_correcao": 0.96, "comandos_copiaveis": 0.98,
		"limites_explicitos": 0.94, "pede_relatorio": 0.95,
		"evidencia_necessaria": 0.9, "tarefa_autocontida": 0.9,
	}
	for k, v := range overrides {
		healthy[k] = v
	}

	choices := map[string]string{
		"tipo_de_tarefa":     "correcao_com_teste",
		"dimensao_dominante": "mecanica",
		"permissao":          "smart",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Questions map[string]map[string]any `json:"questions"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)

		answers := map[string]any{}
		for id, q := range req.Questions {
			switch q["type"] {
			case "choice":
				choice, ok := choices[id]
				if !ok {
					// Pergunta nova sem resposta roteirizada: falhar alto
					// evita que um default mudo aprove o que nao foi testado.
					t.Errorf("fakejev: choice sem default: %q", id)
					choice = "correcao_com_teste"
				}
				answers[id] = map[string]any{"type": "choice", "choice": choice, "confidence": 0.9,
					"probabilities": map[string]any{choice: 0.9}}
			case "score":
				answers[id] = map[string]any{"type": "score", "score": 1.2, "confidence": 0.8}
			default: // noul e qualquer pergunta sem tipo
				p, ok := healthy[id]
				if !ok {
					t.Errorf("fakejev: noul sem default: %q", id)
				}
				answers[id] = map[string]any{"type": "noul", "noul": p}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1.13.0", "answers": answers,
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 0},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
