package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/v-gutierrez/kc/internal/output"
)

func newListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all keys in a vault",
		Long:    "Lists all key names stored in the active vault (or --vault). Values are not shown.",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, _ := cmd.Flags().GetBool("json")
			showValues, _ := cmd.Flags().GetBool("show-values")

			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}

			if showValues {
				if app.Bulk == nil {
					return fmt.Errorf("list: --show-values requires bulk store support")
				}

				entries, err := app.Bulk.GetAll(vault)
				if err != nil {
					return fmt.Errorf("failed to list keys in vault %q: %w", vault, err)
				}

				if jsonOutput {
					return output.WriteJSON(cmd.OutOrStdout(), output.ListItemsWithValues(entries, vault))
				}

				if len(entries) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "No keys in vault %q.\n", vault)
					return nil
				}

				for _, key := range sortedKeys(entries) {
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, entries[key])
				}
				return nil
			}

			keys, err := app.Store.List(vault)
			if err != nil {
				return fmt.Errorf("failed to list keys in vault %q: %w", vault, err)
			}
			sort.Strings(keys)

			if jsonOutput {
				return output.WriteJSON(cmd.OutOrStdout(), output.ListItems(keys, vault))
			}

			if len(keys) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No keys in vault %q.\n", vault)
				return nil
			}

			for _, k := range keys {
				fmt.Fprintln(cmd.OutOrStdout(), k)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output structured JSON")
	cmd.Flags().Bool("show-values", false, "include secret values in list output")
	return cmd
}
