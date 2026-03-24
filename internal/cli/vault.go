package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVaultCmd(app *App) *cobra.Command {
	vaultCmd := &cobra.Command{
		Use:   "vault",
		Short: "Manage vaults (service groups)",
		Long:  "Vaults group secrets under a Keychain service prefix (kc:{name}).",
	}

	vaultCmd.AddCommand(
		newVaultListCmd(app),
		newVaultCreateCmd(app),
		newVaultSwitchCmd(app),
		newVaultDeleteCmd(app),
	)

	return vaultCmd
}

func newVaultListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all vaults",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			vaults, err := app.Vaults.List()
			if err != nil {
				return fmt.Errorf("failed to list vaults: %w", err)
			}

			active, _ := app.Vaults.Active()

			if len(vaults) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No vaults found.")
				return nil
			}

			for _, v := range vaults {
				marker := "  "
				if v == active {
					marker = "* "
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", marker, v)
			}
			return nil
		},
	}
}

func newVaultCreateCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "create NAME",
		Short: "Create a new vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := app.Vaults.Create(name); err != nil {
				return fmt.Errorf("failed to create vault %q: %w", name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created vault %q.\n", name)
			return nil
		},
	}
}

func newVaultSwitchCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "switch NAME",
		Short: "Set the active vault",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completeVaults(app, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := app.Vaults.Switch(name); err != nil {
				return fmt.Errorf("failed to switch to vault %q: %w", name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Switched to vault %q.\n", name)
			return nil
		},
	}
}

func newVaultDeleteCmd(app *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "delete NAME",
		Aliases: []string{"rm"},
		Short:   "Delete a vault",
		Args:    cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completeVaults(app, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := app.Vaults.Delete(name, force); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted vault %q.\n", name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "delete vault even if it contains keys")
	return cmd
}
