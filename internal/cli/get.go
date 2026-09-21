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
			if version, _ := cmd.Flags().GetInt("version"); version > 0 {
				return runGetVersion(app, cmd, vault, key, version)
			}

			if !shouldSkipAuth(cmd) {
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
	cmd.Flags().Bool("no-touch-id", false, "skip Touch ID authentication for protected keys")
	cmd.Flags().Int("version", 0, "read a recorded previous value instead of the live one")
	return cmd
}

// runGetVersion reads one recorded version. It is gated by the same Touch ID
// check as the live value: a previous secret is still a secret.
func runGetVersion(app *App, cmd *cobra.Command, vault, key string, version int) error {
	store, err := app.history()
	if err != nil {
		return err
	}

	versions, err := store.Versions(vault, key)
	if err != nil {
		return fmt.Errorf("failed to read history of %q in vault %q: %w", key, vault, err)
	}
	found := false
	for _, candidate := range versions {
		if candidate.Seq != version {
			continue
		}
		found = true
		if candidate.Protected {
			session := authSession(app)
			if err := session.Authorize("Unlock kc secret"); err != nil {
				return err
			}
		}
		break
	}
	if !found {
		return fmt.Errorf("no version %d of %q in vault %q (see `kc history %s`)", version, key, vault, key)
	}

	value, err := store.Value(vault, key, version)
	if err != nil {
		return err
	}

	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		return output.WriteJSON(cmd.OutOrStdout(), output.GetVersionResult(key, value, vault, version))
	}

	if app.Clipboard != nil {
		if copyErr := app.Clipboard.Copy(value); copyErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: clipboard copy failed: %v\n", copyErr)
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "Copied version %d to clipboard.\n", version)
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), maskValue(value))
	return nil
}

func maskValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "[empty]"
	}
	return strings.Repeat("*", 8)
}
