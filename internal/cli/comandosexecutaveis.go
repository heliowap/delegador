package cli

import (
	"fmt"
	"strings"

	"github.com/heliowap/delegador/internal/tools"
)

// comandosExecutaveis confere que TODO comando que o plan vai escrever no
// briefing sobreviveria a propria camada de permissao do job.
//
// Medido em 2026-09-21, no piloto de capacidade: a tarefa media declarava
// `go test -run TestCheck ./checker/ && go test ./test/issues/888/`. O
// briefing mandava roda-lo, a allowlist o continha inteiro, e tools.Allow o
// recusava — `&&` e metacaractere de shell, e essa recusa e regra de
// seguranca que nao se negocia.
//
// O resultado foi o pior tipo de falha: o delegador entregou ao executor uma
// ordem que ele estava proibido de cumprir. O opus-4-6 tentou o comando
// literal do briefing, levou recusa, tentou de novo e foi vetado por
// `sem_progresso` em seis turnos. O placar registrou "o modelo nao
// conseguiu". O modelo obedeceu; quem estava errado era o plano.
//
// Duas etapas minhas discordando nao e julgamento, e acoplamento — e
// acoplamento se resolve em codigo, na hora de montar, nao na hora de
// executar.
func comandosExecutaveis(worktree string, cmds []string) error {
	p := tools.Policy{Worktree: worktree, AllowCommands: cmds}
	for _, c := range cmds {
		if strings.TrimSpace(c) == "" {
			continue
		}
		d := tools.Allow(tools.Call{Name: "exec", Args: map[string]string{"command": c}}, p)
		if !d.Allowed {
			return fmt.Errorf("o comando %q entraria no briefing e seria recusado na execucao: %s.\n"+
				"  Declare um comando por flag, sem encadeamento: o executor roda um de cada vez",
				c, d.Reason)
		}
	}
	return nil
}
