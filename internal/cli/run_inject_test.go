package cli_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/v-gutierrez/kc/internal/cli"
)

type runCapture struct {
	name string
	args []string
	env  []string
}

func fakeRunner(exitCode int, err error, capture *runCapture) cli.CommandRunner {
	return func(name string, args []string, env []string) (int, error) {
		if capture != nil {
			capture.name = name
			capture.args = args
			capture.env = make([]string, len(env))
			copy(capture.env, env)
		}
		return exitCode, err
	}
}

func newTestAppWithRunner(runner cli.CommandRunner) (*cli.App, *mockStore, *mockVaultManager) {
	store := newMockStore()
	bulk := &mockBulkStore{mockStore: store, rawServices: make(map[string]map[string]string)}
	vaults := &mockVaultManager{vaults: []string{"default"}, active: "default", store: store}
	clip := &mockClipboard{}
	app := &cli.App{
		Store:     store,
		Bulk:      bulk,
		Vaults:    vaults,
		Clipboard: clip,
		Runner:    runner,
	}
	return app, store, vaults
}

func TestRun_RequiresDoubleDash(t *testing.T) {
	capture := &runCapture{}
	app, _, _ := newTestAppWithRunner(fakeRunner(0, nil, capture))
	_, _, err := executeCmd(app, "run", "echo", "hello")
	if err == nil {
		t.Fatal("expected error when -- is missing")
	}
	if !strings.Contains(err.Error(), "--") {
		t.Errorf("error = %q, want mention of --", err.Error())
	}
}

func TestRun_InjectsSecretsIntoChildEnv(t *testing.T) {
	capture := &runCapture{}
	app, store, _ := newTestAppWithRunner(fakeRunner(0, nil, capture))
	if err := store.Set("default", "MY_SECRET", "s3cret"); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "run", "--", "echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capture.name != "echo" {
		t.Errorf("command = %q, want echo", capture.name)
	}
	if len(capture.args) != 1 || capture.args[0] != "hello" {
		t.Errorf("args = %v, want [hello]", capture.args)
	}

	found := false
	for _, e := range capture.env {
		if e == "MY_SECRET=s3cret" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("env missing MY_SECRET=s3cret, got %v", capture.env)
	}
}

func TestRun_SecretsOverrideParentEnv(t *testing.T) {
	capture := &runCapture{}
	app, store, _ := newTestAppWithRunner(fakeRunner(0, nil, capture))
	if err := store.Set("default", "HOME", "/secret/home"); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "run", "--", "echo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	homeCount := 0
	for _, e := range capture.env {
		if strings.HasPrefix(e, "HOME=") {
			homeCount++
			if e != "HOME=/secret/home" {
				t.Errorf("HOME = %q, want HOME=/secret/home", e)
			}
		}
	}
	if homeCount != 1 {
		t.Errorf("expected exactly 1 HOME entry, got %d", homeCount)
	}
}

func TestRun_PreservesParentEnv(t *testing.T) {
	capture := &runCapture{}
	app, store, _ := newTestAppWithRunner(fakeRunner(0, nil, capture))
	if err := store.Set("default", "MY_ONLY", "val"); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "run", "--", "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capture.env) <= 1 {
		t.Fatal("expected parent env vars to be preserved, got only vault secrets")
	}
}

func TestRun_PropagatesNonZeroExitCode(t *testing.T) {
	app, _, _ := newTestAppWithRunner(fakeRunner(42, nil, nil))
	_, _, err := executeCmd(app, "run", "--", "false")
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	var exitErr *cli.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *cli.ExitError", err)
	}
	if exitErr.Code != 42 {
		t.Errorf("exit code = %d, want 42", exitErr.Code)
	}
}

func TestRun_ZeroExitNoError(t *testing.T) {
	app, _, _ := newTestAppWithRunner(fakeRunner(0, nil, nil))
	_, _, err := executeCmd(app, "run", "--", "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_RunnerSystemError(t *testing.T) {
	app, _, _ := newTestAppWithRunner(fakeRunner(0, fmt.Errorf("exec: not found"), nil))
	_, _, err := executeCmd(app, "run", "--", "nonexistent")
	if err == nil {
		t.Fatal("expected error from runner")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want exec error", err.Error())
	}
}

func TestRun_WithVaultFlag(t *testing.T) {
	capture := &runCapture{}
	app, store, vaults := newTestAppWithRunner(fakeRunner(0, nil, capture))
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "PROD_KEY", "prodval"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "DEF_KEY", "defval"); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "run", "--vault", "prod", "--", "echo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundProd := false
	foundDef := false
	for _, e := range capture.env {
		if e == "PROD_KEY=prodval" {
			foundProd = true
		}
		if e == "DEF_KEY=defval" {
			foundDef = true
		}
	}
	if !foundProd {
		t.Error("env missing PROD_KEY=prodval")
	}
	if foundDef {
		t.Error("env should not contain DEF_KEY from default vault")
	}
}

