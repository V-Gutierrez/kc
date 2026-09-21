package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set KEY VALUE",
		Short: "Store or update a secret in the keychain",
		Long:  "Stores VALUE under KEY in the active vault (or --vault). Creates or overwrites.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}
			key, value := args[0], args[1]
			noProtect, _ := cmd.Flags().GetBool("no-protect")
			noHistory, _ := cmd.Flags().GetBool("no-history")
			keepVersions, _ := cmd.Flags().GetInt("keep-versions")

			if err := setSecret(app, vault, key, value, !noProtect, noHistory, keepVersions); err != nil {
				return fmt.Errorf("failed to set %q in vault %q: %w", key, vault, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Stored %q in vault %q.\n", key, vault)
			return nil
		},
	}
	cmd.Flags().Bool("no-protect", false, "store the secret without Touch ID protection")
	cmd.Flags().Bool("no-history", false, "overwrite without recording the previous value")
	cmd.Flags().Int("keep-versions", 0, "previous versions to keep for this key (default: config)")
	return cmd
}

// setSecret writes through the option-aware store when one is wired, so
// --no-history and --keep-versions mean something; otherwise it falls back to a
// plain protected write.
func setSecret(app *App, vault, key, value string, protected, skipHistory bool, retention int) error {
	if store, ok := app.Store.(OptionStore); ok {
		return store.SetWithOptions(vault, key, value, protected, skipHistory, retention)
	}
	return app.Store.SetWithProtection(vault, key, value, protected)
}
