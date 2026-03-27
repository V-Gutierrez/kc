package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInjectCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inject --key KEY",
		Short: "Print a single secret value to stdout (no trailing newline)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, _ := cmd.Flags().GetString("key")
			if key == "" {
				return fmt.Errorf("--key is required")
			}

			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}

			metadata, err := app.Store.ListMetadata(vault)
			if err != nil {
				return fmt.Errorf("inject: %w", err)
			}
			if isProtected(metadata, key) {
				session := authSession(app)
				if err := session.Authorize("Unlock kc secret"); err != nil {
					return err
				}
			}

			value, err := app.Store.Get(vault, key)
			if err != nil {
				return fmt.Errorf("inject: failed to get %q from vault %q: %w", key, vault, err)
			}

			fmt.Fprint(cmd.OutOrStdout(), value)
			return nil
		},
	}
	cmd.Flags().String("key", "", "secret key to retrieve")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}
