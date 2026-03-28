package cli

import "fmt"

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
