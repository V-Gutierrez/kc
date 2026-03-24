package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/v-gutierrez/kc/internal/cli"
)

func TestSetDefaultsToProtected(t *testing.T) {
	app, _, _, _ := newTestApp()

	_, _, err := executeCmd(app, "set", "TOKEN", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	store := app.Store.(*mockStore)
	if !store.protected["default"]["TOKEN"] {
		t.Fatal("TOKEN should be protected by default")
	}
}

func TestSetNoProtectStoresUnprotected(t *testing.T) {
	app, _, _, _ := newTestApp()

	_, _, err := executeCmd(app, "set", "--no-protect", "TOKEN", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	store := app.Store.(*mockStore)
	if store.protected["default"]["TOKEN"] {
		t.Fatal("TOKEN should be unprotected with --no-protect")
	}
}

func TestListProtectedShowsProtectionStatus(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.SetWithProtection("default", "API_KEY", "secret", true)
	store.SetWithProtection("default", "PUBLIC_KEY", "value", false)

	stdout, _, err := executeCmd(app, "list", "--protected")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "API_KEY") || !strings.Contains(stdout, cli.ProtectionProtected) {
		t.Fatalf("stdout = %q, want protected API_KEY", stdout)
	}
	if strings.Contains(stdout, "PUBLIC_KEY") {
		t.Fatalf("stdout = %q, should exclude unprotected entries with --protected", stdout)
	}
}

func TestSearchAcrossVaultsShowsProtection(t *testing.T) {
	app, store, _, vaults := newTestAppWithBulk()
	vaults.Create("prod")
	store.SetWithProtection("default", "MONGO_URL", "mongodb://default", true)
	store.SetWithProtection("prod", "MONGO_PASSWORD", "secret", false)

	stdout, _, err := executeCmd(app, "search", "mongo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "MONGO_URL") || !strings.Contains(stdout, "default") || !strings.Contains(stdout, cli.ProtectionProtected) {
		t.Fatalf("stdout = %q, want protected default result", stdout)
	}
	if !strings.Contains(stdout, "MONGO_PASSWORD") || !strings.Contains(stdout, "prod") || !strings.Contains(stdout, cli.ProtectionUnprotected) {
		t.Fatalf("stdout = %q, want unprotected prod result", stdout)
	}
}

func TestSearchJSONIncludesProtectionAndOmitValuesByDefault(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()
	store.SetWithProtection("default", "MONGO_URL", "mongodb://default", true)

	stdout, _, err := executeCmd(app, "search", "mongo", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v\nstdout=%q", err, stdout)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded length = %d, want 1", len(decoded))
	}
	if decoded[0]["protection"] != cli.ProtectionProtected {
		t.Fatalf("protection = %#v, want %q", decoded[0]["protection"], cli.ProtectionProtected)
	}
	if _, ok := decoded[0]["value"]; ok {
		t.Fatalf("value should be omitted by default: %#v", decoded[0])
	}
}

func TestProtectAllMarksExistingEntriesProtected(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.SetWithProtection("default", "LEGACY_KEY", "legacy", false)

	stdout, _, err := executeCmd(app, "protect", "--all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Protected 1") {
		t.Fatalf("stdout = %q, want protected count", stdout)
	}
	if !store.protected["default"]["LEGACY_KEY"] {
		t.Fatal("LEGACY_KEY should be protected after kc protect --all")
	}
}
