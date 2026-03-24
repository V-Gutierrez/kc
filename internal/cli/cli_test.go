package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/v-gutierrez/kc/internal/cli"
)

// --- Mock implementations ---

type mockStore struct {
	data      map[string]map[string]string // vault -> key -> value
	protected map[string]map[string]bool
}

func newMockStore() *mockStore {
	return &mockStore{data: make(map[string]map[string]string), protected: make(map[string]map[string]bool)}
}

func (m *mockStore) Get(vault, key string) (string, error) {
	v, ok := m.data[vault]
	if !ok {
		return "", fmt.Errorf("vault %q not found", vault)
	}
	val, ok := v[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in vault %q", key, vault)
	}
	return val, nil
}

func (m *mockStore) Set(vault, key, value string) error {
	return m.SetWithProtection(vault, key, value, true)
}

func (m *mockStore) SetWithProtection(vault, key, value string, protected bool) error {
	if m.data[vault] == nil {
		m.data[vault] = make(map[string]string)
	}
	if m.protected[vault] == nil {
		m.protected[vault] = make(map[string]bool)
	}
	m.data[vault][key] = value
	m.protected[vault][key] = protected
	return nil
}

func (m *mockStore) Delete(vault, key string) error {
	v, ok := m.data[vault]
	if !ok {
		return fmt.Errorf("vault %q not found", vault)
	}
	if _, ok := v[key]; !ok {
		return fmt.Errorf("key %q not found in vault %q", key, vault)
	}
	delete(v, key)
	if m.protected[vault] != nil {
		delete(m.protected[vault], key)
	}
	return nil
}