func TestRun_ProtectedVaultAuthorizesOnce(t *testing.T) {
	capture := &runCapture{}
	store := newMockStore()
	bulk := &mockBulkStore{mockStore: store, rawServices: make(map[string]map[string]string)}
	vaults := &mockVaultManager{vaults: []string{"default"}, active: "default", store: store}
	authorizer := &countingAuthorizer{}
	app := &cli.App{
		Store:  store,
		Bulk:   bulk,
		Vaults: vaults,
		Auth:   authorizer,
		Runner: fakeRunner(0, nil, capture),
	}
	if err := store.SetWithProtection("default", "SECRET", "val", true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWithProtection("default", "PUBLIC", "pub", false); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "run", "--", "echo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorizer.calls != 1 {
		t.Errorf("authorizer calls = %d, want 1", authorizer.calls)
	}
}

func TestRun_NilBulkReturnsError(t *testing.T) {
	store := newMockStore()
	vaults := &mockVaultManager{vaults: []string{"default"}, active: "default", store: store}
	app := &cli.App{
		Store:  store,
		Vaults: vaults,
		Bulk:   nil,
		Runner: fakeRunner(0, nil, nil),
	}

	_, _, err := executeCmd(app, "run", "--", "echo")
	if err == nil {
		t.Fatal("expected error when Bulk is nil")
	}
}

func TestRun_NilRunnerReturnsError(t *testing.T) {
	app, _, _ := newTestAppWithRunner(nil)
	app.Runner = nil
	_, _, err := executeCmd(app, "run", "--", "echo")
	if err == nil {
		t.Fatal("expected error when Runner is nil")
	}
}

func TestRun_NoCommandAfterDash(t *testing.T) {
	app, _, _ := newTestAppWithRunner(fakeRunner(0, nil, nil))
	_, _, err := executeCmd(app, "run", "--")
	if err == nil {
		t.Fatal("expected error when no command after --")
	}
}

func TestInject_PrintsSingleSecretRaw(t *testing.T) {
	app, store, _ := newTestAppWithRunner(nil)
	if err := store.Set("default", "API_KEY", "secret123"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "inject", "--key", "API_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "secret123" {
		t.Errorf("stdout = %q, want %q (no trailing newline)", stdout, "secret123")
	}
}

func TestInject_RequiresKeyFlag(t *testing.T) {
	app, _, _ := newTestAppWithRunner(nil)
	_, _, err := executeCmd(app, "inject")
	if err == nil {
		t.Fatal("expected error when --key is missing")
	}
}

func TestInject_KeyNotFound(t *testing.T) {
	app, _, _ := newTestAppWithRunner(nil)
	_, _, err := executeCmd(app, "inject", "--key", "NOPE")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !strings.Contains(err.Error(), "NOPE") {
		t.Errorf("error = %q, want mention of NOPE", err.Error())
	}
}

func TestInject_WithVaultFlag(t *testing.T) {
	app, store, vaults := newTestAppWithRunner(nil)
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "PROD_SECRET", "prodval"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "inject", "--key", "PROD_SECRET", "--vault", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "prodval" {
		t.Errorf("stdout = %q, want %q", stdout, "prodval")
	}
}

func TestInject_ProtectedKeyAuthorizes(t *testing.T) {
	store := newMockStore()
	bulk := &mockBulkStore{mockStore: store, rawServices: make(map[string]map[string]string)}
	vaults := &mockVaultManager{vaults: []string{"default"}, active: "default", store: store}
	authorizer := &countingAuthorizer{}
	app := &cli.App{
		Store:  store,
		Bulk:   bulk,
		Vaults: vaults,
		Auth:   authorizer,
	}
	if err := store.SetWithProtection("default", "SECRET", "val", true); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "inject", "--key", "SECRET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorizer.calls != 1 {
		t.Errorf("authorizer calls = %d, want 1", authorizer.calls)
	}
	if stdout != "val" {
		t.Errorf("stdout = %q, want %q", stdout, "val")
	}
}
