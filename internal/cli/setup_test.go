package cli

import (
	"strings"
	"testing"
)

func TestDetectSecretsFromContent_ZshStyle(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"export API_KEY=sk-test-123",
		"export PATH=/usr/local/bin:$PATH",
		"#kc-migrated# export OLD_TOKEN=should-skip",
		"export TOKEN=$FROM_ENV",
		"export APP_SECRET='quoted-secret'",
	}, "\n")

	found := detectSecretsFromContent(content, shellZsh)
	if len(found) != 2 {
		t.Fatalf("detected %d secrets, want 2", len(found))
	}
	if found[0].Key != "API_KEY" || found[0].Value != "sk-test-123" {
		t.Fatalf("first secret = %+v, want API_KEY/sk-test-123", found[0])
	}
	if found[1].Key != "APP_SECRET" || found[1].Value != "quoted-secret" {
		t.Fatalf("second secret = %+v, want APP_SECRET/quoted-secret", found[1])
	}
}

func TestDetectSecretsFromContent_FishStyle(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"set -gx OPENAI_API_KEY sk-123",
		"set -gx PATH /usr/local/bin $PATH",
		"set -Ux APP_TOKEN \"token-value\"",
	}, "\n")

	found := detectSecretsFromContent(content, shellFish)
	if len(found) != 2 {
		t.Fatalf("detected %d secrets, want 2", len(found))
	}
	if found[0].Key != "OPENAI_API_KEY" || found[0].Value != "sk-123" {
		t.Fatalf("first secret = %+v, want OPENAI_API_KEY/sk-123", found[0])
	}
	if found[1].Key != "APP_TOKEN" || found[1].Value != "token-value" {
		t.Fatalf("second secret = %+v, want APP_TOKEN/token-value", found[1])
	}
}

func TestRenderMigratedContentCommentsLinesAndAppendsSnippetOnce(t *testing.T) {
	t.Parallel()

	original := strings.Join([]string{
		"export API_KEY=sk-test-123",
		"export PATH=/usr/local/bin:$PATH",
	}, "\n") + "\n"

	secrets := []detectedSecret{{Line: 1, Key: "API_KEY", Value: "sk-test-123", RawLine: "export API_KEY=sk-test-123"}}
	updated := renderMigratedContent(original, secrets, initSnippet(shellZsh))
	if !strings.Contains(updated, "#kc-migrated# export API_KEY=sk-test-123") {
		t.Fatalf("updated = %q, want commented secret line", updated)
	}
	if !strings.Contains(updated, "# BEGIN kc") || !strings.Contains(updated, "eval \"$(command kc env)\"") || !strings.Contains(updated, "command kc completion zsh") {
		t.Fatalf("updated = %q, want kc init block with env and completion", updated)
	}

	again := renderMigratedContent(updated, secrets, initSnippet(shellZsh))
	if strings.Count(again, "# BEGIN kc") != 1 {
		t.Fatalf("snippet injected more than once: %q", again)
	}
	if strings.Count(again, "#kc-migrated# export API_KEY=sk-test-123") != 1 {
		t.Fatalf("migrated line duplicated: %q", again)
	}
}

func TestInitSnippetForSupportedShells(t *testing.T) {
	t.Parallel()

	if got := initSnippet(shellZsh); got != "kc() {\n  if [ \"$1\" = \"load\" ]; then\n    if [ \"$#\" -gt 2 ]; then\n      printf 'kc load accepts at most one vault name\\n' >&2\n      return 1\n    fi\n    if [ -n \"$2\" ]; then\n      kc_env_output=\"$(command kc env --vault \"$2\")\" || return $?\n    else\n      kc_env_output=\"$(command kc env)\" || return $?\n    fi\n    eval \"$kc_env_output\"\n  else\n    command kc \"$@\"\n  fi\n}\neval \"$(command kc env)\"\nsource <(command kc completion zsh)" {
		t.Fatalf("zsh snippet = %q", got)
	}
	if got := initSnippet(shellBash); got != "kc() {\n  if [ \"$1\" = \"load\" ]; then\n    if [ \"$#\" -gt 2 ]; then\n      printf 'kc load accepts at most one vault name\\n' >&2\n      return 1\n    fi\n    if [ -n \"$2\" ]; then\n      kc_env_output=\"$(command kc env --vault \"$2\")\" || return $?\n    else\n      kc_env_output=\"$(command kc env)\" || return $?\n    fi\n    eval \"$kc_env_output\"\n  else\n    command kc \"$@\"\n  fi\n}\neval \"$(command kc env)\"\nsource <(command kc completion bash)" {
		t.Fatalf("bash snippet = %q", got)
	}
	if got := initSnippet(shellFish); got != "function kc\n  if test \"$argv[1]\" = \"load\"\n    if test (count $argv) -gt 2\n      printf 'kc load accepts at most one vault name\\n' >&2\n      return 1\n    end\n    set -l kc_env_output\n    if test (count $argv) -ge 2\n      set kc_env_output (command kc env --vault \"$argv[2]\")\n      or return $status\n    else\n      set kc_env_output (command kc env)\n      or return $status\n    end\n    printf '%s\\n' $kc_env_output | source\n  else\n    command kc $argv\n  end\nend\ncommand kc env | source\ncommand kc completion fish | source" {
		t.Fatalf("fish snippet = %q", got)
	}
}

func TestRenderMigratedContentReplacesExistingKCBlock(t *testing.T) {
	t.Parallel()

	original := strings.Join([]string{
		"export PATH=/usr/local/bin:$PATH",
		kcBeginMarker,
		"eval \"$(kc env)\"",
		kcEndMarker,
		"",
	}, "\n")

	updated := renderMigratedContent(original, nil, initSnippet(shellZsh))

	if strings.Count(updated, kcBeginMarker) != 1 {
		t.Fatalf("kc block count = %d, want 1 in %q", strings.Count(updated, kcBeginMarker), updated)
	}
	if !strings.Contains(updated, "source <(command kc completion zsh)") {
		t.Fatalf("updated content missing zsh completion source: %q", updated)
	}
	if strings.Contains(updated, kcBeginMarker+"\n"+"eval \"$(command kc env)\"\n"+kcEndMarker) {
		t.Fatalf("old kc block should be replaced, got %q", updated)
	}
}
