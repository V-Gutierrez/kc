package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	shellZsh  = "zsh"
	shellBash = "bash"
	shellFish = "fish"

	migratedPrefix = "#kc-migrated# "
	kcBeginMarker  = "# BEGIN kc"
	kcEndMarker    = "# END kc"
)

type detectedSecret struct {
	Line    int
	Key     string
	Value   string
	RawLine string
}

func newSetupCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Migrate plaintext shell secrets into kc and install shell init",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")

			shell, err := detectCurrentShell()
			if err != nil {
				return err
			}

			rcPath, err := primaryRCPath(shell)
			if err != nil {
				return err
			}

			content, err := os.ReadFile(rcPath)
			if err != nil {
				if os.IsNotExist(err) {
					content = nil
				} else {
					return fmt.Errorf("setup: read %q: %w", rcPath, err)
				}
			}

			secrets := detectSecretsFromContent(string(content), shell)
			if len(secrets) == 0 {
				if shell == shellFish {
					if err := ensureShellInit(shell); err != nil {
						return err
					}
				} else {
					updated := renderMigratedContent(string(content), nil, initSnippet(shell))
					if updated != string(content) {
						if err := writeBackupAndReplace(rcPath, content, []byte(updated)); err != nil {
							return err
						}
					}
				}
				fmt.Fprintln(cmd.OutOrStdout(), "No plaintext secrets found. Shell init installed.")
				return nil
			}

			for _, secret := range secrets {
				fmt.Fprintf(cmd.OutOrStdout(), "Found %s in %s:%d\n", secret.Key, rcPath, secret.Line)
			}

			if !yes {
				confirmed, err := promptForConfirmation(cmd)
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}

			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}

			entries := make(map[string]string, len(secrets))
			for _, secret := range secrets {
				entries[secret.Key] = secret.Value
			}
			if _, err := app.Bulk.BulkSet(entries, vault); err != nil {
				return fmt.Errorf("setup: import to vault %q: %w", vault, err)
			}

			updated := renderMigratedContent(string(content), secrets, initSnippet(shell))
			if err := writeBackupAndReplace(rcPath, content, []byte(updated)); err != nil {
				return err
			}

			if err := ensureShellInit(shell); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✅ %d secrets migrated. Restart your shell.\n", len(secrets))
			return nil
		},
	}
	cmd.Flags().BoolP("yes", "y", false, "apply migration without interactive confirmation")
	return cmd
}

func promptForConfirmation(cmd *cobra.Command) (bool, error) {
	reader := bufio.NewReader(cmd.InOrStdin())
	fmt.Fprint(cmd.OutOrStdout(), "Continue? [y/N]: ")
	line, err := reader.ReadString('\n')
	if err != nil && err.Error() != "EOF" {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func detectCurrentShell() (string, error) {
	return normalizeShell(filepath.Base(os.Getenv("SHELL")))
}

func normalizeShell(shell string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case shellZsh:
		return shellZsh, nil
	case shellBash:
		return shellBash, nil
	case shellFish:
		return shellFish, nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

func initSnippet(shell string) string {
	if shell == shellFish {
		return "kc env | source"
	}
	return "eval \"$(kc env)\""
}

func primaryRCPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("setup: detect home directory: %w", err)
	}

	switch shell {
	case shellZsh:
		return filepath.Join(home, ".zshrc"), nil
	case shellBash:
		return filepath.Join(home, ".bash_profile"), nil
	case shellFish:
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "fish", "config.fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

func detectSecretsFromContent(content, shell string) []detectedSecret {
	lines := strings.Split(content, "\n")
	secrets := make([]detectedSecret, 0)
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, migratedPrefix) {
			continue
		}

		var key, value string
		var ok bool
		if shell == shellFish {
			key, value, ok = parseFishSecret(trimmed)
		} else {
			key, value, ok = parseExportSecret(trimmed)
		}
		if !ok {
			continue
		}
		secrets = append(secrets, detectedSecret{Line: idx + 1, Key: key, Value: value, RawLine: line})
	}
	return secrets
}

func parseExportSecret(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "export ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	}
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if !looksLikeSecretKey(key) || !isLiteralSecretValue(value) {
		return "", "", false
	}
	return key, unquoteValue(value), true
}

func parseFishSecret(line string) (string, string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || fields[0] != "set" {
		return "", "", false
	}
	var key string
	var valueParts []string
	for i := 1; i < len(fields); i++ {
		field := fields[i]
		if strings.HasPrefix(field, "-") {
			continue
		}
		key = field
		valueParts = fields[i+1:]
		break
	}
	if key == "" || len(valueParts) == 0 {
		return "", "", false
	}
	value := strings.TrimSpace(strings.Join(valueParts, " "))
	if !looksLikeSecretKey(key) || !isLiteralSecretValue(value) {
		return "", "", false
	}
	return key, unquoteValue(value), true
}

func looksLikeSecretKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if upper == "" {
		return false
	}
	for _, marker := range []string{"KEY", "SECRET", "TOKEN", "PASSWORD", "CREDENTIAL"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func isLiteralSecretValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	for _, marker := range []string{"$", "$(", "${", "`"} {
		if strings.Contains(trimmed, marker) {
			return false
		}
	}
	if strings.EqualFold(trimmed, "true") || strings.EqualFold(trimmed, "false") {
		return false
	}
	return true
}

func renderMigratedContent(original string, secrets []detectedSecret, snippet string) string {
	lines := strings.Split(original, "\n")
	byLine := make(map[int]detectedSecret, len(secrets))
	for _, secret := range secrets {
		byLine[secret.Line] = secret
	}
	for i, line := range lines {
		secret, ok := byLine[i+1]
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, migratedPrefix) {
			continue
		}
		if strings.TrimSpace(secret.RawLine) == trimmed {
			lines[i] = migratedPrefix + line
		}
	}
	updated := strings.Join(lines, "\n")
	if strings.Contains(updated, kcBeginMarker) || strings.Contains(updated, snippet) {
		return updated
	}
	if updated != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	if strings.TrimSpace(updated) != "" {
		updated += "\n"
	}
	updated += kcBeginMarker + "\n" + snippet + "\n" + kcEndMarker + "\n"
	return updated
}

func writeBackupAndReplace(path string, original, updated []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("setup: create directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path+".bak", original, 0o600); err != nil {
		return fmt.Errorf("setup: write backup for %q: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".kc-setup-*")
	if err != nil {
		return fmt.Errorf("setup: create temp file for %q: %w", path, err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(updated); err != nil {
		tmp.Close()
		return fmt.Errorf("setup: write temp file for %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("setup: close temp file for %q: %w", path, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("setup: replace %q: %w", path, err)
	}
	return nil
}

func ensureShellInit(shell string) error {
	if shell != shellFish {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("setup: detect home directory: %w", err)
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	confD := filepath.Join(configHome, "fish", "conf.d")
	if err := os.MkdirAll(confD, 0o755); err != nil {
		return fmt.Errorf("setup: create fish conf.d: %w", err)
	}
	confPath := filepath.Join(confD, "kc.fish")
	data := []byte(initSnippet(shellFish) + "\n")
	if existing, err := os.ReadFile(confPath); err == nil && string(existing) == string(data) {
		return nil
	}
	if err := os.WriteFile(confPath, data, 0o644); err != nil {
		return fmt.Errorf("setup: write fish init file: %w", err)
	}
	return nil
}
