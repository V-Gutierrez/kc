package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Bookkeeping variables the cd-hook uses to know what it exported last time.
// They are the only state the hook keeps: a shell that loses them simply
// reloads, it never gets out of sync with a file on disk.
const (
	loadedVaultVar = "KC_LOADED_VAULT"
	loadedKeysVar  = "KC_LOADED_KEYS"
)

func newEnvCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Print shell export statements for all secrets in the active vault",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}
			sync, _ := cmd.Flags().GetBool("sync")

			// In sync mode the vault resolved for this directory is often the
			// one already loaded. Returning early keeps a per-prompt hook free:
			// no Keychain read, and no Touch ID prompt on every cd.
			if sync && os.Getenv(loadedVaultVar) == vault {
				return nil
			}

			// Listing names needs no authentication, so the stale exports of
			// the vault being left can be dropped before anything can fail.
			metadata, err := app.Store.ListMetadata(vault)
			if err != nil {
				return fmt.Errorf("env: %w", err)
			}
			if sync {
				for _, stale := range staleKeys(os.Getenv(loadedKeysVar), metadata) {
					fmt.Fprintf(cmd.OutOrStdout(), "unset %s\n", stale)
				}
			}

			if !shouldSkipAuth(cmd) {
				requiresAuth := false
				for _, item := range metadata {
					if item.Protection == ProtectionProtected {
						requiresAuth = true
						break
					}
				}
				if requiresAuth {
					session := authSession(app)
					if err := session.Authorize("Unlock kc secrets"); err != nil {
						// A declined prompt on a directory change is an answer,
						// not a crash: the stale keys are already unset, so the
						// shell is left clean rather than holding the previous
						// vault. The new vault is still recorded so the hook
						// does not re-prompt on every command.
						if sync {
							fmt.Fprintf(cmd.ErrOrStderr(), "kc: %s not loaded (%v) — run `kc load` to unlock\n", vault, err)
							fmt.Fprintf(cmd.OutOrStdout(), "export %s=%s\n", loadedVaultVar, shellQuote(vault))
							fmt.Fprintf(cmd.OutOrStdout(), "export %s=%s\n", loadedKeysVar, shellQuote(""))
							return nil
						}
						return err
					}
				}
			}

			entries, err := app.Bulk.GetAll(vault)
			if err != nil {
				return fmt.Errorf("env: %w", err)
			}

			keys := sortedKeys(entries)
			for _, k := range keys {
				fmt.Fprintf(cmd.OutOrStdout(), "export %s=%s\n", k, shellQuote(entries[k]))
			}
			if sync {
				fmt.Fprintf(cmd.OutOrStdout(), "export %s=%s\n", loadedVaultVar, shellQuote(vault))
				fmt.Fprintf(cmd.OutOrStdout(), "export %s=%s\n", loadedKeysVar, shellQuote(strings.Join(keys, " ")))
			}
			return nil
		},
	}
	cmd.Flags().Bool("no-touch-id", false, "skip Touch ID authentication for protected keys")
	cmd.Flags().Bool("sync", false, "emit the difference against the vault already loaded (used by kc hook)")
	return cmd
}

// staleKeys returns the previously exported names that the new vault does not
// define, sorted so the output is deterministic. It works off metadata (names
// only) so it never needs the values, and therefore never needs Touch ID.
func staleKeys(loaded string, next []SecretMetadata) []string {
	defined := make(map[string]struct{}, len(next))
	for _, item := range next {
		defined[item.Key] = struct{}{}
	}
	stale := make([]string, 0)
	for _, name := range strings.Fields(loaded) {
		if _, still := defined[name]; !still {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	return stale
}
