package cli

import "github.com/v-gutierrez/kc/internal/tui"

type interactiveDeps = tui.Deps

var runInteractive = func(deps interactiveDeps) error {
	return tui.Run(deps)
}

func launchInteractive(app *App, initialFilter string) error {
	return runInteractive(interactiveDeps{
		Store:         app.Store,
		Vaults:        app.Vaults,
		Clipboard:     app.Clipboard,
		InitialFilter: initialFilter,
	})
}
