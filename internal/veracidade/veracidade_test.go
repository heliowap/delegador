package veracidade

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/heliowap/delegador/internal/agent"
	"github.com/heliowap/delegador/internal/jev"
	"github.com/heliowap/delegador/internal/tools"
)

type askerFixo struct {
	escolha   string
	confianca float64
	requests  int
}

func (a *askerFixo) Ask(_ context.Context, _ any, qs map[string]jev.Question) (jev.Result, error) {
	a.requests++
	corpo := map[string]any{}
	for id := range qs {
		corpo[id] = map[string]any{"type": "choice", "choice": a.escolha,
			"confidence": a.confianca, "probabilities": map[string]any{a.escolha: a.confianca}}
	}
	raw, _ := json.Marshal(corpo)
	var ans jev.Answers
	_ = json.Unmarshal(raw, &ans)
	return jev.Result{Answers: ans, Usage: jev.Usage{InputTokens: 200}}, nil
}

func turnoExec(cmd, saida string) agent.Turn {
	var t agent.Turn
	t.Message.ToolCalls = []tools.Call{{Name: "exec", Args: map[string]string{"command": cmd}}}
	t.Results = []tools.Result{{Output: saida}}
	return t
}

// O caso que importa: o relatorio afirma ter rodado a suite e o trace nao
// registra execucao nenhuma dela. Isso se decide por casamento literal,
// sem modelo — e o modelo nem e consultado sobre esse comando.
func TestComandoCitadoQueNuncaRodouEPegoSemModelo(t *testing.T) {
	relatorio := "Rodei `go test ./...` e passou tudo. Depois `go vet ./...`, sem saida."
	turns := []agent.Turn{turnoExec("go vet ./...", "")}
	a := &askerFixo{escolha: Sustentado, confianca: 0.95}

	vs, _, err := Conferir(context.Background(), a, relatorio,
		[]string{"go test ./...", "go vet ./..."}, turns)
	if err != nil {
		t.Fatal(err)
	}
	porComando := map[string]Veredito{}
	for _, v := range vs {
		porComando[v.Comando] = v
	}
	if got := porComando["go test ./..."].Estado; got != SemExecucao {
		t.Errorf("suite citada e nao executada: quero %q, tenho %q", SemExecucao, got)
	}
	if !porComando["go test ./..."].Suspeito() {
		t.Error("comando citado sem execucao precisa chamar atencao de quem le")
	}
	if got := porComando["go vet ./..."].Estado; got != Sustentado {
		t.Errorf("go vet rodou e confere: quero %q, tenho %q", Sustentado, got)
	}
	if a.requests != 1 {
		t.Errorf("quero UM request para todos os comandos que sobrevivem, tenho %d", a.requests)
	}
}

// Comando que o relatorio nem cita nao e acusacao: e omissao.
func TestComandoNaoCitadoNaoVaiAoModelo(t *testing.T) {
	a := &askerFixo{escolha: Sustentado, confianca: 0.95}
	vs, _, err := Conferir(context.Background(), a, "fiz o que pediram",
		[]string{"go test ./..."}, []agent.Turn{turnoExec("go test ./...", "ok")})
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0].Estado != NaoMencionado {
		t.Errorf("quero nao_mencionado, tenho %+v", vs)
	}
	if vs[0].Suspeito() {
		t.Error("omissao nao e o mesmo que afirmacao falsa")
	}
	if a.requests != 0 {
		t.Errorf("nada sobrou para o modelo julgar, mas houve %d request", a.requests)
	}
}

// Confianca abaixo do limiar nao acusa nem absolve.
func TestConfiancaBaixaViraIncerto(t *testing.T) {
	a := &askerFixo{escolha: Contradito, confianca: 0.55}
	vs, _, err := Conferir(context.Background(), a, "rodei `go test ./...`",
		[]string{"go test ./..."}, []agent.Turn{turnoExec("go test ./...", "FAIL")})
	if err != nil {
		t.Fatal(err)
	}
	if vs[0].Estado != Incerto {
		t.Errorf("quero incerto, tenho %q", vs[0].Estado)
	}
	if vs[0].Suspeito() {
		t.Error("incerto nao pode ser lido como acusacao")
	}
	if vs[0].Confianca != 0.55 {
		t.Errorf("a confianca precisa ir junto para quem le: %v", vs[0].Confianca)
	}
}

// O relatorio descreve o estado FINAL: um comando que falhou no comeco e
// passou no fim nao torna o relatorio falso. Vale a ultima execucao.
func TestValeAUltimaExecucao(t *testing.T) {
	turns := []agent.Turn{
		turnoExec("go test ./...", "FAIL: vermelho inicial"),
		turnoExec("go test ./...", "ok  github.com/x 0.3s"),
	}
	var visto string
	espiao := askerEspiaoSaida{&visto}
	_, _, err := Conferir(context.Background(), espiao, "rodei `go test ./...` e ficou verde",
		[]string{"go test ./..."}, turns)
	if err != nil {
		t.Fatal(err)
	}
	if visto != "ok  github.com/x 0.3s" {
		t.Errorf("quero a ultima saida, tenho %q", visto)
	}
}

type askerEspiaoSaida struct{ out *string }

func (e askerEspiaoSaida) Ask(_ context.Context, state any, qs map[string]jev.Question) (jev.Result, error) {
	m := state.(map[string]any)
	lista := m["comandos"].([]map[string]any)
	*e.out = lista[0]["saida_real"].(string)
	corpo := map[string]any{}
	for id := range qs {
		corpo[id] = map[string]any{"type": "choice", "choice": Sustentado, "confidence": 0.95}
	}
	raw, _ := json.Marshal(corpo)
	var ans jev.Answers
	_ = json.Unmarshal(raw, &ans)
	return jev.Result{Answers: ans}, nil
}
