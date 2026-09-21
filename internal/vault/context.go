package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DirMarkerFile pins a directory (and everything under it) to a vault.
	DirMarkerFile = ".kc-vault"
	// EnvVaultVar overrides every other source of the active vault.
	EnvVaultVar = "KC_VAULT"
)

// Sources of the resolved active vault, in precedence order.
const (
	SourceEnv     = "env"
	SourceDir     = "dir"
	SourceFile    = "file"
	SourceDefault = "default"
)

// ActiveVault returns the vault kc should act on when no --vault flag is given.
func (m *Manager) ActiveVault() string {
	name, _, err := m.ActiveVaultContext()
	if err != nil || name == "" {
		return DefaultVault
	}
	return name
}

// ActiveVaultContext resolves the active vault and reports where it came from:
// the KC_VAULT environment variable, the nearest .kc-vault marker walking up
// from the working directory, the persisted active vault, or the default.
//
// Resolution happens inside kc on every invocation — there is no shell hook to
// install and nothing to keep in sync, so `cd` into a project simply works.
func (m *Manager) ActiveVaultContext() (string, string, error) {
	if name := strings.TrimSpace(os.Getenv(EnvVaultVar)); name != "" {
		return name, SourceEnv, nil
	}

	name, _, err := m.DirMarker()
	if err != nil {
		return "", "", err
	}
	if name != "" {
		return name, SourceDir, nil
	}

	if persisted := m.activeVaultFromFile(); persisted != "" {
		return persisted, SourceFile, nil
	}
	return DefaultVault, SourceDefault, nil
}

// DirMarker returns the vault named by the nearest .kc-vault file and the path
// of that file. Empty name means no marker was found.
func (m *Manager) DirMarker() (string, string, error) {
	dir := m.workDir()
	if dir == "" {
		return "", "", nil
	}

	for {
		path := filepath.Join(dir, DirMarkerFile)
		data, err := readFileIfExists(path)
		if err != nil {
			return "", "", err
		}
		if data != nil {
			name := strings.TrimSpace(string(data))
			if name != "" {
				return name, path, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", nil
		}
		dir = parent
	}
}

// UseDir pins a directory to a vault by writing a .kc-vault marker.
func (m *Manager) UseDir(name, dir string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if err := m.requireVault(name); err != nil {
		return err
	}
	if dir == "" {
		dir = m.workDir()
	}
	if dir == "" {
		return fmt.Errorf("vault: cannot resolve a directory to pin")
	}
	return writeFile600(filepath.Join(dir, DirMarkerFile), name+"\n")
}

// ClearDir removes a directory's .kc-vault marker. Missing marker is not an error.
func (m *Manager) ClearDir(dir string) error {
	if dir == "" {
		dir = m.workDir()
	}
	if dir == "" {
		return fmt.Errorf("vault: cannot resolve a directory to unpin")
	}

	err := os.Remove(filepath.Join(dir, DirMarkerFile))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("vault: remove marker in %q: %w", dir, err)
	}
	return nil
}

func (m *Manager) workDir() string {
	if m.WorkDir != "" {
		return m.WorkDir
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

func (m *Manager) activeVaultFromFile() string {
	data, err := os.ReadFile(m.activeVaultPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
