// Package vault manages named vaults backed by macOS Keychain services.
//
// Each vault maps to a Keychain service with the prefix "kc:" (e.g. "kc:default").
// The active vault is resolved per invocation (see context.go) and persisted to
// ~/.kc/active_vault. Vault metadata (the list of known vaults, plus optional
// description, tags and flags) is materialized in ~/.kc/vaults.
//
// Every write goes through SetWithOptions, which records the value it is about
// to replace (see the history package) and enforces a vault's protection
// policy. Nothing in kc overwrites a secret without leaving a way back.
package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/v-gutierrez/kc/internal/config"
	"github.com/v-gutierrez/kc/internal/history"
	"github.com/v-gutierrez/kc/internal/keychain"
)

const (
	// ServicePrefix is prepended to vault names when stored in Keychain.
	ServicePrefix = "kc:"
	// DefaultVault is the vault used when none is specified.
	DefaultVault = "default"
)

// Common errors.
var (
	ErrNotFound           = errors.New("vault: not found")
	ErrAlreadyExists      = errors.New("vault: already exists")
	ErrInvalidName        = errors.New("vault: invalid name (must be non-empty alphanumeric/dash/underscore)")
	ErrDefaultVault       = errors.New("vault: cannot delete the default vault")
	ErrProtectionRequired = errors.New("vault: requires Touch ID protected secrets")
)

// KeychainBackend is the subset of keychain operations vault needs.
type KeychainBackend interface {
	Get(service, account string) (string, error)
	Set(service, account, password string) error
	SetWithProtection(service, account, password string, protected bool) error
	Delete(service, account string) error
	List(service string) ([]string, error)
	ListMetadata(service string) ([]keychain.ItemMetadata, error)
	ProtectAll(service string) (int, error)
}

type SecretMetadata struct {
	Key        string
	Protection string
	Modified   string
}

const (
	ProtectionUnknown     = "unknown"
	ProtectionProtected   = "protected"
	ProtectionUnprotected = "unprotected"
)

// SetOptions controls a single write.
type SetOptions struct {
	// Protected stores the secret behind Touch ID.
	Protected bool
	// SkipHistory writes without recording the value being replaced.
	SkipHistory bool
	// Retention overrides how many previous versions to keep for this key.
	Retention int
}

// Manager handles vault lifecycle.
type Manager struct {
	KC      KeychainBackend
	DataDir string // defaults to ~/.kc
	WorkDir string // defaults to the process working directory

	settings *config.Config
	recorder *history.Recorder
}

// New creates a Manager with the given backend and default data dir (~/.kc).
func New(kc KeychainBackend) *Manager {
	home, _ := os.UserHomeDir()
	return &Manager{
		KC:      kc,
		DataDir: filepath.Join(home, ".kc"),
	}
}

// ServiceName returns the full Keychain service name for a vault.
func ServiceName(vaultName string) string {
	return ServicePrefix + vaultName
}

// Settings returns the user configuration, loading it on first use.
func (m *Manager) Settings() (*config.Config, error) {
	if m.settings == nil {
		settings, err := config.Load(m.DataDir)
		if err != nil {
			return nil, err
		}
		m.settings = settings
	}
	return m.settings, nil
}

// History returns the version recorder backed by the same Keychain.
func (m *Manager) History() *history.Recorder {
	if m.recorder == nil {
		m.recorder = &history.Recorder{Store: m.KC, Retention: m.effectiveRetention(0)}
	}
	return m.recorder
}

// effectiveRetention resolves how many versions to keep for one write.
// An explicit override always wins; otherwise the config decides, and
// history.enabled=false resolves to zero (no history at all).
func (m *Manager) effectiveRetention(override int) int {
	if override > 0 {
		return override
	}
	settings, err := m.Settings()
	if err != nil {
		return 0
	}
	if !settings.Bool(config.HistoryEnabled) {
		return 0
	}
	return settings.Int(config.HistoryRetention)
}

// ArchiveRetentionDays is how long a soft-deleted vault stays restorable.
func (m *Manager) ArchiveRetentionDays() int {
	settings, err := m.Settings()
	if err != nil {
		return 0
	}
	return settings.Int(config.ArchiveRetentionDays)
}

// ensureDataDir creates the data directory if needed.
func (m *Manager) ensureDataDir() error {
	return os.MkdirAll(m.DataDir, 0o700)
}

// --- Active vault ---

func (m *Manager) activeVaultPath() string {
	return filepath.Join(m.DataDir, "active_vault")
}

// Switch sets the persisted active vault. The vault must exist.
func (m *Manager) Switch(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if err := m.requireVault(name); err != nil {
		return err
	}
	if err := m.ensureDataDir(); err != nil {
		return err
	}
	return writeFile600(m.activeVaultPath(), name+"\n")
}