func (m *mockStore) List(vault string) ([]string, error) {
	v, ok := m.data[vault]
	if !ok {
		return nil, nil
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mockStore) ListMetadata(vault string) ([]cli.SecretMetadata, error) {
	v, ok := m.data[vault]
	if !ok {
		return nil, nil
	}
	items := make([]cli.SecretMetadata, 0, len(v))
	for key := range v {
		protection := cli.ProtectionUnprotected
		if m.protected[vault][key] {
			protection = cli.ProtectionProtected
		}
		items = append(items, cli.SecretMetadata{Key: key, Vault: vault, Protection: protection})
	}
	return items, nil
}

func (m *mockStore) ProtectAll(vault string) (int, error) {
	v, ok := m.data[vault]
	if !ok {
		return 0, nil
	}
	if m.protected[vault] == nil {
		m.protected[vault] = make(map[string]bool)
	}
	count := 0
	for key := range v {
		if m.protected[vault][key] {
			continue
		}
		m.protected[vault][key] = true
		count++
	}
	return count, nil
}

type mockVaultManager struct {
	vaults []string
	active string
}

func (m *mockVaultManager) List() ([]string, error) {
	return m.vaults, nil
}

func (m *mockVaultManager) Create(name string) error {
	for _, v := range m.vaults {
		if v == name {
			return fmt.Errorf("vault %q already exists", name)
		}
	}
	m.vaults = append(m.vaults, name)
	return nil
}

func (m *mockVaultManager) Active() (string, error) {
	return m.active, nil
}

func (m *mockVaultManager) Switch(name string) error {
	for _, v := range m.vaults {
		if v == name {
			m.active = name
			return nil
		}
	}
	return fmt.Errorf("vault %q does not exist", name)
}

type mockClipboard struct {
	last string
}

type countingAuthorizer struct {
	calls int
	err   error
}

func (a *countingAuthorizer) Authorize(reason string) error {
	a.calls++
	return a.err
}

func (*countingAuthorizer) bootSessionEnabled() bool { //nolint:unused // implements auth.bootCapable interface
	return false
}

func (m *mockClipboard) Copy(value string) error {
	m.last = value
	return nil
}

// --- Helpers ---

func newTestApp() (*cli.App, *mockStore, *mockVaultManager, *mockClipboard) {
	store := newMockStore()
	bulk := &mockBulkStore{mockStore: store, rawServices: make(map[string]map[string]string)}
	vaults := &mockVaultManager{vaults: []string{"default"}, active: "default"}
	clip := &mockClipboard{}
	app := &cli.App{Store: store, Bulk: bulk, Vaults: vaults, Clipboard: clip}
	return app, store, vaults, clip
}

func executeCmd(app *cli.App, args ...string) (stdout, stderr string, err error) {
	root := cli.NewRootCmd(app)
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

type mockBulkStore struct {
	*mockStore
	rawServices map[string]map[string]string
}

func (m *mockBulkStore) BulkSet(entries map[string]string, vault string) (int, error) {
	return m.BulkSetWithProtection(entries, vault, true)
}

func (m *mockBulkStore) BulkSetWithProtection(entries map[string]string, vault string, protected bool) (int, error) {
	n := 0
	for k, v := range entries {
		if err := m.mockStore.SetWithProtection(vault, k, v, protected); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (m *mockBulkStore) GetAll(vault string) (map[string]string, error) {
	keys, err := m.mockStore.List(vault)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(keys))
	for _, k := range keys {
		v, err := m.mockStore.Get(vault, k)
		if err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, nil
}

func (m *mockBulkStore) ReadRawService(service string) (map[string]string, error) {
	svc, ok := m.rawServices[service]
	if !ok {
		return map[string]string{}, nil
	}
	result := make(map[string]string, len(svc))
	for k, v := range svc {
		result[k] = v
	}
	return result, nil
}

// --- Tests ---

func TestGetSuccess(t *testing.T) {
	app, store, _, clip := newTestApp()
	if err := store.Set("default", "API_KEY", "secret123"); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeCmd(app, "get", "API_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout); got != "********" {
		t.Errorf("stdout = %q, want %q", got, "********")
	}
	if !strings.Contains(stderr, "Copied to clipboard") {
		t.Errorf("stderr = %q, want clipboard confirmation", stderr)
	}
	if clip.last != "secret123" {
		t.Errorf("clipboard = %q, want %q", clip.last, "secret123")
	}
}

func TestGetNotFound(t *testing.T) {
	app, _, _, _ := newTestApp()

	_, _, err := executeCmd(app, "get", "NOPE")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !strings.Contains(err.Error(), "NOPE") {
		t.Errorf("error = %q, want it to mention the key", err.Error())
	}
}

func TestGetWithVaultFlag(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	if err := vaults.Create("staging"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("staging", "DB_PASS", "pg123"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "get", "DB_PASS", "--vault", "staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout); got != "********" {
		t.Errorf("stdout = %q, want %q", got, "********")
	}
}

func TestGetMissingArg(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "get")
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}

func TestSetSuccess(t *testing.T) {
	app, store, _, _ := newTestApp()

	stdout, _, err := executeCmd(app, "set", "TOKEN", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Stored") {
		t.Errorf("stdout = %q, want confirmation", stdout)
	}

	val, _ := store.Get("default", "TOKEN")
	if val != "abc" {
		t.Errorf("stored value = %q, want %q", val, "abc")
	}
}

func TestSetWithVaultFlag(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "set", "KEY", "val", "--vault", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, _ := store.Get("prod", "KEY")
	if val != "val" {
		t.Errorf("stored value = %q, want %q", val, "val")
	}
}

func TestSetWithUnknownVaultFlag(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "set", "KEY", "val", "--vault", "missing")
	if err == nil {
		t.Fatal("expected error for unknown vault")
	}
}

func TestGetWithInvalidVaultFlag(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "get", "KEY", "--vault", "bad name")
	if err == nil {
		t.Fatal("expected error for invalid vault name")
	}
}

func TestSetMissingArgs(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "set", "KEY")
	if err == nil {
		t.Fatal("expected error for missing value argument")
	}
}

func TestDelSuccess(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.Set("default", "OLD_KEY", "oldval"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "del", "OLD_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Deleted") {
		t.Errorf("stdout = %q, want confirmation", stdout)
	}

	_, getErr := store.Get("default", "OLD_KEY")
	if getErr == nil {
		t.Error("expected key to be deleted")
	}
}

func TestDelNotFound(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "del", "GHOST")
	if err == nil {
		t.Fatal("expected error deleting non-existent key")
	}
}

