package job

import (
	"os"
	"path/filepath"
)

// Root e a raiz do job store, respeitando XDG_STATE_HOME.
func Root() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "devin-plugin-cc", "jobs"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "devin-plugin-cc", "jobs"), nil
}
