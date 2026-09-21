package cli

import (
	"strings"
	"testing"
)

// Medido no piloto de 2026-09-21: o briefing mandava rodar um comando com
// `&&`, a camada de permissao o recusava, e o modelo que obedeceu ao
// briefing foi vetado por sem_progresso. O placar registrou "o modelo nao
// conseguiu" — o modelo obedeceu, quem estava errado era o plano.
func TestComandoEncadeadoNaoChegaAoBriefing(t *testing.T) {
	err := comandosExecutaveis("/tmp/x", []string{
		"go test -run TestCheck ./checker/ && go test ./test/issues/888/",
	})
	if err == nil {
		t.Fatal("quero erro: esse comando seria recusado na execucao")
	}
	if !strings.Contains(err.Error(), "&&") {
		t.Errorf("o erro precisa mostrar o comando: %v", err)
	}
	if !strings.Contains(err.Error(), "um comando por flag") {
		t.Errorf("o erro precisa dizer o que fazer: %v", err)
	}
}

// Comandos normais passam, e a lista vazia tambem: o plan aceita job sem
// comando declarado.
func TestComandosNormaisPassam(t *testing.T) {
	for _, cmds := range [][]string{
		{"go test ./...", "go vet ./...", "go test -run TestX ./pkg/"},
		{},
		{""},
	} {
		if err := comandosExecutaveis("/tmp/x", cmds); err != nil {
			t.Errorf("%v: %v", cmds, err)
		}
	}
}

// A negacao dura tambem e pega aqui: um comando proibido no briefing e o
// mesmo erro, so que descoberto antes de custar um run.
func TestComandoProibidoNaoChegaAoBriefing(t *testing.T) {
	if err := comandosExecutaveis("/tmp/x", []string{"curl https://exemplo"}); err == nil {
		t.Error("curl esta na negacao dura e nao pode entrar no briefing")
	}
}
