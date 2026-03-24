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
			protectedOnly, _ := cmd.Flags().GetBool("protected")

			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}

			metadata, err := app.Store.ListMetadata(vault)
			if err != nil {
				return fmt.Errorf("failed to list key metadata in vault %q: %w", vault, err)
			}
			metadataByKey := make(map[string]SecretMetadata, len(metadata))
			for _, item := range metadata {
				metadataByKey[item.Key] = item
			}

			if showValues {
				if app.Bulk == nil {
					return fmt.Errorf("list: --show-values requires bulk store support")
				}

				session := authSession(app)
				entries, err := app.Bulk.GetAll(vault)
				if err != nil {
					return fmt.Errorf("failed to list keys in vault %q: %w", vault, err)
				}
				filtered := make(map[string]string)
				for key, value := range entries {
					item := metadataByKey[key]
					if protectedOnly && item.Protection != ProtectionProtected {
						continue
					}
					if item.Protection == ProtectionProtected {
						if err := session.Authorize("Unlock kc secrets"); err != nil {
							return err
						}
					}
					filtered[key] = value
				}

				if jsonOutput {
					items := make([]output.ListItem, 0, len(filtered))
					for _, key := range sortedKeys(filtered) {
						items = append(items, output.ListItem{Key: key, Vault: vault, Value: filtered[key], Protection: metadataByKey[key].Protection})
					}
					return output.WriteJSON(cmd.OutOrStdout(), items)
				}

				if len(filtered) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "No keys in vault %q.\n", vault)
					return nil
				}

				for _, key := range sortedKeys(filtered) {
					if protectedOnly {
						fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", key, filtered[key], metadataByKey[key].Protection)
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, filtered[key])
				}
				return nil
			}

			keys, err := app.Store.List(vault)
			if err != nil {
				return fmt.Errorf("failed to list keys in vault %q: %w", vault, err)
			}
			sort.Strings(keys)
			visibleMetadata := make([]SecretMetadata, 0, len(keys))
			for _, key := range keys {
				item, ok := metadataByKey[key]
				if !ok {
					item = SecretMetadata{Key: key, Vault: vault, Protection: ProtectionUnknown}
				}
				if protectedOnly && item.Protection != ProtectionProtected {
					continue
				}
				visibleMetadata = append(visibleMetadata, item)
			}

			if jsonOutput {
				items := make([]output.ListItem, 0, len(visibleMetadata))
				for _, item := range visibleMetadata {
					items = append(items, output.ListItem{Key: item.Key, Vault: item.Vault, Protection: item.Protection})
				}
				return output.WriteJSON(cmd.OutOrStdout(), items)
			}

			if len(visibleMetadata) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No keys in vault %q.\n", vault)
				return nil
			}

			for _, item := range visibleMetadata {
				if protectedOnly {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", item.Key, item.Protection)
					continue
				}
				fmt.Fprintln(cmd.OutOrStdout(), item.Key)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output structured JSON")
	cmd.Flags().Bool("show-values", false, "include secret values in list output")
	cmd.Flags().Bool("protected", false, "show only Touch ID protected keys and their status")
	return cmd
}
