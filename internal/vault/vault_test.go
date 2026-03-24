package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/v-gutierrez/kc/internal/keychain"
)

// mockKC implements KeychainBackend for testing.
type mockKC struct {
	store     map[string]map[string]string // service -> account -> password
	protected map[string]map[string]bool
}

func newMockKC() *mockKC {
	return &mockKC{store: make(map[string]map[string]string), protected: make(map[string]map[string]bool)}
}

func (m *mockKC) Get(service, account string) (string, error) {
	svc, ok := m.store[service]
	if !ok {
		return "", errors.New("not found")
	}
	val, ok := svc[account]
	if !ok {
		return "", errors.New("not found")
	}
	return val, nil
}

func (m *mockKC) Set(service, account, password string) error {
	return m.SetWithProtection(service, account, password, true)
}

func (m *mockKC) SetWithProtection(service, account, password string, protected bool) error {
	if m.store[service] == nil {
		m.store[service] = make(map[string]string)
	}
	if m.protected[service] == nil {
		m.protected[service] = make(map[string]bool)
	}
	m.store[service][account] = password
	m.protected[service][account] = protected
	return nil
}

func (m *mockKC) Delete(service, account string) error {
	svc, ok := m.store[service]
	if !ok {
		return errors.New("not found")
	}
	if _, ok := svc[account]; !ok {
		return errors.New("not found")
	}
	delete(svc, account)
	if m.protected[service] != nil {
		delete(m.protected[service], account)
	}
	return nil
}

