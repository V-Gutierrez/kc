package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// App holds injected dependencies for all CLI commands.
type App struct {
	Store     KeychainStore
	Bulk      BulkStore
	Vaults    VaultManager
	Clipboard Clipboard
}

// resolveVault returns the vault from --vault flag, or falls back to active vault,
// or falls back to DefaultVault.
func (a *App) resolveVault(cmd *cobra.Command) (string, error) {
	v, _ := cmd.Flags().GetString("vault")
	if v != "" {
		vaults, err := a.Vaults.List()
		if err != nil {
			return "", err
		}
		for _, vault := range vaults {
			if vault == v {
				return v, nil
			}
		}
		return "", fmt.Errorf("vault %q not found", v)
	}
	active, err := a.Vaults.Active()
	if err != nil {
		return DefaultVault, nil
	}
	if active == "" {
		return DefaultVault, nil
	}

	return active, nil
}

// NewRootCmd builds the root cobra.Command with all subcommands wired.
func NewRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:     "kc",
		Short:   "A human-friendly CLI for macOS Keychain",
		Long:    "kc replaces the macOS security command with an intuitive CLI for managing secrets stored in the native Keychain.",
		Version: Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			interactive, _ := cmd.Flags().GetBool("interactive")
			initialFilter, _ := cmd.Flags().GetString("vault")
			if interactive || len(args) == 0 {
				return launchInteractive(app, initialFilter)
			}
			return cmd.Help()
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().String("vault", "", "target vault (overrides active vault)")
	root.Flags().BoolP("interactive", "i", false, "launch interactive TUI")

	root.AddCommand(
		newGetCmd(app),
		newSetCmd(app),
		newDelCmd(app),
		newListCmd(app),
		newInitCmd(app),
		newSetupCmd(app),
		newVaultCmd(app),
		newImportCmd(app),
		newExportCmd(app),
		newEnvCmd(app),
		newMigrateCmd(app),
	)

	return root
}
