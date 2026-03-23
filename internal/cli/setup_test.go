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
	if !strings.Contains(updated, "# BEGIN kc") || !strings.Contains(updated, "eval \"$(kc env)\"") {
		t.Fatalf("updated = %q, want kc init block", updated)
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

	if got := initSnippet(shellZsh); got != "eval \"$(kc env)\"" {
		t.Fatalf("zsh snippet = %q", got)
	}
	if got := initSnippet(shellBash); got != "eval \"$(kc env)\"" {
		t.Fatalf("bash snippet = %q", got)
	}
	if got := initSnippet(shellFish); got != "kc env | source" {
		t.Fatalf("fish snippet = %q", got)
	}
}
