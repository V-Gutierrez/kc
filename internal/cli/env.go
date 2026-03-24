package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newEnvCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Print shell export statements for all secrets in the active vault",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}
			metadata, err := app.Store.ListMetadata(vault)
			if err != nil {
				return fmt.Errorf("env: %w", err)
			}
			protectedKeys := make(map[string]bool, len(metadata))
			for _, item := range metadata {
				protectedKeys[item.Key] = item.Protection == ProtectionProtected
			}
			entries, err := app.Bulk.GetAll(vault)
			if err != nil {
				return fmt.Errorf("env: %w", err)
			}
			session := authSession(app)
			authorized := false
			for _, k := range sortedKeys(entries) {
				if protectedKeys[k] && !authorized {
					if err := session.Authorize("Unlock kc secrets"); err != nil {
						return err
					}
					authorized = true
				}
				fmt.Fprintf(cmd.OutOrStdout(), "export %s=%s\n", k, shellQuote(entries[k]))
			}
			return nil
		},
	}
}