func TestDelAlias(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.Set("default", "X", "y"); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "rm", "X")
	if err != nil {
		t.Fatalf("unexpected error using 'rm' alias: %v", err)
	}
}

func TestListSuccess(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.Set("default", "A", "1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "B", "2"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "A") || !strings.Contains(stdout, "B") {
		t.Errorf("stdout = %q, want keys A and B", stdout)
	}
}

func TestListEmpty(t *testing.T) {
	app, _, _, _ := newTestApp()

	stdout, _, err := executeCmd(app, "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "No keys") {
		t.Errorf("stdout = %q, want empty message", stdout)
	}
}

func TestListWithVault(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	if err := vaults.Create("staging"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("staging", "DB_HOST", "localhost"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "list", "--vault", "staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "DB_HOST") {
		t.Errorf("stdout = %q, want DB_HOST", stdout)
	}
}

func TestListAlias(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "ls")
	if err != nil {
		t.Fatalf("unexpected error using 'ls' alias: %v", err)
	}
}

func TestVaultList(t *testing.T) {
	app, _, _, _ := newTestApp()

	stdout, _, err := executeCmd(app, "vault", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "default") {
		t.Errorf("stdout = %q, want default vault", stdout)
	}
	// Active vault should be marked with *.
	if !strings.Contains(stdout, "* default") {
		t.Errorf("stdout = %q, want active marker on default", stdout)
	}
}

func TestVaultCreate(t *testing.T) {
	app, _, vaults, _ := newTestApp()

	stdout, _, err := executeCmd(app, "vault", "create", "staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Created") {
		t.Errorf("stdout = %q, want confirmation", stdout)
	}
	if len(vaults.vaults) != 2 {
		t.Errorf("vaults count = %d, want 2", len(vaults.vaults))
	}
}

func TestVaultCreateDuplicate(t *testing.T) {
	app, _, _, _ := newTestApp()

	_, _, err := executeCmd(app, "vault", "create", "default")
	if err == nil {
		t.Fatal("expected error creating duplicate vault")
	}
}

func TestVaultSwitch(t *testing.T) {
	app, _, vaults, _ := newTestApp()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "vault", "switch", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Switched") {
		t.Errorf("stdout = %q, want confirmation", stdout)
	}
	if vaults.active != "prod" {
		t.Errorf("active = %q, want %q", vaults.active, "prod")
	}
}

func TestVaultSwitchNonexistent(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "vault", "switch", "nope")
	if err == nil {
		t.Fatal("expected error switching to non-existent vault")
	}
}

func TestActiveVaultFallback(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	if err := vaults.Create("staging"); err != nil {
		t.Fatal(err)
	}
	if err := vaults.Switch("staging"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("staging", "MYKEY", "myval"); err != nil {
		t.Fatal(err)
	}

	// No --vault flag → uses active vault (staging).
	stdout, _, err := executeCmd(app, "get", "MYKEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout); got != "********" {
		t.Errorf("stdout = %q, want %q", got, "********")
	}
}

func TestDefaultVaultFallback(t *testing.T) {
	store := newMockStore()
	// VaultManager with empty active.
	vaults := &mockVaultManager{vaults: []string{"default"}, active: ""}
	app := &cli.App{Store: store, Vaults: vaults, Clipboard: nil}

	if err := store.Set("default", "X", "y"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "get", "X")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout); got != "********" {
		t.Errorf("stdout = %q, want %q", got, "********")
	}
}

func TestGetWithNilClipboard(t *testing.T) {
	store := newMockStore()
	vaults := &mockVaultManager{vaults: []string{"default"}, active: "default"}
	app := &cli.App{Store: store, Vaults: vaults, Clipboard: nil}

	if err := store.Set("default", "K", "V"); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeCmd(app, "get", "K")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout); got != "********" {
		t.Errorf("stdout = %q, want %q", got, "********")
	}
	if strings.Contains(stderr, "clipboard") {
		t.Error("should not mention clipboard when nil")
	}
}

func TestGetEmptyValueMask(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.Set("default", "EMPTY", ""); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "get", "EMPTY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout); got != "[empty]" {
		t.Errorf("stdout = %q, want %q", got, "[empty]")
	}
}