// --- Vault metadata (materialized list) ---

func (m *Manager) vaultsPath() string {
	return filepath.Join(m.DataDir, "vaults")
}

// ListVaults returns sorted vault names. Always includes "default".
func (m *Manager) ListVaults() ([]string, error) {
	infos, err := m.ListVaultInfos()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Name)
	}
	return names, nil
}

// Create registers a new vault with no metadata attached.
func (m *Manager) Create(name string) error {
	return m.CreateWithInfo(Info{Name: name})
}

// --- CRUD pass-through using the resolved vault ---

// Get retrieves a secret from the specified vault (or active vault if empty).
func (m *Manager) Get(key, vaultName string) (string, error) {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return "", err
	}
	return m.KC.Get(ServiceName(vn), key)
}

// Set stores a secret in the specified vault (or active vault if empty).
func (m *Manager) Set(key, value, vaultName string) error {
	return m.SetWithProtection(key, value, vaultName, true)
}

// SetWithProtection stores a secret, recording the value it replaces.
func (m *Manager) SetWithProtection(key, value, vaultName string, protected bool) error {
	return m.SetWithOptions(key, value, vaultName, SetOptions{Protected: protected})
}

// SetWithOptions stores a secret with explicit history and protection control.
func (m *Manager) SetWithOptions(key, value, vaultName string, opts SetOptions) error {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return err
	}
	if err := m.enforceProtection(vn, opts.Protected); err != nil {
		return err
	}
	if !opts.SkipHistory {
		if err := m.snapshot(vn, key, value, opts.Retention, m.protectionLookup(vn)); err != nil {
			return err
		}
	}
	return m.KC.SetWithProtection(ServiceName(vn), key, value, opts.Protected)
}

// Delete removes a secret, recording it first so it can be rolled back.
func (m *Manager) Delete(key, vaultName string) error {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return err
	}
	if err := m.snapshot(vn, key, "", 0, m.protectionLookup(vn)); err != nil {
		return err
	}
	return m.KC.Delete(ServiceName(vn), key)
}

// Rollback restores a recorded version as the live value and records the value
// it replaced. seq <= 0 restores the most recent version. It returns the
// sequence that was restored.
func (m *Manager) Rollback(key, vaultName string, seq int) (int, error) {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return 0, err
	}

	versions, err := m.History().Versions(vn, key)
	if err != nil {
		return 0, err
	}
	if len(versions) == 0 {
		return 0, fmt.Errorf("vault: no recorded history for %q in vault %q", key, vn)
	}

	if seq <= 0 {
		seq = versions[0].Seq
	}
	protected := true
	found := false
	for _, version := range versions {
		if version.Seq == seq {
			protected = version.Protected
			found = true
			break
		}
	}
	if !found {
		return 0, fmt.Errorf("%w: %s version %d in vault %q", history.ErrVersionNotFound, key, seq, vn)
	}

	value, err := m.History().Value(vn, key, seq)
	if err != nil {
		return 0, err
	}

	// A vault that demands protection never gets a weaker secret back.
	if info, err := m.VaultInfo(vn); err == nil && info.RequireProtection {
		protected = true
	}
	if err := m.SetWithOptions(key, value, vn, SetOptions{Protected: protected}); err != nil {
		return 0, err
	}
	return seq, nil
}

// ListKeys returns all keys in the specified vault (or active vault if empty).
func (m *Manager) ListKeys(vaultName string) ([]string, error) {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return nil, err
	}
	return m.KC.List(ServiceName(vn))
}

// ReadRawService reads all key/value pairs from an arbitrary Keychain service without vault validation.
func (m *Manager) ReadRawService(service string) (map[string]string, error) {
	if service == "" {
		return nil, ErrInvalidName
	}
	keys, err := m.KC.List(service)
	if err != nil {
		return nil, fmt.Errorf("vault: list service %q: %w", service, err)
	}
	result := make(map[string]string, len(keys))
	for _, k := range keys {
		val, err := m.KC.Get(service, k)
		if err != nil {
			return nil, fmt.Errorf("vault: get %q from service %q: %w", k, service, err)
		}
		result[k] = val
	}
	return result, nil
}

// BulkSet stores multiple key/value pairs into the specified vault (or active vault if empty).
func (m *Manager) BulkSet(entries map[string]string, vaultName string) (int, error) {
	return m.BulkSetWithProtection(entries, vaultName, true)
}

// BulkSetWithProtection stores multiple secrets, recording every value it replaces.
func (m *Manager) BulkSetWithProtection(entries map[string]string, vaultName string, protected bool) (int, error) {
	return m.BulkSetWithOptions(entries, vaultName, SetOptions{Protected: protected})
}