func (m *mockKC) List(service string) ([]string, error) {
	svc, ok := m.store[service]
	if !ok {
		return nil, nil
	}
	var keys []string
	for k := range svc {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mockKC) ListMetadata(service string) ([]keychain.ItemMetadata, error) {
	svc, ok := m.store[service]
	if !ok {
		return nil, nil
	}
	items := make([]keychain.ItemMetadata, 0, len(svc))
	for key := range svc {
		items = append(items, keychain.ItemMetadata{Account: key, Protected: m.protected[service][key]})
	}
	return items, nil
}

func (m *mockKC) ProtectAll(service string) (int, error) {
	svc, ok := m.store[service]
	if !ok {
		return 0, nil
	}
	if m.protected[service] == nil {
		m.protected[service] = make(map[string]bool)
	}
	count := 0
	for key := range svc {
		if m.protected[service][key] {
			continue
		}
		m.protected[service][key] = true
		count++
	}
	return count, nil
}

// newTestManager creates a Manager with a temp data dir.
func newTestManager(t *testing.T) (*Manager, *mockKC) {
	t.Helper()
	kc := newMockKC()
	dir := t.TempDir()
	return &Manager{KC: kc, DataDir: dir}, kc
}

// --- ServiceName ---

func TestServiceName(t *testing.T) {
	if got := ServiceName("prod"); got != "kc:prod" {
		t.Fatalf("got %q, want %q", got, "kc:prod")
	}
	if got := ServiceName("default"); got != "kc:default" {
		t.Fatalf("got %q, want %q", got, "kc:default")
	}
}

// --- ActiveVault ---

func TestActiveVault_Default(t *testing.T) {
	mgr, _ := newTestManager(t)
	if got := mgr.ActiveVault(); got != DefaultVault {
		t.Fatalf("got %q, want %q", got, DefaultVault)
	}
}

func TestActiveVault_Persisted(t *testing.T) {
	mgr, _ := newTestManager(t)
	// Manually write active_vault file
	if err := os.MkdirAll(mgr.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mgr.DataDir, "active_vault"), []byte("staging\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := mgr.ActiveVault(); got != "staging" {
		t.Fatalf("got %q, want %q", got, "staging")
	}
}

// --- Create ---

func TestCreate_Success(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.Create("staging"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vaults, err := mgr.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	if len(vaults) != 2 {
		t.Fatalf("expected 2 vaults, got %d: %v", len(vaults), vaults)
	}
}

func TestCreate_Duplicate(t *testing.T) {
	mgr, _ := newTestManager(t)
	_ = mgr.Create("staging")

	err := mgr.Create("staging")
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestCreate_InvalidName(t *testing.T) {
	mgr, _ := newTestManager(t)

	tests := []string{"", "has space", "foo/bar", "a.b"}
	for _, name := range tests {
		err := mgr.Create(name)
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("Create(%q) = %v, want ErrInvalidName", name, err)
		}
	}
}

// --- ListVaults ---

func TestListVaults_InitializesDefault(t *testing.T) {
	mgr, _ := newTestManager(t)

	vaults, err := mgr.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	if len(vaults) != 1 || vaults[0] != DefaultVault {
		t.Fatalf("expected [default], got %v", vaults)
	}
}

func TestListVaults_Sorted(t *testing.T) {
	mgr, _ := newTestManager(t)
	_ = mgr.Create("zebra")
	_ = mgr.Create("alpha")

	vaults, err := mgr.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"alpha", "default", "zebra"}
	if len(vaults) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, vaults)
	}
	for i, v := range vaults {
		if v != expected[i] {
			t.Fatalf("vaults[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

// --- Switch ---

func TestSwitch_Success(t *testing.T) {
	mgr, _ := newTestManager(t)
	_ = mgr.Create("staging")

	if err := mgr.Switch("staging"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mgr.ActiveVault(); got != "staging" {
		t.Fatalf("got %q, want %q", got, "staging")
	}
}

func TestSwitch_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	err := mgr.Switch("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSwitch_InvalidName(t *testing.T) {
	mgr, _ := newTestManager(t)

	err := mgr.Switch("")
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
}

// --- CRUD pass-through ---

func TestGet_UsesActiveVault(t *testing.T) {
	mgr, kc := newTestManager(t)
	if err := kc.Set("kc:default", "API_KEY", "secret123"); err != nil {
		t.Fatal(err)
	}

	val, err := mgr.Get("API_KEY", "")
	if err != nil {
		t.Fatal(err)
	}
	if val != "secret123" {
		t.Fatalf("got %q, want %q", val, "secret123")
	}
}

func TestGet_UsesExplicitVault(t *testing.T) {
	mgr, kc := newTestManager(t)
	_ = mgr.Create("prod")
	if err := kc.Set("kc:prod", "DB_PASS", "prod-pass"); err != nil {
		t.Fatal(err)
	}

	val, err := mgr.Get("DB_PASS", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if val != "prod-pass" {
		t.Fatalf("got %q, want %q", val, "prod-pass")
	}
}

func TestSet_UsesActiveVault(t *testing.T) {
	mgr, kc := newTestManager(t)

	if err := mgr.Set("TOKEN", "abc", ""); err != nil {
		t.Fatal(err)
	}
	// Verify it went into kc:default
	val, err := kc.Get("kc:default", "TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if val != "abc" {
		t.Fatalf("got %q, want %q", val, "abc")
	}
}

func TestDelete_UsesActiveVault(t *testing.T) {
	mgr, kc := newTestManager(t)
	if err := kc.Set("kc:default", "TOKEN", "xyz"); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Delete("TOKEN", ""); err != nil {
		t.Fatal(err)
	}
	_, err := kc.Get("kc:default", "TOKEN")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestListKeys(t *testing.T) {
	mgr, kc := newTestManager(t)
	if err := mgr.Create("default"); err != nil && !errors.Is(err, ErrAlreadyExists) {
		t.Fatal(err)
	}
	if err := kc.Set("kc:default", "A", "1"); err != nil {
		t.Fatal(err)
	}
	if err := kc.Set("kc:default", "B", "2"); err != nil {
		t.Fatal(err)
	}

	keys, err := mgr.ListKeys("")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestSet_UnknownExplicitVault(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.Set("TOKEN", "abc", "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGet_InvalidExplicitVaultName(t *testing.T) {
	mgr, _ := newTestManager(t)
	_, err := mgr.Get("TOKEN", "bad name")
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
}

// --- ReadRawService (migrate path) ---

func TestReadRawService_ReadsArbitraryService(t *testing.T) {
	mgr, kc := newTestManager(t)
	if err := kc.Set("zshrc-secrets", "GITHUB_TOKEN", "ghp_abc"); err != nil {
		t.Fatal(err)
	}
	if err := kc.Set("zshrc-secrets", "AWS_SECRET", "aws-secret"); err != nil {
		t.Fatal(err)
	}

	entries, err := mgr.ReadRawService("zshrc-secrets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(entries), entries)
	}
	if entries["GITHUB_TOKEN"] != "ghp_abc" {
		t.Errorf("GITHUB_TOKEN = %q, want %q", entries["GITHUB_TOKEN"], "ghp_abc")
	}
	if entries["AWS_SECRET"] != "aws-secret" {
		t.Errorf("AWS_SECRET = %q, want %q", entries["AWS_SECRET"], "aws-secret")
	}
}

func TestReadRawService_EmptyService(t *testing.T) {
	mgr, _ := newTestManager(t)

	_, err := mgr.ReadRawService("")
	if err == nil {
		t.Fatal("expected error for empty service name")
	}
}

func TestReadRawService_NoEntries(t *testing.T) {
	mgr, _ := newTestManager(t)

	entries, err := mgr.ReadRawService("nonexistent-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// --- BulkSet (import path) ---

func TestBulkSet_StoresAllEntries(t *testing.T) {
	mgr, kc := newTestManager(t)
	_ = mgr.Create("default")

	entries := map[string]string{
		"API_KEY": "secret1",
		"DB_PASS": "secret2",
		"TOKEN":   "secret3",
	}

	n, err := mgr.BulkSet(entries, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("BulkSet returned %d, want 3", n)
	}

	for k, want := range entries {
		got, err := kc.Get("kc:default", k)
		if err != nil {
			t.Errorf("key %q not stored: %v", k, err)
		}
		if got != want {
			t.Errorf("key %q = %q, want %q", k, got, want)
		}
	}
}

func TestBulkSet_ExplicitVault(t *testing.T) {
	mgr, kc := newTestManager(t)
	_ = mgr.Create("prod")

	entries := map[string]string{"SECRET": "value"}
	_, err := mgr.BulkSet(entries, "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := kc.Get("kc:prod", "SECRET")
	if err != nil {
		t.Fatalf("key not found: %v", err)
	}
	if got != "value" {
		t.Errorf("got %q, want %q", got, "value")
	}
}

func TestBulkSet_UnknownVault(t *testing.T) {
	mgr, _ := newTestManager(t)

	_, err := mgr.BulkSet(map[string]string{"K": "V"}, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- GetAllKeys (export/env path) ---

func TestGetAllKeys_ReturnsAllKeyValues(t *testing.T) {
	mgr, kc := newTestManager(t)
	if err := kc.Set("kc:default", "FOO", "bar"); err != nil {
		t.Fatal(err)
	}
	if err := kc.Set("kc:default", "BAZ", "qux"); err != nil {
		t.Fatal(err)
	}

	entries, err := mgr.GetAllKeys("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", entries["FOO"], "bar")
	}
	if entries["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want %q", entries["BAZ"], "qux")
	}
}

func TestGetAllKeys_ExplicitVault(t *testing.T) {
	mgr, kc := newTestManager(t)
	_ = mgr.Create("staging")
	if err := kc.Set("kc:staging", "HOST", "localhost"); err != nil {
		t.Fatal(err)
	}

	entries, err := mgr.GetAllKeys("staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries["HOST"] != "localhost" {
		t.Errorf("HOST = %q, want %q", entries["HOST"], "localhost")
	}
}

// --- DeleteVault ---

func TestDeleteVault_Empty(t *testing.T) {
	mgr, _ := newTestManager(t)
	_ = mgr.Create("staging")

	if err := mgr.DeleteVault("staging", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vaults, err := mgr.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vaults {
		if v == "staging" {
			t.Fatal("staging should have been removed from vault list")
		}
	}
}

func TestDeleteVault_WithKeysNoForce(t *testing.T) {
	mgr, kc := newTestManager(t)
	_ = mgr.Create("staging")
	_ = kc.Set("kc:staging", "KEY1", "v1")
	_ = kc.Set("kc:staging", "KEY2", "v2")

	err := mgr.DeleteVault("staging", false)
	if err == nil {
		t.Fatal("expected error when vault has keys and force=false")
	}
	want := "Vault has 2 keys. Delete them first or use --force."
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestDeleteVault_WithKeysForce(t *testing.T) {
	mgr, kc := newTestManager(t)
	_ = mgr.Create("staging")
	_ = kc.Set("kc:staging", "KEY1", "v1")
	_ = kc.Set("kc:staging", "KEY2", "v2")

	if err := mgr.DeleteVault("staging", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vaults, _ := mgr.ListVaults()
	for _, v := range vaults {
		if v == "staging" {
			t.Fatal("staging should have been removed")
		}
	}

	keys, _ := kc.List("kc:staging")
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}

func TestDeleteVault_DefaultVault(t *testing.T) {
	mgr, _ := newTestManager(t)

	err := mgr.DeleteVault("default", false)
	if !errors.Is(err, ErrDefaultVault) {
		t.Fatalf("expected ErrDefaultVault, got %v", err)
	}
}

func TestDeleteVault_DefaultVaultForce(t *testing.T) {
	mgr, _ := newTestManager(t)

	err := mgr.DeleteVault("default", true)
	if !errors.Is(err, ErrDefaultVault) {
		t.Fatalf("expected ErrDefaultVault even with force, got %v", err)
	}
}

func TestDeleteVault_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	err := mgr.DeleteVault("nonexistent", false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteVault_InvalidName(t *testing.T) {
	mgr, _ := newTestManager(t)

	err := mgr.DeleteVault("bad name", false)
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
}

func TestDeleteVault_ActiveVaultSwitchesToDefault(t *testing.T) {
	mgr, _ := newTestManager(t)
	_ = mgr.Create("staging")
	_ = mgr.Switch("staging")

	if err := mgr.DeleteVault("staging", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mgr.ActiveVault(); got != DefaultVault {
		t.Fatalf("active vault = %q, want %q", got, DefaultVault)
	}
}

// --- validateName ---

func TestValidateName(t *testing.T) {
	valid := []string{"default", "prod", "staging-2", "my_vault", "A1"}
	for _, n := range valid {
		if err := validateName(n); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", n, err)
		}
	}

	invalid := []string{"", "has space", "a/b", "x.y", "hello!"}
	for _, n := range invalid {
		if err := validateName(n); !errors.Is(err, ErrInvalidName) {
			t.Errorf("validateName(%q) = %v, want ErrInvalidName", n, err)
		}
	}
}