func TestRootHelp(t *testing.T) {
	app, _, _, _ := newTestApp()

	stdout, _, err := executeCmd(app, "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "kc") {
		t.Errorf("help output should mention 'kc'")
	}
	// Verify all subcommands are listed.
	for _, sub := range []string{"get", "set", "del", "list", "vault"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("help output missing subcommand %q", sub)
		}
	}
}

func TestVersion(t *testing.T) {
	app, _, _, _ := newTestApp()

	stdout, _, err := executeCmd(app, "--version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "dev") {
		t.Errorf("version output = %q, want 'dev'", stdout)
	}
}

func TestUnknownCommand(t *testing.T) {
	app, _, _, _ := newTestApp()
	_, _, err := executeCmd(app, "bogus")
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func newTestAppWithBulk() (*cli.App, *mockStore, *mockBulkStore, *mockVaultManager) {
	store := newMockStore()
	bulk := &mockBulkStore{mockStore: store, rawServices: make(map[string]map[string]string)}
	vaults := &mockVaultManager{vaults: []string{"default"}, active: "default"}
	clip := &mockClipboard{}
	app := &cli.App{Store: store, Bulk: bulk, Vaults: vaults, Clipboard: clip}
	return app, store, bulk, vaults
}

func newAuthorizedTestAppWithBulk() (*cli.App, *mockStore, *mockBulkStore, *mockVaultManager, *countingAuthorizer) {
	app, store, bulk, vaults := newTestAppWithBulk()
	authorizer := &countingAuthorizer{}
	app.Auth = authorizer
	return app, store, bulk, vaults, authorizer
}

func TestImportDotEnv_ActiveVault(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()

	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("API_KEY=secret123\nDB_PASS=pg456\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "import", envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Imported 2") {
		t.Errorf("stdout = %q, want import count message", stdout)
	}

	if val, _ := store.Get("default", "API_KEY"); val != "secret123" {
		t.Errorf("API_KEY = %q, want %q", val, "secret123")
	}
	if val, _ := store.Get("default", "DB_PASS"); val != "pg456" {
		t.Errorf("DB_PASS = %q, want %q", val, "pg456")
	}
}

func TestImportDotEnv_ExplicitVault(t *testing.T) {
	app, store, _, vaults := newTestAppWithBulk()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}

	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("TOKEN=abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "import", envFile, "--vault", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val, _ := store.Get("prod", "TOKEN"); val != "abc" {
		t.Errorf("TOKEN = %q, want %q", val, "abc")
	}
}

func TestImportDotEnv_ParsesQuotedValues(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()

	envFile := filepath.Join(t.TempDir(), ".env")
	content := `SINGLE='hello world'
DOUBLE="foo bar"
UNQUOTED=plain
`
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "import", envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]string{
		"SINGLE":   "hello world",
		"DOUBLE":   "foo bar",
		"UNQUOTED": "plain",
	}
	for k, want := range cases {
		got, _ := store.Get("default", k)
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestImportDotEnv_SkipsCommentsAndBlankLines(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()

	envFile := filepath.Join(t.TempDir(), ".env")
	content := `# this is a comment
REAL_KEY=value

# another comment
`
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "import", envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Imported 1") {
		t.Errorf("stdout = %q, want Imported 1", stdout)
	}
	if val, _ := store.Get("default", "REAL_KEY"); val != "value" {
		t.Errorf("REAL_KEY = %q, want %q", val, "value")
	}
}

func TestImportDotEnv_StripsInlineComments(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()

	envFile := filepath.Join(t.TempDir(), ".env")
	content := "PLAIN=value # trailing comment\nQUOTED=\"value # kept\"\nSINGLE='value # kept too'\n"
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeCmd(app, "import", envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val, _ := store.Get("default", "PLAIN"); val != "value" {
		t.Errorf("PLAIN = %q, want %q", val, "value")
	}
	if val, _ := store.Get("default", "QUOTED"); val != "value # kept" {
		t.Errorf("QUOTED = %q, want %q", val, "value # kept")
	}
	if val, _ := store.Get("default", "SINGLE"); val != "value # kept too" {
		t.Errorf("SINGLE = %q, want %q", val, "value # kept too")
	}
}

func TestImportDotEnv_ExactReportMessage(t *testing.T) {
	app, _, _, vaults := newTestAppWithBulk()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}

	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("TOKEN=abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "import", envFile, "--vault", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := strings.TrimSpace(stdout); got != "Imported 1 keys into vault prod" {
		t.Errorf("stdout = %q, want %q", got, "Imported 1 keys into vault prod")
	}
}

