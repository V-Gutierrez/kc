package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/v-gutierrez/kc/internal/output"
)

func newHistoryCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history KEY",
		Short: "List the recorded previous values of a secret",
		Long: "Lists every version kc recorded before a value was overwritten or deleted.\n" +
			"Values are shown as digests only — read one with `kc get KEY --version N`,\n" +
			"restore one with `kc rollback KEY --version N`.",
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completeKeys(app, cmd, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := app.history()
			if err != nil {
				return err
			}
			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}

			key := args[0]
			versions, err := store.Versions(vault, key)
			if err != nil {
				return fmt.Errorf("history: read %q in vault %q: %w", key, vault, err)
			}

			jsonOutput, _ := cmd.Flags().GetBool("json")
			if jsonOutput {
				return output.WriteJSON(cmd.OutOrStdout(), output.HistoryItems(toOutputVersions(versions)))
			}

			if len(versions) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No recorded versions for %q in vault %q.\n", key, vault)
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), "VERSION\tRECORDED\tPROTECTION\tDIGEST")
			for _, version := range versions {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\n",
					version.Seq, orDash(version.Recorded), protectionLabel(version.Protected), shortDigest(version.Digest))
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output structured JSON")
	return cmd
}

func newRollbackCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback KEY",
		Short: "Restore a previously recorded value of a secret",
		Long: "Restores a recorded version as the live value. The value being replaced is\n" +
			"itself recorded first, so a rollback is never destructive and can be undone.\n" +
			"Without --version the most recent recorded value is restored.",
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completeKeys(app, cmd, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := app.history()
			if err != nil {
				return err
			}
			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}

			key := args[0]
			version, _ := cmd.Flags().GetInt("version")

			versions, err := store.Versions(vault, key)
			if err != nil {
				return fmt.Errorf("rollback: read history of %q: %w", key, err)
			}
			if len(versions) == 0 {
				return fmt.Errorf("rollback: no recorded history for %q in vault %q", key, vault)
			}
			if anyVersionProtected(versions) {
				session := authSession(app)
				if err := session.Authorize("Restore kc secret"); err != nil {
					return err
				}
			}

			restored, err := store.Rollback(key, vault, version)
			if err != nil {
				return fmt.Errorf("rollback: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Restored %q in vault %q from version %d.\n", key, vault, restored)
			return nil
		},
	}
	cmd.Flags().Int("version", 0, "version to restore (default: the most recent)")
	return cmd
}

func toOutputVersions(versions []HistoryVersion) []output.HistoryItem {
	items := make([]output.HistoryItem, 0, len(versions))
	for _, version := range versions {
		items = append(items, output.HistoryItem{
			Version:   version.Seq,
			Key:       version.Key,
			Vault:     version.Vault,
			Recorded:  version.Recorded,
			Protected: version.Protected,
			Digest:    version.Digest,
		})
	}
	return items
}

func anyVersionProtected(versions []HistoryVersion) bool {
	for _, version := range versions {
		if version.Protected {
			return true
		}
	}
	return false
}

func protectionLabel(protected bool) string {
	if protected {
		return ProtectionProtected
	}
	return ProtectionUnprotected
}

func shortDigest(digest string) string {
	if len(digest) <= 12 {
		return orDash(digest)
	}
	return digest[:12]
}

func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
