// Package vault manages named vaults backed by macOS Keychain services.
//
// Each vault maps to a Keychain service with the prefix "kc:" (e.g. "kc:default").
// The active vault is persisted to ~/.kc/active_vault.
// Vault metadata (the list of known vaults) is materialized in ~/.kc/vaults.
package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	ErrNotFound      = errors.New("vault: not found")
	ErrAlreadyExists = errors.New("vault: already exists")
	ErrInvalidName   = errors.New("vault: invalid name (must be non-empty alphanumeric/dash/underscore)")
	ErrDefaultVault  = errors.New("vault: cannot delete the default vault")
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

// Manager handles vault lifecycle.
type Manager struct {
	KC      KeychainBackend
	DataDir string // defaults to ~/.kc
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

// ensureDataDir creates the data directory if needed.
func (m *Manager) ensureDataDir() error {
	return os.MkdirAll(m.DataDir, 0o700)
}

// --- Active vault ---

func (m *Manager) activeVaultPath() string {
	return filepath.Join(m.DataDir, "active_vault")
}

// ActiveVault returns the currently active vault name.
// Returns DefaultVault if no vault has been explicitly set.
func (m *Manager) ActiveVault() string {
	data, err := os.ReadFile(m.activeVaultPath())
	if err != nil {
		return DefaultVault
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return DefaultVault
	}
	return name
}

// Switch sets the active vault. The vault must exist (be registered).
func (m *Manager) Switch(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	vaults, err := m.ListVaults()
	if err != nil {
		return err
	}
	found := false
	for _, v := range vaults {
		if v == name {
			found = true
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	if err := m.ensureDataDir(); err != nil {
		return err
	}
	return os.WriteFile(m.activeVaultPath(), []byte(name+"\n"), 0o600)
}

// --- Vault metadata (materialized list) ---

func (m *Manager) vaultsPath() string {
	return filepath.Join(m.DataDir, "vaults")
}

// ListVaults returns sorted vault names. Always includes "default".
func (m *Manager) ListVaults() ([]string, error) {
	if err := m.ensureDataDir(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(m.vaultsPath())
	if err != nil {
		// File doesn't exist yet — seed with default.
		if errors.Is(err, os.ErrNotExist) {
			if wErr := m.writeVaults([]string{DefaultVault}); wErr != nil {
				return nil, wErr
			}
			return []string{DefaultVault}, nil
		}
		return nil, err
	}

	vaults := parseVaultList(string(data))
	if len(vaults) == 0 {
		vaults = []string{DefaultVault}
		_ = m.writeVaults(vaults)
	}
	return vaults, nil
}

// Create registers a new vault.
func (m *Manager) Create(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	vaults, err := m.ListVaults()
	if err != nil {
		return err
	}
	for _, v := range vaults {
		if v == name {
			return ErrAlreadyExists
		}
	}
	vaults = append(vaults, name)
	sort.Strings(vaults)
	return m.writeVaults(vaults)
}

// --- CRUD pass-through using active vault ---

// DeleteVault removes a vault and optionally force-deletes its keys.
func (m *Manager) DeleteVault(name string, force bool) error {
	if err := validateName(name); err != nil {
		return err
	}
	if name == DefaultVault {
		return ErrDefaultVault
	}
	if err := m.requireVault(name); err != nil {
		return err
	}

	svc := ServiceName(name)
	keys, err := m.KC.List(svc)
	if err != nil {
		return fmt.Errorf("vault: list keys for %q: %w", name, err)
	}

	if len(keys) > 0 && !force {
		return fmt.Errorf("vault has %d keys: delete them first or use --force", len(keys))
	}

	for _, k := range keys {
		if err := m.KC.Delete(svc, k); err != nil {
			return fmt.Errorf("vault: delete key %q from %q: %w", k, name, err)
		}
	}

	vaults, err := m.ListVaults()
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(vaults)-1)
	for _, v := range vaults {
		if v != name {
			filtered = append(filtered, v)
		}
	}
	if err := m.writeVaults(filtered); err != nil {
		return err
	}

	if m.ActiveVault() == name {
		return m.Switch(DefaultVault)
	}
	return nil
}

// --- Helpers ---

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

func (m *Manager) SetWithProtection(key, value, vaultName string, protected bool) error {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return err
	}
	return m.KC.SetWithProtection(ServiceName(vn), key, value, protected)
}

// Delete removes a secret from the specified vault (or active vault if empty).
func (m *Manager) Delete(key, vaultName string) error {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return err
	}
	return m.KC.Delete(ServiceName(vn), key)
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

func (m *Manager) BulkSetWithProtection(entries map[string]string, vaultName string, protected bool) (int, error) {
	vn, err := m.resolveVault(vaultName)
	if err != nil {
		return 0, err
	}
	svc := ServiceName(vn)
	n := 0
	for k, v := range entries {
		if err := m.KC.SetWithProtection(svc, k, v, protected); err != nil {
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

func (m *Manager) writeVaults(names []string) error {
	if err := m.ensureDataDir(); err != nil {
		return err
	}
	var buf strings.Builder
	for _, n := range names {
		buf.WriteString(n)
		buf.WriteByte('\n')
	}
	return os.WriteFile(m.vaultsPath(), []byte(buf.String()), 0o600)
}

func parseVaultList(data string) []string {
	var vaults []string
	for _, line := range strings.Split(data, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			vaults = append(vaults, name)
		}
	}
	sort.Strings(vaults)
	return vaults
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