func TestImportExportImport_RoundTripsApostrophes(t *testing.T) {
	app, store, _, vaults := newTestAppWithBulk()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "AUTHOR", "O'Reilly"); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(t.TempDir(), "roundtrip.env")
	_, _, err := executeCmd(app, "export", "--vault", "prod", "-o", outFile)
	if err != nil {
		t.Fatalf("unexpected export error: %v", err)
	}

	if err := vaults.Create("stage"); err != nil {
		t.Fatal(err)
	}
	_, _, err = executeCmd(app, "import", outFile, "--vault", "stage")
	if err != nil {
		t.Fatalf("unexpected import error: %v", err)
	}

	if got, _ := store.Get("stage", "AUTHOR"); got != "O'Reilly" {
		t.Errorf("AUTHOR = %q, want %q", got, "O'Reilly")
	}
}

func TestImportDotEnv_MissingFile(t *testing.T) {
	app, _, _, _ := newTestAppWithBulk()

	_, _, err := executeCmd(app, "import", "/tmp/nonexistent-kc-test.env")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestExport_ActiveVault(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()
	if err := store.Set("default", "FOO", "bar"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "BAZ", "qux"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "export")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "FOO=bar") {
		t.Errorf("stdout = %q, want FOO=bar", stdout)
	}
	if !strings.Contains(stdout, "BAZ=qux") {
		t.Errorf("stdout = %q, want BAZ=qux", stdout)
	}
}

func TestExport_ExplicitVault(t *testing.T) {
	app, store, _, vaults := newTestAppWithBulk()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("prod", "SECRET", "s3cr3t"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "export", "--vault", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "SECRET=s3cr3t") {
		t.Errorf("stdout = %q, want SECRET=s3cr3t", stdout)
	}
}

func TestExport_OutputFile(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()
	if err := store.Set("default", "KEY", "value"); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(t.TempDir(), "out.env")
	_, _, err := executeCmd(app, "export", "-o", outFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if !strings.Contains(string(data), "KEY=value") {
		t.Errorf("output file = %q, want KEY=value", string(data))
	}
}

func TestExport_QuotesValuesNeedingEnvEscaping(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()
	if err := store.Set("default", "PLAIN", "value"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "SPACED", "two words"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "HASHED", "value # comment"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "APOSTROPHE", "O'Reilly"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "export")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "PLAIN=value") {
		t.Errorf("stdout = %q, want unquoted plain value", stdout)
	}
	if !strings.Contains(stdout, "SPACED=\"two words\"") {
		t.Errorf("stdout = %q, want double-quoted spaced value", stdout)
	}
	if !strings.Contains(stdout, "HASHED=\"value # comment\"") {
		t.Errorf("stdout = %q, want quoted hash value", stdout)
	}
	if !strings.Contains(stdout, "APOSTROPHE=\"O'Reilly\"") {
		t.Errorf("stdout = %q, want apostrophe-safe dotenv quoting", stdout)
	}
}

func TestExport_ProtectedVaultAuthorizesBeforeExport(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	if err := store.SetWithProtection("default", "SECRET", "s3cr3t", true); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "export")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}
	if !strings.Contains(stdout, "SECRET=s3cr3t") {
		t.Fatalf("stdout = %q, want exported secret", stdout)
	}
	if strings.Contains(stdout, "authentication failed") {
		t.Fatalf("stdout = %q, should not leak auth errors", stdout)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Fatalf("stdout = %q, want trailing newline", stdout)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls after export = %d, want 1", authorizer.calls)
	}
	if app.Auth == nil {
		t.Fatal("app.Auth should remain configured")
	}
}

