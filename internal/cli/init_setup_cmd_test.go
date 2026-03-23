package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/v-gutierrez/kc/internal/cli"
)

func executeCmdWithInput(app *cli.App, input string, args ...string) (stdout, stderr string, err error) {
	root := cli.NewRootCmd(app)
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetIn(strings.NewReader(input))
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestInitOutputsShellSnippet(t *testing.T) {
	app, _, _, _ := newTestAppWithBulk()

	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{name: "zsh", shell: "zsh", want: "eval \"$(kc env)\"\n"},
		{name: "bash", shell: "bash", want: "eval \"$(kc env)\"\n"},
		{name: "fish", shell: "fish", want: "kc env | source\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeCmd(app, "init", tt.shell)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			if stdout != tt.want {
				t.Fatalf("stdout = %q, want %q", stdout, tt.want)
			}
		})
	}
}

func TestInitRejectsUnsupportedShell(t *testing.T) {
	app, _, _, _ := newTestAppWithBulk()
	_, _, err := executeCmd(app, "init", "tcsh")
	if err == nil {
		t.Fatal("expected unsupported shell error")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("error = %q, want unsupported shell", err.Error())
	}
}

func TestSetupMigratesZshSecretsAndInjectsSnippet(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")

	rcPath := filepath.Join(home, ".zshrc")
	rcContent := strings.Join([]string{
		"export API_KEY=sk-test-123",
		"export AWS_SECRET_ACCESS_KEY='top-secret'",
		"export PATH=/usr/local/bin:$PATH",
		"# existing comment",
	}, "\n") + "\n"
	if err := os.WriteFile(rcPath, []byte(rcContent), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeCmd(app, "setup", "--yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "API_KEY") || !strings.Contains(stdout, "AWS_SECRET_ACCESS_KEY") {
		t.Fatalf("stdout = %q, want detected secret names", stdout)
	}
	if !strings.Contains(stdout, "✅ 2 secrets migrated. Restart your shell.") {
		t.Fatalf("stdout = %q, want migration summary", stdout)
	}

	if got, _ := store.Get("default", "API_KEY"); got != "sk-test-123" {
		t.Fatalf("API_KEY = %q, want %q", got, "sk-test-123")
	}
	if got, _ := store.Get("default", "AWS_SECRET_ACCESS_KEY"); got != "top-secret" {
		t.Fatalf("AWS_SECRET_ACCESS_KEY = %q, want %q", got, "top-secret")
	}

	data, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := string(data)
	if !strings.Contains(updated, "#kc-migrated# export API_KEY=sk-test-123") {
		t.Fatalf("updated rc missing migrated API_KEY line: %q", updated)
	}
	if !strings.Contains(updated, "#kc-migrated# export AWS_SECRET_ACCESS_KEY='top-secret'") {
		t.Fatalf("updated rc missing migrated AWS_SECRET_ACCESS_KEY line: %q", updated)
	}
	if !strings.Contains(updated, "eval \"$(kc env)\"") {
		t.Fatalf("updated rc missing init snippet: %q", updated)
	}

	backup, err := os.ReadFile(rcPath + ".bak")
	if err != nil {
		t.Fatalf("expected backup file: %v", err)
	}
	if string(backup) != rcContent {
		t.Fatalf("backup = %q, want original content %q", string(backup), rcContent)
	}
}

func TestSetupWithoutYesCancelsWhenUserDeclines(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")

	rcPath := filepath.Join(home, ".bash_profile")
	original := "export GITHUB_TOKEN=ghp_decline_me\n"
	if err := os.WriteFile(rcPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmdWithInput(app, "n\n", "setup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Aborted.") {
		t.Fatalf("stdout = %q, want abort confirmation", stdout)
	}
	if _, err := store.Get("default", "GITHUB_TOKEN"); err == nil {
		t.Fatal("secret should not be imported when user declines")
	}

	data, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("rc file changed on decline: %q", string(data))
	}
}

func TestSetupFishWritesConfDFile(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()
	home := t.TempDir()
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("SHELL", "/opt/homebrew/bin/fish")

	configPath := filepath.Join(xdg, "fish", "config.fish")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("set -gx OPENAI_API_KEY sk-fish-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "setup", "--yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "✅ 1 secrets migrated. Restart your shell.") {
		t.Fatalf("stdout = %q, want fish migration summary", stdout)
	}
	if got, _ := store.Get("default", "OPENAI_API_KEY"); got != "sk-fish-123" {
		t.Fatalf("OPENAI_API_KEY = %q, want %q", got, "sk-fish-123")
	}

	confDPath := filepath.Join(xdg, "fish", "conf.d", "kc.fish")
	data, err := os.ReadFile(confDPath)
	if err != nil {
		t.Fatalf("expected fish conf.d file: %v", err)
	}
	if string(data) != "kc env | source\n" {
		t.Fatalf("conf.d file = %q, want fish source snippet", string(data))
	}
}

func TestSetupWithoutSecretsStillInstallsInitSnippet(t *testing.T) {
	app, _, _, _ := newTestAppWithBulk()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")

	rcPath := filepath.Join(home, ".zshrc")
	original := "export PATH=/usr/local/bin:$PATH\n"
	if err := os.WriteFile(rcPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "setup", "--yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "No plaintext secrets found. Shell init installed.") {
		t.Fatalf("stdout = %q, want no-secrets summary", stdout)
	}

	data, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := string(data)
	if !strings.Contains(updated, original) {
		t.Fatalf("updated rc missing original content: %q", updated)
	}
	if !strings.Contains(updated, "eval \"$(kc env)\"") {
		t.Fatalf("updated rc missing init snippet: %q", updated)
	}
}
