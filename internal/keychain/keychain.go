// Package keychain wraps the macOS `security` CLI to manage Keychain items.
// It uses exec.Command (no CGo) so the binary stays pure Go.
package keychain

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Common errors returned by Keychain operations.
var (
	ErrNotFound = errors.New("keychain: item not found")
	ErrDup      = errors.New("keychain: item already exists")
)

// CommandRunner abstracts command execution so tests can inject fakes.
type CommandRunner interface {
	// Run executes a command and returns combined stdout, stderr, and error.
	Run(name string, args ...string) ([]byte, error)
}

// ExecRunner is the real implementation that calls os/exec.
type ExecRunner struct{}

// Run executes the given command via exec.Command.
func (ExecRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// Keychain provides CRUD operations against the macOS Keychain.
type Keychain struct {
	Runner CommandRunner
}

type ItemMetadata struct {
	Account   string
	Protected bool
}

const protectedComment = "kc-meta:v1:protected"

// New returns a Keychain using the real exec runner.
func New() *Keychain {
	return &Keychain{Runner: ExecRunner{}}
}

// Get retrieves the password for (service, account).
func (k *Keychain) Get(service, account string) (string, error) {
	out, err := k.Runner.Run("security", "find-generic-password",
		"-s", service,
		"-a", account,
		"-w",
	)
	if err != nil {
		if strings.Contains(string(out), "could not be found") ||
			strings.Contains(string(out), "SecKeychainSearchCopyNext") {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("keychain get: %w: %s", err, string(out))
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// Set stores or updates the password for (service, account).
func (k *Keychain) Set(service, account, password string) error {
	return k.SetWithProtection(service, account, password, true)
}

func (k *Keychain) SetWithProtection(service, account, password string, protected bool) error {
	comment := ""
	if protected {
		comment = protectedComment
	}

	out, err := k.Runner.Run("security", "add-generic-password",
		"-s", service,
		"-a", account,
		"-w", password,
		"-j", comment,
		"-U", // update if duplicate (belt-and-suspenders)
	)
	if err != nil {
		return fmt.Errorf("keychain set: %w: %s", err, string(out))
	}
	return nil
}

func (k *Keychain) ListMetadata(service string) ([]ItemMetadata, error) {
	dumpOut, dumpErr := k.Runner.Run("security", "dump-keychain")
	if dumpErr != nil {
		return nil, fmt.Errorf("keychain list metadata: %w: %s", dumpErr, string(dumpOut))
	}

	items := parseMetadata(string(dumpOut), service)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Account < items[j].Account
	})
	return items, nil
}

func (k *Keychain) Protection(service, account string) (bool, error) {
	items, err := k.ListMetadata(service)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Account == account {
			return item.Protected, nil
		}
	}
	return false, ErrNotFound
}

func (k *Keychain) ProtectAll(service string) (int, error) {
	items, err := k.ListMetadata(service)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, item := range items {
		if item.Protected {
			continue
		}

		value, err := k.Get(service, item.Account)
		if err != nil {
			return count, err
		}
		if err := k.SetWithProtection(service, item.Account, value, true); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// Delete removes the item for (service, account).
func (k *Keychain) Delete(service, account string) error {
	out, err := k.Runner.Run("security", "delete-generic-password",
		"-s", service,
		"-a", account,
	)
	if err != nil {
		if strings.Contains(string(out), "could not be found") ||
			strings.Contains(string(out), "SecKeychainSearchCopyNext") {
			return ErrNotFound
		}
		return fmt.Errorf("keychain delete: %w: %s", err, string(out))
	}
	return nil
}

// List returns account names for all generic-password items matching service.
// It parses the output of `security dump-keychain` filtered by service.
func (k *Keychain) List(service string) ([]string, error) {
	items, err := k.ListMetadata(service)
	if err != nil {
		return nil, err
	}
	accounts := make([]string, 0, len(items))
	for _, item := range items {
		accounts = append(accounts, item.Account)
	}
	return accounts, nil
}

// parseAccounts extracts "acct" values from dump-keychain output
// for entries whose "svce" (service) matches the target.
//
// This parser is intentionally conservative because `security dump-keychain`
// output is a loosely structured text format rather than a stable machine API.
// Tests use fixture samples to document the exact shapes we currently accept,
// including NULL comments and mixed service blocks.
func parseAccounts(dump, service string) []string {
	items := parseMetadata(dump, service)
	var accounts []string
	for _, item := range items {
		accounts = append(accounts, item.Account)
	}
	return accounts
}

func parseMetadata(dump, service string) []ItemMetadata {
	items := make([]ItemMetadata, 0)
	for _, block := range strings.Split(dump, "class:") {
		itemService, itemAccount, comment := parseBlock(block)
		if itemService != service || itemAccount == "" {
			continue
		}
		items = append(items, ItemMetadata{Account: itemAccount, Protected: isProtectedComment(comment)})
	}
	return items
}

func parseBlock(block string) (string, string, string) {
	var service string
	var account string
	var comment string

	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.Contains(trimmed, `"svce"`):
			service = extractQuotedValue(trimmed)
		case strings.Contains(trimmed, `"acct"`):
			account = extractQuotedValue(trimmed)
		case strings.Contains(trimmed, `"icmt"`):
			comment = extractQuotedValue(trimmed)
		}
	}

	return service, account, comment
}

func isProtectedComment(comment string) bool {
	return strings.TrimSpace(comment) == protectedComment
}

func Digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum)
}

// extractQuotedValue pulls the last ="..." value from a dump-keychain attribute line.
func extractQuotedValue(line string) string {
	idx := strings.LastIndex(line, `="`)
	if idx < 0 {
		return ""
	}
	rest := line[idx+2:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}
