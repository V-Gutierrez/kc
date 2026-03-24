package cli

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
	"github.com/v-gutierrez/kc/internal/output"
)

func newSearchCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search QUERY",
		Short: "Fuzzy search keys across vaults",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vaults, err := resolveAuditVaults(app, cmd)
			if err != nil {
				return err
			}
			jsonOutput, _ := cmd.Flags().GetBool("json")
			showValues, _ := cmd.Flags().GetBool("show-values")
			query := strings.ToLower(strings.TrimSpace(args[0]))
			results := make([]output.ListItem, 0)

			for _, vault := range vaults {
				metadata, err := app.Store.ListMetadata(vault)
				if err != nil {
					return fmt.Errorf("search: read metadata for vault %q: %w", vault, err)
				}
				searchTargets := make([]string, len(metadata))
				for i, item := range metadata {
					searchTargets[i] = strings.ToLower(item.Key)
				}
				matches := fuzzy.Find(query, searchTargets)
				values := map[string]string{}
				if showValues && len(matches) > 0 {
					requiresAuth := false
					for _, match := range matches {
						if metadata[match.Index].Protection == ProtectionProtected {
							requiresAuth = true
							break
						}
					}
					if requiresAuth {
						session := authSession(app)
						if err := session.Authorize("Unlock kc secrets"); err != nil {
							return err
						}
					}
					values, err = app.Bulk.GetAll(vault)
					if err != nil {
						return fmt.Errorf("search: read values for vault %q: %w", vault, err)
					}
				}
				for _, match := range matches {
					item := metadata[match.Index]
					result := output.ListItem{Key: item.Key, Vault: vault, Protection: item.Protection}
					if showValues {
						result.Value = values[item.Key]
					}
					results = append(results, result)
				}
			}

			if jsonOutput {
				return output.WriteJSON(cmd.OutOrStdout(), results)
			}
			if len(results) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No keys matching %q.\n", args[0])
				return nil
			}
			for _, item := range results {
				if showValues {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", item.Key, item.Vault, item.Protection, item.Value)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", item.Key, item.Vault, item.Protection)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output structured JSON")
	cmd.Flags().Bool("show-values", false, "include secret values in results")
	return cmd
}
