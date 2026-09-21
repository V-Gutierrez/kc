package cli_test

import (
	"errors"
	"strings"
	"testing"
)

// `kc env --sync` is what a shell cd-hook calls on every prompt. When the
// resolved vault is already the loaded one it must print nothing at all —
// otherwise every directory change re-reads the Keychain and, for protected
// secrets, asks for Touch ID.
func TestEnvSyncIsSilentWhenVaultUnchanged(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.SetWithProtection("default", "API_KEY", "v1", false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_LOADED_VAULT", "default")
	t.Setenv("KC_LOADED_KEYS", "API_KEY")

	stdout, _, err := executeCmd(app, "env", "--sync")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty when the vault did not change", stdout)
	}
}

// When the vault changes, the keys the previous vault exported have to be
// unset. Leaving them behind silently mixes two vaults in one shell — the
// exact failure a per-directory vault is meant to prevent.
func TestEnvSyncUnsetsStaleKeysAndExportsTheNewVault(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	vaults.vaults = append(vaults.vaults, "staging")
	if err := store.SetWithProtection("staging", "SHARED", "new", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWithProtection("staging", "ONLY_STAGING", "s", false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_LOADED_VAULT", "prod")
	t.Setenv("KC_LOADED_KEYS", "SHARED ONLY_PROD")

	stdout, _, err := executeCmd(app, "env", "--sync", "--vault", "staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "unset ONLY_PROD") {
		t.Fatalf("stale key from the previous vault was not unset:\n%s", stdout)
	}
	if strings.Contains(stdout, "unset SHARED") {
		t.Fatalf("SHARED exists in the new vault and must not be unset:\n%s", stdout)
	}
	if !strings.Contains(stdout, "export SHARED=") || !strings.Contains(stdout, "export ONLY_STAGING=") {
		t.Fatalf("new vault was not exported:\n%s", stdout)
	}
	if !strings.Contains(stdout, "export KC_LOADED_VAULT=staging") {
		t.Fatalf("bookkeeping variable KC_LOADED_VAULT missing:\n%s", stdout)
	}
	if !strings.Contains(stdout, "KC_LOADED_KEYS=") {
		t.Fatalf("bookkeeping variable KC_LOADED_KEYS missing:\n%s", stdout)
	}
}

// The unset must come before the exports, or a key present in both vaults gets
// wiped right after being written.
func TestEnvSyncUnsetsBeforeExporting(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	vaults.vaults = append(vaults.vaults, "staging")
	if err := store.SetWithProtection("staging", "SHARED", "new", false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_LOADED_VAULT", "prod")
	t.Setenv("KC_LOADED_KEYS", "OLD_ONE")

	stdout, _, err := executeCmd(app, "env", "--sync", "--vault", "staging")
	if err != nil {
		t.Fatal(err)
	}
	unsetAt := strings.Index(stdout, "unset ")
	exportAt := strings.Index(stdout, "export SHARED=")
	if unsetAt < 0 || exportAt < 0 || unsetAt > exportAt {
		t.Fatalf("unset must precede export:\n%s", stdout)
	}
}

// A first load has nothing to unset, but still records what it exported.
func TestEnvSyncFirstLoadRecordsBookkeeping(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.SetWithProtection("default", "API_KEY", "v1", false); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "env", "--sync")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, "unset") {
		t.Fatalf("nothing was loaded before, so nothing can be unset:\n%s", stdout)
	}
	if !strings.Contains(stdout, "export API_KEY=") || !strings.Contains(stdout, "export KC_LOADED_VAULT=default") {
		t.Fatalf("first load did not export the vault and its marker:\n%s", stdout)
	}
}

// Plain `kc env` keeps its old contract: exports only, no bookkeeping noise.
func TestEnvWithoutSyncStaysUnchanged(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.SetWithProtection("default", "API_KEY", "v1", false); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "env")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, "KC_LOADED_VAULT") || strings.Contains(stdout, "unset") {
		t.Fatalf("plain env must not emit sync bookkeeping:\n%s", stdout)
	}
	if !strings.Contains(stdout, "export API_KEY=") {
		t.Fatalf("plain env stopped exporting secrets:\n%s", stdout)
	}
}

// `kc hook SHELL` prints the cd-hook that drives --sync. It is opt-in: kc init
// does not install it, because it changes what `cd` does.
func TestHookEmitsShellSpecificSnippet(t *testing.T) {
	app, _, _, _ := newTestApp()

	cases := map[string]string{
		"zsh":  "chpwd",
		"bash": "PROMPT_COMMAND",
		"fish": "--on-variable PWD",
	}
	for shell, marker := range cases {
		stdout, _, err := executeCmd(app, "hook", shell)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", shell, err)
		}
		if !strings.Contains(stdout, marker) {
			t.Fatalf("%s hook missing %q:\n%s", shell, marker, stdout)
		}
		if !strings.Contains(stdout, "env --sync") {
			t.Fatalf("%s hook does not call env --sync:\n%s", shell, stdout)
		}
	}
}

func TestHookRejectsUnknownShell(t *testing.T) {
	app, _, _, _ := newTestApp()
	if _, _, err := executeCmd(app, "hook", "nushell"); err == nil {
		t.Fatal("expected an error for an unsupported shell")
	}
}

// Declining the Touch ID prompt on a directory change must still clear the
// previous vault's exports. Leaving them in place is the worse outcome of the
// two: the shell would keep secrets the new directory is not entitled to,
// exactly the mix the per-directory vault exists to prevent.
func TestEnvSyncClearsStaleKeysWhenUnlockIsDeclined(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	app.Auth = &countingAuthorizer{err: errors.New("authentication failed: app cancel")}
	vaults.vaults = append(vaults.vaults, "staging")
	if err := store.SetWithProtection("staging", "LOCKED", "s", true); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_LOADED_VAULT", "prod")
	t.Setenv("KC_LOADED_KEYS", "OLD_PROD_KEY")

	stdout, _, err := executeCmd(app, "env", "--sync", "--vault", "staging")
	if err != nil {
		t.Fatalf("sync must not hard-fail the shell hook: %v", err)
	}
	if !strings.Contains(stdout, "unset OLD_PROD_KEY") {
		t.Fatalf("stale key survived a declined unlock:\n%s", stdout)
	}
	if strings.Contains(stdout, "export LOCKED=") {
		t.Fatalf("a declined unlock must not export secrets:\n%s", stdout)
	}
	if !strings.Contains(stdout, "export KC_LOADED_VAULT=staging") {
		t.Fatalf("the new vault must still be recorded, or the hook retries on every prompt:\n%s", stdout)
	}
	if !strings.Contains(stdout, "export KC_LOADED_KEYS=''") {
		t.Fatalf("nothing was loaded, so the key list must be empty:\n%s", stdout)
	}
}

// Plain `kc env` keeps failing loudly: it is a command the user typed.
func TestEnvWithoutSyncStillFailsOnDeclinedUnlock(t *testing.T) {
	app, store, _, _ := newTestApp()
	app.Auth = &countingAuthorizer{err: errors.New("authentication failed: app cancel")}
	if err := store.SetWithProtection("default", "LOCKED", "s", true); err != nil {
		t.Fatal(err)
	}

	if _, _, err := executeCmd(app, "env"); err == nil {
		t.Fatal("expected plain env to surface the authentication failure")
	}
}
