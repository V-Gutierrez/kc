package cli

import "github.com/v-gutierrez/kc/internal/tui"

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
	return runInteractive(deps)
}
