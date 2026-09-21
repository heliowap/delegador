package job

import (
	"strings"
	"testing"
)

// Um id fora do formato newID nunca pode chegar ao filesystem: "../../x"
// escaparia da raiz do store para status/result/run. O guarda vive em Load
// porque todos os subcomandos passam por ele.
func TestLoadRejeitaIdForaDoFormato(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, id := range []string{
		"../../etc",
		"..",
		".",
		"job-../../x",
		"job-ABCDEF0123",  // maiusculo nao e hex do newID
		"job-012345678",   // 9 chars
		"job-01234567890", // 11 chars
		"job-012345678g",  // g nao e hex
		"qualquer",
		"",
	} {
		_, err := Load(id)
		if err == nil {
			t.Errorf("Load(%q) = nil erro, quero rejeicao de formato", id)
			continue
		}
		if !strings.Contains(err.Error(), "id invalido") {
			t.Errorf("Load(%q) erro %q — quero a rejeicao de formato, nao outra causa", id, err)
		}
	}
}

// Um id bem-formado passa a validacao e so entao falha no arquivo ausente —
// prova que o guarda nao barra ids legitimos.
func TestLoadIdValidoPassaAValidacao(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	_, err := Load("job-0123456789")
	if err == nil {
		t.Fatal("job inexistente tinha que falhar")
	}
	if strings.Contains(err.Error(), "id invalido") {
		t.Fatalf("id bem-formado nao pode cair no guarda de formato: %v", err)
	}
}
