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

			if err := app.Store.SetWithProtection(vault, key, value, !noProtect); err != nil {
				return fmt.Errorf("failed to set %q in vault %q: %w", key, vault, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Stored %q in vault %q.\n", key, vault)
			return nil
		},
	}
	cmd.Flags().Bool("no-protect", false, "store the secret without Touch ID protection")
	return cmd
}
