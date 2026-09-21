package job

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// State e o estado do job.
type State string

const (
	StatePlanned   State = "planned"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Terminal indica estado do qual nao se sai.
func (s State) Terminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateCancelled
}

// CancelReason e o motivo legivel de um cancelamento pelo watchdog.
type CancelReason struct {
	Signal        string    `json:"signal"`
	Probability   float64   `json:"probability"`
	TurnExcerpt   string    `json:"turn_excerpt"`
	ResumeCommand string    `json:"resume_command"`
	At            time.Time `json:"at"`
}

// Job e o estado persistido de uma delegacao.
type Job struct {
	ID         string `json:"id"`
	State      State  `json:"state"`
	Worktree   string `json:"worktree"`
	Branch     string `json:"branch"`
	Model      string `json:"model"`
	Permission string `json:"permission"`

	// O que o run precisa para remontar o tools.Policy do job: a escrita vai
	// para os prefixos declarados e a allowlist de comando vem dos comandos
	// do briefing. Worktree ja esta no campo acima.
	WritePrefixes []string `json:"write_prefixes"`
	AllowCommands []string `json:"allow_commands"`

	// O que o run precisa para montar o verify.Config sem repedir flags:
	// os papeis dos comandos (qual e o teste, qual a suite, qual o lint) e
	// os globs que classificam arquivo de teste na sonda de mutacao.
	TestCmd   string   `json:"test_cmd"`
	SuiteCmd  string   `json:"suite_cmd"`
	LintCmd   string   `json:"lint_cmd"`
	TestGlobs []string `json:"test_globs"`

	// O registro da rota: a cascata re-roteia a partir de Percentil sem
	// repreguntar ao Jev, e o relatorio explica a escolha sem refazer a conta.
	Percentil   float64 `json:"percentil"`
	Dimensao    string  `json:"dimensao"`
	Autocontida float64 `json:"autocontida"`

	// Escaladas conta quantas escaladas a tarefa ja consumiu. Persistido no
	// mesmo Save que atualiza Percentil e Model na escalada: um restart
	// entre o degrau do percentil e o registro da tentativa nao pode
	// reaplicar o degrau nem perder o modelo novo.
	Escaladas int `json:"escaladas"`

	PID          int           `json:"pid"`
	PGID         int           `json:"pgid"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	CancelReason *CancelReason `json:"cancel_reason,omitempty"`

	dir string
}

// Dir e o diretorio do job.
func (j *Job) Dir() string { return j.dir }

// Path devolve um caminho dentro do diretorio do job.
func (j *Job) Path(name string) string { return filepath.Join(j.dir, name) }

// Save grava job.json atomicamente.
func (j *Job) Save() error {
	j.UpdatedAt = time.Now()
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := j.Path("job.json.tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, j.Path("job.json"))
}

func newID() string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return "job-" + hex.EncodeToString(b)
}

// canonical devolve o caminho absoluto com symlinks resolvidos. A trava e
// a politica comparam caminho por string: duas grafias da mesma pasta
// ("/tmp/wt" vs "/private/tmp/wt", ou via symlink) tem que cair na mesma
// worktree, senao dois jobs dividem o mesmo diretorio — o erro proibido.
func canonical(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// lockPath e o arquivo de trava da worktree: sha256 do caminho canonico em
// hex completo — truncar so economizaria nome a custa de colisao, e a trava
// e a primitiva que impede dois executores na mesma pasta. O hex tambem nao
// vaza a localizacao no nome do arquivo, como o caminho cru vazaria.
// Worktree irresoluvel cai na grafia absoluta: Create barra esse caso antes
// de travar; o fallback cobre chamadas diretas com path de versao antiga.
func lockPath(root, worktree string) string {
	canon, err := canonical(worktree)
	if err != nil {
		canon, _ = filepath.Abs(worktree)
	}
	sum := sha256.Sum256([]byte(canon))
	return filepath.Join(root, ".locks", hex.EncodeToString(sum[:])+".lock")
}

// Create cria um job novo e trava a worktree. Duas delegacoes na mesma pasta
// e o erro que o protocolo proibe explicitamente.
func Create(worktree string) (*Job, error) {
	// Canoniza primeiro e guarda a forma canonica: j.Worktree alimenta a
	// trava, o cmd.Dir das ferramentas e a politica — uma forma so.
	canon, err := canonical(worktree)
	if err != nil {
		return nil, fmt.Errorf("worktree %s nao resolve: %w", worktree, err)
	}
	if info, err := os.Stat(canon); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("worktree %s nao e um diretorio existente", worktree)
	}
	worktree = canon

	root, err := Root()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, ".locks"), 0o755); err != nil {
		return nil, err
	}

	j := &Job{
		ID:        newID(),
		State:     StatePlanned,
		Worktree:  worktree,
		CreatedAt: time.Now(),
	}
	j.dir = filepath.Join(root, j.ID)

	lock := lockPath(root, j.Worktree)
	if err := j.Reacquire(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(j.dir, 0o755); err != nil {
		_ = os.Remove(lock)
		return nil, err
	}
	if err := j.Save(); err != nil {
		_ = os.Remove(lock)
		return nil, err
	}
	return j, nil
}

// Reacquire (re)cria a trava da worktree em nome do job: ausente, cria;
// existente com o mesmo id, e idempotente (a trava ja e nossa); existente
// com id alheio, devolve erro nomeando o dono. A retomada de um job cuja
// trava ja foi solta usa isto para nunca rodar destravada.
func (j *Job) Reacquire() error {
	root, err := Root()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".locks"), 0o755); err != nil {
		return err
	}
	lock := lockPath(root, j.Worktree)
	for range 2 {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.WriteString(j.ID)
			return f.Close()
		}
		if !os.IsExist(err) {
			return err
		}
		owner, rerr := os.ReadFile(lock)
		switch {
		case rerr == nil && string(owner) == j.ID:
			return nil
		case rerr == nil:
			return fmt.Errorf("worktree %s ja esta em uso pelo job %s", j.Worktree, string(owner))
		case !os.IsNotExist(rerr):
			return rerr
		}
		// A trava sumiu entre o create e a leitura — repete uma vez.
	}
	return fmt.Errorf("trava da worktree %s instavel", j.Worktree)
}

// idRe e o formato que newID gera. Sem esta validacao um id como
// "../../x" escapava da raiz do store: status/result liam diretorios
// arbitrarios e run escrevia/executava um job.json plantado.
var idRe = regexp.MustCompile(`^job-[0-9a-f]{10}$`)

// Load le um job pelo id.
func Load(id string) (*Job, error) {
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("job %q: id invalido", id)
	}
	root, err := Root()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, id)
	raw, err := os.ReadFile(filepath.Join(dir, "job.json"))
	if err != nil {
		return nil, fmt.Errorf("job %s: %w", id, err)
	}
	var j Job
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, fmt.Errorf("job %s: job.json invalido: %w", id, err)
	}
	j.dir = dir
	return &j, nil
}

// Release solta a trava da worktree do job — mas so a dele: a trava que
// existe com outro id e de outro job, e Release a deixa intacta. Ausente
// ou alheia, nao e erro: nao havia trava nossa para soltar.
func Release(id string) error {
	j, err := Load(id)
	if err != nil {
		return err
	}
	root, err := Root()
	if err != nil {
		return err
	}
	lock := lockPath(root, j.Worktree)
	owner, err := os.ReadFile(lock)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if string(owner) != j.ID {
		return nil // trava alheia: nao e nossa para soltar
	}
	return os.Remove(lock)
}