// BulkSetWithOptions stores multiple secrets with explicit history control.
// The whole batch is snapshotted before anything is written, so one Keychain
// listing serves every key instead of one per write.
func (m *Manager) BulkSetWithOptions(entries map[string]string, vaultName string, opts SetOptions) (int, error) {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return 0, err
	}
	if err := m.enforceProtection(vn, opts.Protected); err != nil {
		return 0, err
	}

	if !opts.SkipHistory {
		protectionOf := m.protectionLookup(vn)
		for key, value := range entries {
			if err := m.snapshot(vn, key, value, opts.Retention, protectionOf); err != nil {
				return 0, err
			}
		}
	}

	svc := ServiceName(vn)
	n := 0
	for k, v := range entries {
		if err := m.KC.SetWithProtection(svc, k, v, opts.Protected); err != nil {
			return n, fmt.Errorf("vault: bulk set %q: %w", k, err)
		}
		n++
	}
	return n, nil
}

// GetAllKeys returns all key/value pairs from the specified vault (or active vault if empty).
func (m *Manager) GetAllKeys(vaultName string) (map[string]string, error) {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return nil, err
	}
	svc := ServiceName(vn)
	keys, err := m.KC.List(svc)
	if err != nil {
		return nil, fmt.Errorf("vault: list keys: %w", err)
	}
	result := make(map[string]string, len(keys))
	for _, k := range keys {
		val, err := m.KC.Get(svc, k)
		if err != nil {
			return nil, fmt.Errorf("vault: get %q: %w", k, err)
		}
		result[k] = val
	}
	return result, nil
}

func (m *Manager) ListKeyMetadata(vaultName string) ([]SecretMetadata, error) {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return nil, err
	}
	items, err := m.KC.ListMetadata(ServiceName(vn))
	if err != nil {
		return nil, err
	}
	result := make([]SecretMetadata, 0, len(items))
	for _, item := range items {
		protection := ProtectionUnprotected
		if item.Protected {
			protection = ProtectionProtected
		}
		result = append(result, SecretMetadata{Key: item.Account, Protection: protection, Modified: item.Modified})
	}
	return result, nil
}

func (m *Manager) ProtectAllKeys(vaultName string) (int, error) {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return 0, err
	}
	return m.KC.ProtectAll(ServiceName(vn))
}

// --- Helpers ---

// snapshot records the value currently stored at key before it is replaced by
// newValue. Writing the same value again, or writing a key that does not exist
// yet, records nothing — there is no earlier state worth keeping.
func (m *Manager) snapshot(vaultName, key, newValue string, retention int, protectionOf func(string) bool) error {
	budget := m.effectiveRetention(retention)
	if budget <= 0 {
		return nil
	}

	current, err := m.KC.Get(ServiceName(vaultName), key)
	if err != nil || current == "" || current == newValue {
		return nil
	}
	return m.History().Snapshot(vaultName, key, current, protectionOf(key), budget)
}

// protectionLookup returns a function that reports a key's protection level,
// reading the vault's metadata at most once.
func (m *Manager) protectionLookup(vaultName string) func(string) bool {
	var cache map[string]bool
	return func(key string) bool {
		if cache == nil {
			cache = make(map[string]bool)
			if items, err := m.KC.ListMetadata(ServiceName(vaultName)); err == nil {
				for _, item := range items {
					cache[item.Account] = item.Protected
				}
			}
		}
		if protected, ok := cache[key]; ok {
			return protected
		}
		return true
	}
}

// enforceProtection rejects unprotected writes into a vault that requires Touch ID.
func (m *Manager) enforceProtection(vaultName string, protected bool) error {
	if protected {
		return nil
	}
	info, err := m.VaultInfo(vaultName)
	if err != nil {
		return nil
	}
	if info.RequireProtection {
		return fmt.Errorf("%w: vault %q (drop --no-protect, or run `kc vault unprotect %s`)", ErrProtectionRequired, vaultName, vaultName)
	}
	return nil
}

func (m *Manager) resolveVault(name string) (string, error) {
	if name == "" {
		return m.ActiveVault(), nil
	}
	if err := validateName(name); err != nil {
		return "", err
	}
	if err := m.requireVault(name); err != nil {
		return "", err
	}
	return name, nil
}

func (m *Manager) requireVault(name string) error {
	vaults, err := m.ListVaults()
	if err != nil {
		return err
	}
	for _, vault := range vaults {
		if vault == name {
			return nil
		}
	}
	return ErrNotFound
}

func validateName(name string) error {
	if name == "" {
		return ErrInvalidName
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return ErrInvalidName
		}
	}
	return nil
}

// readFileIfExists returns nil data (and no error) when the file is absent.
func readFileIfExists(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("vault: read %q: %w", path, err)
	}
	return data, nil
}

func writeFile600(path, body string) error {
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("vault: write %q: %w", path, err)
	}
	return nil
}
