package job

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	WritePrefixes []string      `json:"write_prefixes"`
	AllowCommands []string      `json:"allow_commands"`
	PID           int           `json:"pid"`
	PGID          int           `json:"pgid"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	CancelReason  *CancelReason `json:"cancel_reason,omitempty"`

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

func lockPath(root, worktree string) string {
	sum := hex.EncodeToString([]byte(filepath.Clean(worktree)))
	if len(sum) > 40 {
		sum = sum[:40]
	}
	return filepath.Join(root, ".locks", sum+".lock")
}

// Create cria um job novo e trava a worktree. Duas delegacoes na mesma pasta
// e o erro que o protocolo proibe explicitamente.
func Create(worktree string) (*Job, error) {
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

	lock := lockPath(root, worktree)
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			owner, _ := os.ReadFile(lock)
			return nil, fmt.Errorf("worktree %s ja esta em uso pelo job %s", worktree, string(owner))
		}
		return nil, err
	}
	_, _ = f.WriteString(j.ID)
	f.Close()

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

// Load le um job pelo id.
func Load(id string) (*Job, error) {
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

// Release solta a trava da worktree do job.
func Release(id string) error {
	j, err := Load(id)
	if err != nil {
		return err
	}
	root, err := Root()
	if err != nil {
		return err
	}
	if err := os.Remove(lockPath(root, j.Worktree)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
