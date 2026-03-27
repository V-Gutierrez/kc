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
		}
	}
	return out, nil
}

func launchInteractive(app *App, initialFilter string) error {
	return runInteractive(interactiveDeps{
		Store:         tuiStoreAdapter{app.Store},
		Vaults:        app.Vaults,
		Clipboard:     app.Clipboard,
		InitialFilter: initialFilter,
	})
}