func TestExport_ProtectedVaultReturnsErrorBeforeWritingFile(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	authorizer.err = fmt.Errorf("denied")
	if err := store.SetWithProtection("default", "SECRET", "s3cr3t", true); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(t.TempDir(), "out.env")
	_, _, err := executeCmd(app, "export", "-o", outFile)
	if err == nil {
		t.Fatal("expected export error")
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}
	if _, statErr := os.Stat(outFile); !os.IsNotExist(statErr) {
		t.Fatalf("Stat error = %v, want not exist", statErr)
	}
}

func TestEnv_ProtectedVaultAuthorizesBeforeReadingValues(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	if err := store.SetWithProtection("default", "PUBLIC", "value", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWithProtection("default", "SECRET", "s3cr3t", true); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}
	if !strings.Contains(stdout, "export PUBLIC=") || !strings.Contains(stdout, "export SECRET=") {
		t.Fatalf("stdout = %q, want both exports", stdout)
	}
}

func TestListShowValues_ProtectedVaultAuthorizesOnce(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	if err := store.SetWithProtection("default", "SECRET_A", "one", true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWithProtection("default", "SECRET_B", "two", true); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "list", "--show-values")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}
	if !strings.Contains(stdout, "SECRET_A=one") || !strings.Contains(stdout, "SECRET_B=two") {
		t.Fatalf("stdout = %q, want both values", stdout)
	}
}

func TestSearchShowValues_ProtectedMatchAuthorizesOnce(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	if err := store.SetWithProtection("default", "MONGO_URL", "mongodb://default", true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWithProtection("default", "MONGO_PUBLIC", "mongodb://public", false); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "search", "mongo", "--show-values")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}
	if !strings.Contains(stdout, "mongodb://default") || !strings.Contains(stdout, "mongodb://public") {
		t.Fatalf("stdout = %q, want values in output", stdout)
	}
}

func TestEnv_ActiveVault(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()
	if err := store.Set("default", "MY_VAR", "my_val"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "export MY_VAR=") {
		t.Errorf("stdout = %q, want export MY_VAR=...", stdout)
	}
	if !strings.Contains(stdout, "my_val") {
		t.Errorf("stdout = %q, want value my_val", stdout)
	}
}

