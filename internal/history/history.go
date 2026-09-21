// Package history keeps previous values of a secret so a write is reversible.
//
// macOS Keychain has no native item history: `SecItemUpdate` overwrites in
// place and the old password is gone. kc therefore records versions itself,
// reusing the very same Keychain primitives the live secrets use — no extra
// crypto, no extra dependency, no plaintext on disk.
//
//	live secret   → service "kc:{vault}"              account "{key}"
//	history entry → service "kc:{vault}:__history__"   account "{key}~{seq}"
//
// The sequence is per key and monotonic; the newest versions win when the
// retention budget prunes. Timestamps come from the Keychain item's own
// modification date, so nothing hand-rolls a clock.
package history

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/v-gutierrez/kc/internal/keychain"
)

// HistorySuffix is appended to a vault's Keychain service to hold its history.
const HistorySuffix = ":__history__"

// separator splits "{key}~{seq}" in a history account name.
const separator = "~"

// ErrVersionNotFound is returned when the requested sequence does not exist.
var ErrVersionNotFound = errors.New("history: version not found")

// Store is the Keychain subset the recorder needs.
type Store interface {
	Get(service, account string) (string, error)
	SetWithProtection(service, account, password string, protected bool) error
	Delete(service, account string) error
	ListMetadata(service string) ([]keychain.ItemMetadata, error)
}

// Version describes one recorded value of a key. The value itself is not
// carried — only its digest, so listings never hold plaintext.
type Version struct {
	Seq       int
	Key       string
	Vault     string
	Recorded  string
	Protected bool
	Digest    string
}

// Recorder appends and prunes versions for a vault's keys.
type Recorder struct {
	Store     Store
	Retention int
}

// ServiceName returns the Keychain service holding a vault's history.
func ServiceName(vault string) string {
	return "kc:" + vault + HistorySuffix
}

// AccountName returns the history account name for a key/sequence pair.
func AccountName(key string, seq int) string {
	return fmt.Sprintf("%s%s%05d", key, separator, seq)
}

// ParseAccount splits a history account back into key and sequence.
// It splits on the last separator so keys containing "~" survive the round trip.
func ParseAccount(account string) (key string, seq int, ok bool) {
	idx := strings.LastIndex(account, separator)
	if idx <= 0 || idx == len(account)-1 {
		return "", 0, false
	}
	seq, err := strconv.Atoi(account[idx+1:])
	if err != nil || seq <= 0 {
		return "", 0, false
	}
	return account[:idx], seq, true
}

// Snapshot records value as the newest version of key, then prunes older
// versions beyond the retention budget. retention <= 0 uses the recorder's
// default; a resolved budget of zero disables history entirely. An empty value
// is never recorded — there is nothing to roll back to.
func (r *Recorder) Snapshot(vault, key, value string, protected bool, retention int) error {
	budget := r.budget(retention)
	if budget <= 0 || value == "" {
		return nil
	}

	versions, err := r.Versions(vault, key)
	if err != nil {
		return err
	}

	next := 1
	if len(versions) > 0 {
		next = versions[0].Seq + 1
	}

	service := ServiceName(vault)
	if err := r.Store.SetWithProtection(service, AccountName(key, next), value, protected); err != nil {
		return fmt.Errorf("history: record %q version %d: %w", key, next, err)
	}

	// versions is newest-first and excludes the write above, so everything from
	// index budget-1 onwards falls outside the budget once the new one lands.
	for i := budget - 1; i < len(versions); i++ {
		if err := r.Store.Delete(service, AccountName(key, versions[i].Seq)); err != nil {
			return fmt.Errorf("history: prune %q version %d: %w", key, versions[i].Seq, err)
		}
	}
	return nil
}

// Versions returns every recorded version of key, newest first.
func (r *Recorder) Versions(vault, key string) ([]Version, error) {
	service := ServiceName(vault)
	items, err := r.Store.ListMetadata(service)
	if err != nil {
		return nil, fmt.Errorf("history: list %q: %w", service, err)
	}

	versions := make([]Version, 0, len(items))
	for _, item := range items {
		itemKey, seq, ok := ParseAccount(item.Account)
		if !ok || itemKey != key {
			continue
		}
		version := Version{
			Seq:       seq,
			Key:       key,
			Vault:     vault,
			Recorded:  item.Modified,
			Protected: item.Protected,
		}
		if value, err := r.Store.Get(service, item.Account); err == nil {
			version.Digest = keychain.Digest(value)
		}
		versions = append(versions, version)
	}

	sort.Slice(versions, func(i, j int) bool { return versions[i].Seq > versions[j].Seq })
	return versions, nil
}

// Latest returns the most recent version of key, if any.
func (r *Recorder) Latest(vault, key string) (Version, bool, error) {
	versions, err := r.Versions(vault, key)
	if err != nil {
		return Version{}, false, err
	}
	if len(versions) == 0 {
		return Version{}, false, nil
	}
	return versions[0], true, nil
}

