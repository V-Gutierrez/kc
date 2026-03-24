package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/v-gutierrez/kc/internal/output"
)

func newGetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get KEY",
		Short: "Read a secret from the keychain",
		Long:  "Retrieves the value for KEY from the active vault (or --vault) and copies it to the clipboard.",
		Args:  cobra.ExactArgs(1),
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
			metadata, err := app.Store.ListMetadata(vault)
			if err != nil {
				return fmt.Errorf("failed to inspect %q in vault %q: %w", key, vault, err)
			}
			if isProtected(metadata, key) {
				session := authSession(app)
				if err := session.Authorize("Unlock kc secret"); err != nil {
					return err
				}
			}

			value, err := app.Store.Get(vault, key)
			if err != nil {
				return fmt.Errorf("failed to get %q from vault %q: %w", key, vault, err)
			}

			jsonOutput, _ := cmd.Flags().GetBool("json")
			if jsonOutput {
				return output.WriteJSON(cmd.OutOrStdout(), output.GetResult(key, value, vault))
			}

			// Copy to clipboard if available.
			if app.Clipboard != nil {
				if copyErr := app.Clipboard.Copy(value); copyErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: clipboard copy failed: %v\n", copyErr)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "Copied to clipboard.\n")
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), maskValue(value))
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output structured JSON")
	return cmd
}

func maskValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "[empty]"
	}
	return strings.Repeat("*", 8)
}
