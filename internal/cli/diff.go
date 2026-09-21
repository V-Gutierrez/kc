package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	internaldiff "github.com/v-gutierrez/kc/internal/diff"
	"github.com/v-gutierrez/kc/internal/keychain"
)

func newDiffCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [ENV_FILE | KEY --version N]",
		Short: "Compare env files, vaults, or two recorded versions of one secret",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("diff: accepts at most one .env file argument")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			vaults, _ := cmd.Flags().GetStringArray("vault")

			if versions, _ := cmd.Flags().GetIntSlice("version"); len(versions) > 0 {
				if len(args) != 1 {
					return fmt.Errorf("diff: --version needs a key, e.g. `kc diff API_KEY --version 2`")
				}
				return runVersionDiff(app, cmd, args[0], versions)
			}

			var leftName string
			var leftEntries map[string]string
			var rightName string
			var rightEntries map[string]string

			switch {
			case len(args) == 1:
				leftName = args[0]
				file, err := os.Open(args[0])
				if err != nil {
					return fmt.Errorf("diff: cannot open %q: %w", args[0], err)
				}
				defer file.Close()
				leftEntries = parseEnvReader(file)

				targetVault, err := resolveDiffTargetVault(app, vaults)
				if err != nil {
					return err
				}
				rightName = targetVault
				rightEntries, err = app.Bulk.GetAll(targetVault)
				if err != nil {
					return fmt.Errorf("diff: read vault %q: %w", targetVault, err)
				}
			case len(args) == 0 && len(vaults) == 2:
				leftName = vaults[0]
				rightName = vaults[1]
				var err error
				leftEntries, err = app.Bulk.GetAll(leftName)
				if err != nil {
					return fmt.Errorf("diff: read vault %q: %w", leftName, err)
				}
				rightEntries, err = app.Bulk.GetAll(rightName)
				if err != nil {
					return fmt.Errorf("diff: read vault %q: %w", rightName, err)
				}
			default:
				return fmt.Errorf("diff: use `kc diff .env [--vault target]` or `kc diff --vault source --vault target`")
			}

			entries := internaldiff.Compare(leftEntries, rightEntries)
			for _, entry := range entries {
				switch entry.Status {
				case internaldiff.Changed:
					fmt.Fprintf(cmd.OutOrStdout(), "~ %s %s -> %s\n", entry.Key, maskValue(entry.Left), maskValue(entry.Right))
				case internaldiff.Added:
					fmt.Fprintf(cmd.OutOrStdout(), "+ %s\n", entry.Key)
				case internaldiff.Removed:
					fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", entry.Key)
				case internaldiff.Equal:
					fmt.Fprintf(cmd.OutOrStdout(), "= %s\n", entry.Key)
				}
			}

			if len(entries) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No differences between %s and %s.\n", leftName, rightName)
			}
			return nil
		},
	}
	cmd.Flags().StringArray("vault", nil, "vault(s) to compare; repeat twice for vault-to-vault diff")
	cmd.Flags().IntSlice("version", nil, "recorded version(s) of a key to compare; one compares against the live value")
	return cmd
}

// runVersionDiff compares two points in a single secret's life. It prints
// digests rather than values — whether something changed is the question, and
// the answer does not require showing the secret.
func runVersionDiff(app *App, cmd *cobra.Command, key string, versions []int) error {
	if len(versions) > 2 {
		return fmt.Errorf("diff: pass at most two --version flags")
	}
	store, err := app.history()
	if err != nil {
		return err
	}
	vault, err := app.resolveVault(cmd)
	if err != nil {
		return err
	}

	leftLabel, leftValue, err := versionValue(app, store, vault, key, versions[0])
	if err != nil {
		return err
	}

	rightLabel, rightValue := "live", ""
	if len(versions) == 2 {
		rightLabel, rightValue, err = versionValue(app, store, vault, key, versions[1])
	} else {
		rightValue, err = app.Store.Get(vault, key)
		if err != nil {
			return fmt.Errorf("diff: read live %q in vault %q: %w", key, vault, err)
		}
	}
	if err != nil {
		return err
	}

	status := "="
	if leftValue != rightValue {
		status = "~"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s  %s (%s) -> %s (%s)\n",
		status, key, leftLabel, shortDigest(keychain.Digest(leftValue)), rightLabel, shortDigest(keychain.Digest(rightValue)))
	if status == "=" {
		fmt.Fprintf(cmd.OutOrStdout(), "No difference between %s and %s.\n", leftLabel, rightLabel)
	}
	return nil
}

func versionValue(app *App, store HistoryStore, vault, key string, version int) (string, string, error) {
	value, err := store.Value(vault, key, version)
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("v%d", version), value, nil
}

func resolveDiffTargetVault(app *App, vaults []string) (string, error) {
	if len(vaults) > 1 {
		return "", fmt.Errorf("diff: use a single --vault with an env file, or two --vault flags without a file")
	}
	if len(vaults) == 1 && strings.TrimSpace(vaults[0]) != "" {
		if err := ensureVaultExists(app, vaults[0]); err != nil {
			return "", err
		}
		return vaults[0], nil
	}
	return activeOrDefaultVault(app)
}

func activeOrDefaultVault(app *App) (string, error) {
	active, err := app.Vaults.Active()
	if err == nil && strings.TrimSpace(active) != "" {
		return active, nil
	}
	return DefaultVault, nil
}

func ensureVaultExists(app *App, name string) error {
	vaults, err := app.Vaults.List()
	if err != nil {
		return err
	}
	for _, vault := range vaults {
		if vault == name {
			return nil
		}
	}
	return fmt.Errorf("vault %q not found", name)
}
