package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/v-gutierrez/kc/internal/auth"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// App holds injected dependencies for all CLI commands.
type App struct {
	Store     KeychainStore
	Bulk      BulkStore
	Vaults    VaultManager
	Clipboard Clipboard
	Auth      auth.Authorizer
	Runner    CommandRunner

	// Admin, History and Config are wired by cmd/kc. Commands that need them
	// fail with a clear message rather than a nil dereference when they are not.
	Admin   VaultAdmin
	History HistoryStore
	Config  Settings
}

func (a *App) admin() (VaultAdmin, error) {
	if a.Admin == nil {
		return nil, fmt.Errorf("vault administration is unavailable in this build")
	}
	return a.Admin, nil
}

func (a *App) history() (HistoryStore, error) {
	if a.History == nil {
		return nil, fmt.Errorf("secret history is unavailable in this build")
	}
	return a.History, nil
}

func (a *App) settings() (Settings, error) {
	if a.Config == nil {
		return nil, fmt.Errorf("configuration is unavailable in this build")
	}
	return a.Config, nil
}

// activeVaultContext resolves the vault kc acts on and where that came from.
func (a *App) activeVaultContext() (string, string) {
	if a.Admin != nil {
		if name, source, err := a.Admin.Context(); err == nil && name != "" {
			return name, source
		}
	}
	active, err := a.Vaults.Active()
	if err != nil || active == "" {
		return DefaultVault, VaultSourceDefault
	}
	return active, VaultSourceFile
}

func (a *App) vaultKnown(name string) bool {
	vaults, err := a.Vaults.List()
	if err != nil {
		return true // cannot tell; do not cry wolf
	}
	for _, vault := range vaults {
		if vault == name {
			return true
		}
	}
	return false
}

// vaultSourceLabel explains where an active vault came from.
func vaultSourceLabel(source string) string {
	switch source {
	case VaultSourceEnv:
		return "KC_VAULT"
	case VaultSourceDir:
		return ".kc-vault marker"
	case VaultSourceFile:
		return "active vault"
	default:
		return "default"
	}
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
	name, source := a.activeVaultContext()
	if (source == VaultSourceDir || source == VaultSourceEnv) && !a.vaultKnown(name) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s selects vault %q, which does not exist\n", vaultSourceLabel(source), name)
	}
	return name, nil
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
	if err := root.RegisterFlagCompletionFunc("vault", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeVaults(app, toComplete)
	}); err != nil {
		panic(err)
	}
	root.Flags().BoolP("interactive", "i", false, "launch interactive TUI")

	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newAuditCmd(app),
		newConfigCmd(app),
		newHistoryCmd(app),
		newRollbackCmd(app),
		newMoveCmd(app),
		newCopyCmd(app),
		newDiffCmd(app),
		newGetCmd(app),
		newLoadCmd(app),
		newSetCmd(app),
		newDelCmd(app),
		newListCmd(app),
		newSearchCmd(app),
		newInitCmd(app),
		newHookCmd(app),
		newSetupCmd(app),
		newVaultCmd(app),
		newImportCmd(app),
		newExportCmd(app),
		newEnvCmd(app),
		newProtectCmd(app),
		newMigrateCmd(app),
		newCompletionCmd(),
		newRunCmd(app),
		newInjectCmd(app),
		newResolveCmd(app),
	)

	return root
}
