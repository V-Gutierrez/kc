package cli

import (
	"strconv"

	"github.com/v-gutierrez/kc/internal/tui"
)

type interactiveDeps = tui.Deps

var runInteractive = func(deps interactiveDeps) error {
	return tui.Run(deps)
}

type tuiStoreAdapter struct {
	KeychainStore
}

func (a tuiStoreAdapter) ListMetadata(vault string) ([]tui.SecretMetadata, error) {
	metas, err := a.KeychainStore.ListMetadata(vault)
	if err != nil {
		return nil, err
	}
	out := make([]tui.SecretMetadata, len(metas))
	for i, m := range metas {
		out[i] = tui.SecretMetadata{
			Key:        m.Key,
			Vault:      m.Vault,
			Protection: m.Protection,
			Modified:   m.Modified,
		}
	}
	return out, nil
}

// tuiHistoryAdapter bridges the CLI history port to the TUI's. It exists mainly
// to normalise the argument order: HistoryStore.Rollback takes (key, vault) and
// the TUI, like every other call it makes, takes (vault, key).
type tuiHistoryAdapter struct {
	store HistoryStore
}

func (a tuiHistoryAdapter) Versions(vault, key string) ([]tui.Version, error) {
	versions, err := a.store.Versions(vault, key)
	if err != nil {
		return nil, err
	}
	out := make([]tui.Version, len(versions))
	for i, v := range versions {
		out[i] = tui.Version{
			Seq:       v.Seq,
			Recorded:  v.Recorded,
			Protected: v.Protected,
			Digest:    v.Digest,
		}
	}
	return out, nil
}

func (a tuiHistoryAdapter) Value(vault, key string, seq int) (string, error) {
	return a.store.Value(vault, key, seq)
}

func (a tuiHistoryAdapter) Rollback(vault, key string, seq int) error {
	_, err := a.store.Rollback(key, vault, seq)
	return err
}

func launchInteractive(app *App, initialFilter string) error {
	deps := interactiveDeps{
		Store:         tuiStoreAdapter{app.Store},
		Vaults:        app.Vaults,
		Clipboard:     app.Clipboard,
		InitialFilter: initialFilter,
	}
	// History is optional: without it the TUI simply has no history view,
	// rather than a nil dereference behind the h key.
	if app.History != nil {
		deps.History = tuiHistoryAdapter{store: app.History}
	}
	deps.PinnedVault = pinnedVault(app)
	deps.RotationDays = configuredRotationDays(app)
	return runInteractive(deps)
}

// pinnedVault names the vault a .kc-vault marker fixes for this directory, and
// only that: an active vault chosen by `kc vault switch` is not a pin, and
// saying so would be a claim about the filesystem that is not true.
func pinnedVault(app *App) string {
	if app.Admin == nil {
		return ""
	}
	name, source, err := app.Admin.Context()
	if err != nil || source != VaultSourceDir {
		return ""
	}
	return name

}

// configuredRotationDays reads the same setting `kc audit` uses, so a key shown
// as stale in the TUI is a key `kc audit` would flag.
func configuredRotationDays(app *App) int {
	if app.Config == nil {
		return 0
	}
	days, err := strconv.Atoi(app.Config.Get("audit.rotation_days"))
	if err != nil {
		return 0
	}
	return days
}