func TestEnv_ExplicitVault(t *testing.T) {
	app, store, _, vaults := newTestAppWithBulk()
	if err := vaults.Create("staging"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("staging", "HOST", "localhost"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "env", "--vault", "staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "export HOST=") {
		t.Errorf("stdout = %q, want export HOST=...", stdout)
	}
}

func TestEnv_ShellSafeQuoting(t *testing.T) {
	app, store, _, _ := newTestAppWithBulk()
	if err := store.Set("default", "SPECIAL", "has spaces & 'quotes'"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout, "has spaces & 'quotes'") {
		t.Errorf("stdout = %q, value should be quoted for shell safety", stdout)
	}
	if !strings.Contains(stdout, "export SPECIAL=") {
		t.Errorf("stdout = %q, want export SPECIAL=...", stdout)
	}
}

func TestMigrate_FromRawService(t *testing.T) {
	app, store, bulk, _ := newTestAppWithBulk()
	bulk.rawServices["zshrc-secrets"] = map[string]string{
		"GH_TOKEN": "ghp_123",
		"AWS_KEY":  "aws_456",
	}

	stdout, _, err := executeCmd(app, "migrate", "--from", "zshrc-secrets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Migrated 2") {
		t.Errorf("stdout = %q, want Migrated 2", stdout)
	}

	if val, _ := store.Get("default", "GH_TOKEN"); val != "ghp_123" {
		t.Errorf("GH_TOKEN = %q, want %q", val, "ghp_123")
	}
	if val, _ := store.Get("default", "AWS_KEY"); val != "aws_456" {
		t.Errorf("AWS_KEY = %q, want %q", val, "aws_456")
	}
}

func TestMigrate_IntoExplicitVault(t *testing.T) {
	app, store, bulk, vaults := newTestAppWithBulk()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	bulk.rawServices["legacy-app"] = map[string]string{
		"DB_URL": "postgres://localhost/db",
	}

	_, _, err := executeCmd(app, "migrate", "--from", "legacy-app", "--vault", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val, _ := store.Get("prod", "DB_URL"); val != "postgres://localhost/db" {
		t.Errorf("DB_URL = %q, want postgres://localhost/db", val)
	}
}

func TestMigrate_MissingFromFlag(t *testing.T) {
	app, _, _, _ := newTestAppWithBulk()
	_, _, err := executeCmd(app, "migrate")
	if err == nil {
		t.Fatal("expected error when --from flag is missing")
	}
}

func TestGetCompletesKeys(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.Set("default", "API_KEY", "s1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "DB_PASS", "s2"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "__complete", "get", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "API_KEY") {
		t.Errorf("completion missing API_KEY: %q", stdout)
	}
	if !strings.Contains(stdout, "DB_PASS") {
		t.Errorf("completion missing DB_PASS: %q", stdout)
	}
	if !strings.Contains(stdout, ":4") {
		t.Errorf("expected NoFileComp directive (:4) in output: %q", stdout)
	}
}

func TestDelCompletesKeys(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.Set("default", "OLD_KEY", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "OTHER_KEY", "v2"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "__complete", "del", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "OLD_KEY") {
		t.Errorf("completion missing OLD_KEY: %q", stdout)
	}
	if !strings.Contains(stdout, "OTHER_KEY") {
		t.Errorf("completion missing OTHER_KEY: %q", stdout)
	}
	if !strings.Contains(stdout, ":4") {
		t.Errorf("expected NoFileComp directive (:4) in output: %q", stdout)
	}
}

func TestGetCompletesKeysWithVaultFlag(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	if err := vaults.Create("staging"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("staging", "STAGE_KEY", "sv1"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "__complete", "get", "--vault", "staging", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "STAGE_KEY") {
		t.Errorf("completion missing STAGE_KEY: %q", stdout)
	}
}

func TestGetCompletionFiltersByPrefix(t *testing.T) {
	app, store, _, _ := newTestApp()
	if err := store.Set("default", "API_KEY", "s1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("default", "DB_PASS", "s2"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "__complete", "get", "A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "API_KEY") {
		t.Fatalf("completion missing API_KEY: %q", stdout)
	}
	if strings.Contains(stdout, "DB_PASS") {
		t.Fatalf("completion should filter out DB_PASS for prefix A: %q", stdout)
	}
}

func TestVaultFlagCompletionFiltersByPrefix(t *testing.T) {
	app, _, vaults, _ := newTestApp()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := vaults.Create("staging"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "__complete", "get", "--vault", "st")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "staging") {
		t.Fatalf("completion missing staging: %q", stdout)
	}
	if strings.Contains(stdout, "default") || strings.Contains(stdout, "prod") {
		t.Fatalf("vault completion should filter to prefix st: %q", stdout)
	}
}

func TestVaultSwitchCompletesVaultNames(t *testing.T) {
	app, _, vaults, _ := newTestApp()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := vaults.Create("staging"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "__complete", "vault", "switch", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "prod") {
		t.Errorf("completion missing prod: %q", stdout)
	}
	if !strings.Contains(stdout, "staging") {
		t.Errorf("completion missing staging: %q", stdout)
	}
	if !strings.Contains(stdout, ":4") {
		t.Errorf("expected NoFileComp directive (:4) in output: %q", stdout)
	}
}

func TestVaultFlagCompletesVaultNames(t *testing.T) {
	app, _, vaults, _ := newTestApp()
	if err := vaults.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := vaults.Create("staging"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeCmd(app, "__complete", "get", "--vault", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "default") {
		t.Errorf("completion missing default: %q", stdout)
	}
	if !strings.Contains(stdout, "prod") {
		t.Errorf("completion missing prod: %q", stdout)
	}
	if !strings.Contains(stdout, "staging") {
		t.Errorf("completion missing staging: %q", stdout)
	}
	if !strings.Contains(stdout, ":4") {
		t.Errorf("expected NoFileComp directive (:4) in output: %q", stdout)
	}
}
