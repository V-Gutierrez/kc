package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newProtectCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "protect",
		Short: "Protect existing secrets with Touch ID",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			if !all {
				return fmt.Errorf("protect: --all flag is required")
			}
			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}
			count, err := app.Store.ProtectAll(vault)
			if err != nil {
				return fmt.Errorf("protect: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Protected %d keys in vault %q.\n", count, vault)
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "protect all secrets in the target vault")
	return cmd
}
