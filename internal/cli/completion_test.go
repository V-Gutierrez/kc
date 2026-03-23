package cli_test

import (
	"strings"
	"testing"
)

func TestCompletionBash(t *testing.T) {
	app, _, _, _ := newTestApp()
	stdout, stderr, err := executeCmd(app, "completion", "bash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "bash") || !strings.Contains(stdout, "complet") {
		t.Fatal("expected bash completion script on stdout")
	}
	if len(stdout) < 100 {
		t.Fatalf("completion script too short: %d bytes", len(stdout))
	}
}

func TestCompletionZsh(t *testing.T) {
	app, _, _, _ := newTestApp()
	stdout, stderr, err := executeCmd(app, "completion", "zsh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if len(stdout) < 100 {
		t.Fatalf("completion script too short: %d bytes", len(stdout))
	}
}

func TestCompletionFish(t *testing.T) {
	app, _, _, _ := newTestApp()
	stdout, stderr, err := executeCmd(app, "completion", "fish")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "fish") || !strings.Contains(stdout, "complet") {
		t.Fatal("expected fish completion script on stdout")
	}
}

func TestCompletionPowershell(t *testing.T) {
	app, _, _, _ := newTestApp()
	stdout, stderr, err := executeCmd(app, "completion", "powershell")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if len(stdout) < 100 {
		t.Fatalf("completion script too short: %d bytes", len(stdout))
	}
}

func TestCompletionUnsupportedShell(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "completion", "tcsh")
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("error = %q, want 'unsupported shell'", err.Error())
	}
}

func TestCompletionNoArgs(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "completion")
	if err == nil {
		t.Fatal("expected error when no shell argument given")
	}
}

func TestCompletionTooManyArgs(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "completion", "bash", "extra")
	if err == nil {
		t.Fatal("expected error with too many args")
	}
}

func TestCompletionInHelp(t *testing.T) {
	app, _, _, _ := newTestApp()
	stdout, _, err := executeCmd(app, "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "completion") {
		t.Fatal("completion subcommand missing from help output")
	}
}
