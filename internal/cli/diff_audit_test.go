package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffEnvVsVault(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "A", "same"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "B", "old"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "D", "vault-only"); err != nil {
		t.Fatal(err)
	}

	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("A=same\nB=new\nC=env-only\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "diff", envFile, "--vault", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"= A", "~ B ******** -> ********", "+ C", "- D"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "new") || strings.Contains(stdout, "old") {
		t.Fatalf("stdout leaked raw values: %q", stdout)
	}
}

func TestDiffVaultToVault(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	if err := vaults.Create("dev"); err != nil {
		t.Fatal(err)
	}
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("dev", "KEY", "one"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "KEY", "two"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "diff", "--vault", "dev", "--vault", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "~ KEY ******** -> ********") {
		t.Fatalf("stdout = %q, want masked changed entry", stdout)
	}
}

func TestAuditFindings(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "API_KEY", "shared-secret-value!"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "TEMP", "password"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "DUPLICATE", "shared-secret-value!"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "old_token", "abc123"); err != nil {
		t.Fatal(err)
	}

	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("API_KEY=shared-secret-value!\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "audit", "--env-file", envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"SEVERITY", "HIGH\tdefault\tAPI_KEY\tduplicate", "MEDIUM\tdefault\tTEMP\tweak-secret", "LOW\tprod\told_token\tsuspicious-name", "LOW\tprod\tDUPLICATE\tstale"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want substring %q", stdout, want)
		}
	}
}

func TestAuditCleanVault(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.Set("default", "API_KEY", "long-secret-value!@#"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("API_KEY", "present")

	stdout, _, err := executeCmd(app, "audit", "--vault", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(stdout) != "No issues found." {
		t.Fatalf("stdout = %q, want no issues", stdout)
	}
}