// Value reads the stored value of a specific version.
func (r *Recorder) Value(vault, key string, seq int) (string, error) {
	value, err := r.Store.Get(ServiceName(vault), AccountName(key, seq))
	if err != nil {
		if errors.Is(err, keychain.ErrNotFound) {
			return "", fmt.Errorf("%w: %s version %d in vault %q", ErrVersionNotFound, key, seq, vault)
		}
		return "", fmt.Errorf("history: read %q version %d: %w", key, seq, err)
	}
	return value, nil
}

// Keys lists the distinct keys that have at least one recorded version.
func (r *Recorder) Keys(vault string) ([]string, error) {
	items, err := r.Store.ListMetadata(ServiceName(vault))
	if err != nil {
		return nil, fmt.Errorf("history: list vault %q: %w", vault, err)
	}

	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if key, _, ok := ParseAccount(item.Account); ok {
			seen[key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// PurgeKey removes every recorded version of a single key.
func (r *Recorder) PurgeKey(vault, key string) (int, error) {
	versions, err := r.Versions(vault, key)
	if err != nil {
		return 0, err
	}

	service := ServiceName(vault)
	removed := 0
	for _, version := range versions {
		if err := r.Store.Delete(service, AccountName(key, version.Seq)); err != nil {
			return removed, fmt.Errorf("history: purge %q version %d: %w", key, version.Seq, err)
		}
		removed++
	}
	return removed, nil
}

// PurgeVault removes every recorded version in a vault.
func (r *Recorder) PurgeVault(vault string) (int, error) {
	service := ServiceName(vault)
	items, err := r.Store.ListMetadata(service)
	if err != nil {
		return 0, fmt.Errorf("history: list vault %q: %w", vault, err)
	}

	removed := 0
	for _, item := range items {
		if _, _, ok := ParseAccount(item.Account); !ok {
			continue
		}
		if err := r.Store.Delete(service, item.Account); err != nil {
			return removed, fmt.Errorf("history: purge %q: %w", item.Account, err)
		}
		removed++
	}
	return removed, nil
}

// RenameVault moves every recorded version to another vault's history service.
func (r *Recorder) RenameVault(oldVault, newVault string) error {
	source := ServiceName(oldVault)
	target := ServiceName(newVault)
	items, err := r.Store.ListMetadata(source)
	if err != nil {
		return fmt.Errorf("history: list vault %q: %w", oldVault, err)
	}

	for _, item := range items {
		if _, _, ok := ParseAccount(item.Account); !ok {
			continue
		}
		value, err := r.Store.Get(source, item.Account)
		if err != nil {
			return fmt.Errorf("history: read %q: %w", item.Account, err)
		}
		if err := r.Store.SetWithProtection(target, item.Account, value, item.Protected); err != nil {
			return fmt.Errorf("history: move %q: %w", item.Account, err)
		}
		if err := r.Store.Delete(source, item.Account); err != nil {
			return fmt.Errorf("history: drop %q: %w", item.Account, err)
		}
	}
	return nil
}

// RenameKey moves a key's recorded versions to a new key name within the same
// vault, appending them to whatever history the destination already has.
func (r *Recorder) RenameKey(vault, oldKey, newKey string) error {
	return r.MoveKey(vault, oldKey, vault, newKey)
}

// MoveKey moves a key's recorded versions to another key, possibly in another
// vault, appending them (oldest first) to the destination's existing history so
// sequences never collide.
func (r *Recorder) MoveKey(srcVault, srcKey, dstVault, dstKey string) error {
	if srcVault == dstVault && srcKey == dstKey {
		return nil
	}

	versions, err := r.Versions(srcVault, srcKey)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return nil
	}

	target, err := r.Versions(dstVault, dstKey)
	if err != nil {
		return err
	}
	next := 1
	if len(target) > 0 {
		next = target[0].Seq + 1
	}

	source := ServiceName(srcVault)
	destination := ServiceName(dstVault)
	// Oldest first so the destination keeps chronological order.
	for i := len(versions) - 1; i >= 0; i-- {
		version := versions[i]
		value, err := r.Store.Get(source, AccountName(srcKey, version.Seq))
		if err != nil {
			return fmt.Errorf("history: read %q version %d: %w", srcKey, version.Seq, err)
		}
		if err := r.Store.SetWithProtection(destination, AccountName(dstKey, next), value, version.Protected); err != nil {
			return fmt.Errorf("history: move %q version %d: %w", srcKey, version.Seq, err)
		}
		if err := r.Store.Delete(source, AccountName(srcKey, version.Seq)); err != nil {
			return fmt.Errorf("history: drop %q version %d: %w", srcKey, version.Seq, err)
		}
		next++
	}
	return nil
}

func (r *Recorder) budget(retention int) int {
	if retention > 0 {
		return retention
	}
	return r.Retention
}
