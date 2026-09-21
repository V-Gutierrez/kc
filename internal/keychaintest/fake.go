// Package keychaintest provides an in-memory Keychain backend for tests.
//
// It mirrors the behaviour the real macOS backend gives kc: items are keyed by
// (service, account), updates overwrite in place, and every write stamps a
// modification date — which is exactly what makes history worth having.
package keychaintest

import (
	"sort"
	"sync"
	"time"

	"github.com/v-gutierrez/kc/internal/keychain"
)

type item struct {
	value     string
	protected bool
	modified  time.Time
}

// Fake is an in-memory stand-in for the macOS Keychain.
type Fake struct {
	mu    sync.Mutex
	items map[string]map[string]item
	// Now lets a test control the clock stamped on writes.
	Now func() time.Time
}

// New returns an empty Fake.
func New() *Fake {
	return &Fake{items: make(map[string]map[string]item)}
}

func (f *Fake) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

// Get returns the stored value, or keychain.ErrNotFound.
func (f *Fake) Get(service, account string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	entry, ok := f.items[service][account]
	if !ok {
		return "", keychain.ErrNotFound
	}
	return entry.value, nil
}

// Set stores a protected item.
func (f *Fake) Set(service, account, password string) error {
	return f.SetWithProtection(service, account, password, true)
}

// SetWithProtection stores an item, overwriting any previous value.
func (f *Fake) SetWithProtection(service, account, password string, protected bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.items[service] == nil {
		f.items[service] = make(map[string]item)
	}
	f.items[service][account] = item{value: password, protected: protected, modified: f.now()}
	return nil
}

// Delete removes an item, or reports keychain.ErrNotFound.
func (f *Fake) Delete(service, account string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.items[service][account]; !ok {
		return keychain.ErrNotFound
	}
	delete(f.items[service], account)
	return nil
}

// List returns the account names of a service, sorted.
func (f *Fake) List(service string) ([]string, error) {
	items, err := f.ListMetadata(service)
	if err != nil {
		return nil, err
	}
	accounts := make([]string, 0, len(items))
	for _, entry := range items {
		accounts = append(accounts, entry.Account)
	}
	return accounts, nil
}

// ListMetadata returns every item of a service, sorted by account.
func (f *Fake) ListMetadata(service string) ([]keychain.ItemMetadata, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	entries := f.items[service]
	items := make([]keychain.ItemMetadata, 0, len(entries))
	for account, entry := range entries {
		items = append(items, keychain.ItemMetadata{
			Account:   account,
			Protected: entry.protected,
			Modified:  entry.modified.Format("2006-01-02 15:04"),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Account < items[j].Account })
	return items, nil
}

// ProtectAll marks every unprotected item of a service as protected.
func (f *Fake) ProtectAll(service string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	count := 0
	for account, entry := range f.items[service] {
		if entry.protected {
			continue
		}
		entry.protected = true
		f.items[service][account] = entry
		count++
	}
	return count, nil
}

// Services returns every service that holds at least one item, sorted.
func (f *Fake) Services() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	services := make([]string, 0, len(f.items))
	for service, entries := range f.items {
		if len(entries) > 0 {
			services = append(services, service)
		}
	}
	sort.Strings(services)
	return services
}

// SetModified backdates an item, so rotation and retention rules can be tested.
func (f *Fake) SetModified(service, account string, when time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	entry, ok := f.items[service][account]
	if !ok {
		return
	}
	entry.modified = when
	f.items[service][account] = entry
}
