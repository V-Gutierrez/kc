package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDelCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "del KEY",
		Aliases: []string{"delete", "rm"},
		Short:   "Delete a secret from the keychain",
		Long:    "Removes KEY from the active vault (or --vault).",
		Args:    cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completeKeys(app, cmd, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}
			key := args[0]

			if err := app.Store.Delete(vault, key); err != nil {
				return fmt.Errorf("failed to delete %q from vault %q: %w", key, vault, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted %q from vault %q.\n", key, vault)
			return nil
		},
	}
}
