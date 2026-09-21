package cli

import (
	"fmt"
	"time"
)

type SecretMetadata struct {
	Key        string
	Vault      string
	Protection string
	Modified   string
}

const (
	ProtectionUnknown     = "unknown"
	ProtectionProtected   = "protected"
	ProtectionUnprotected = "unprotected"
)

// KeychainStore abstracts CRUD operations against the macOS Keychain.
// The vault parameter corresponds to the Keychain "service" field (prefixed "kc:{vault}").
// The key parameter corresponds to the Keychain "account" field.
type KeychainStore interface {
	Get(vault, key string) (string, error)
	Set(vault, key, value string) error
	SetWithProtection(vault, key, value string, protected bool) error
	Delete(vault, key string) error
	List(vault string) ([]string, error)
	ListMetadata(vault string) ([]SecretMetadata, error)
	ProtectAll(vault string) (int, error)
}

// VaultManager handles vault lifecycle: listing, creating, switching the active vault.
// Active vault is persisted across invocations (e.g. in ~/.kc/config).
type VaultManager interface {
	List() ([]string, error)
	Create(name string) error
	Delete(name string, force bool) error
	Active() (string, error)
	Switch(name string) error
}

// Clipboard abstracts clipboard write + optional auto-clear.
type Clipboard interface {
	Copy(value string) error
}

// BulkStore extends KeychainStore with bulk operations needed for import/export/migrate.
type BulkStore interface {
	KeychainStore
	BulkSet(entries map[string]string, vault string) (int, error)
	BulkSetWithProtection(entries map[string]string, vault string, protected bool) (int, error)
	GetAll(vault string) (map[string]string, error)
	ReadRawService(service string) (map[string]string, error)
}

// DefaultVault is the fallback when no --vault flag and no active vault override.
const DefaultVault = "default"

// ExitError wraps a non-zero child exit code so main can propagate it
// without printing a duplicate error message.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}

// CommandRunner executes a command with the given environment.
// It returns the exit code of the child process plus any system-level error.
type CommandRunner func(name string, args []string, env []string) (exitCode int, err error)

// VaultInfo carries a vault's metadata for display and editing.
type VaultInfo struct {
	Name              string
	Description       string
	Tags              []string
	Created           string
	RequireProtection bool
}

// ArchivedVault is a soft-deleted vault that can still be restored.
type ArchivedVault struct {
	Name      string
	DeletedAt time.Time
	Keys      int
	Info      VaultInfo
}

// HistoryVersion is one recorded previous value of a key. The value itself is
// never carried here — only its digest, so listings hold no plaintext.
type HistoryVersion struct {
	Seq       int
	Key       string
	Vault     string
	Recorded  string
	Protected bool
	Digest    string
}

// VaultAdmin covers vault lifecycle beyond create/switch/delete: renaming,
// cloning, metadata, the archive of deleted vaults, directory pinning, and
// moving a single key between vaults.
type VaultAdmin interface {
	Rename(oldName, newName string) error
	Clone(src, dst string) (int, error)
	Infos() ([]VaultInfo, error)
	Info(name string) (VaultInfo, error)
	Describe(name string, description *string, tags *[]string) error
	RequireProtection(name string, require bool) error
	DeleteWithOptions(name string, force, purge bool) error
	ListArchived() ([]ArchivedVault, error)
	Restore(name string) (int, error)
	PurgeArchived(name string) (int, error)
	GCArchived(days int) ([]string, error)
	ArchiveRetentionDays() int
	Context() (name string, source string, err error)
	UseDir(name, dir string) error
	ClearDir(dir string) error
	MoveKey(key, from, to, newKey string, copyOnly, force bool) error
}

// HistoryStore exposes the recorded versions of a secret.
type HistoryStore interface {
	Versions(vault, key string) ([]HistoryVersion, error)
	Value(vault, key string, seq int) (string, error)
	Keys(vault string) ([]string, error)
	Rollback(key, vault string, seq int) (int, error)
	PurgeKey(vault, key string) (int, error)
}

// OptionStore writes a secret with explicit history control. Stores that do not
// implement it simply lose the --no-history / --keep-versions flags.
type OptionStore interface {
	SetWithOptions(vault, key, value string, protected, skipHistory bool, retention int) error
}

// Settings is the user configuration file behind `kc config`.
type Settings interface {
	Get(key string) string
	All() map[string]string
	Set(key, value string) error
	Unset(key string) error
	Path() string
	Known(key string) bool
	Keys() []string
	Describe(key string) string
}

// Sources of the resolved active vault, mirrored from the vault package.
const (
	VaultSourceEnv     = "env"
	VaultSourceDir     = "dir"
	VaultSourceFile    = "file"
	VaultSourceDefault = "default"
)
