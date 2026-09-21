package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/v-gutierrez/kc/internal/output"
)

func newVaultCmd(app *App) *cobra.Command {
	vaultCmd := &cobra.Command{
		Use:   "vault",
		Short: "Manage vaults (service groups)",
		Long:  "Vaults group secrets under a Keychain service prefix (kc:{name}).",
	}

	vaultCmd.AddCommand(
		newVaultListCmd(app),
		newVaultCreateCmd(app),
		newVaultSwitchCmd(app),
		newVaultDeleteCmd(app),
		newVaultRenameCmd(app),
		newVaultCloneCmd(app),
		newVaultInfoCmd(app),
		newVaultDescribeCmd(app),
		newVaultProtectCmd(app, true),
		newVaultProtectCmd(app, false),
		newVaultRestoreCmd(app),
		newVaultPurgeCmd(app),
		newVaultUseCmd(app),
		newVaultUnuseCmd(app),
		newVaultWhichCmd(app),
	)

	return vaultCmd
}

func newVaultListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all vaults",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deleted, _ := cmd.Flags().GetBool("deleted")
			if deleted {
				return runVaultListDeleted(app, cmd)
			}

			vaults, err := app.Vaults.List()
			if err != nil {
				return fmt.Errorf("failed to list vaults: %w", err)
			}
			if len(vaults) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No vaults found.")
				return nil
			}

			active, _ := app.Vaults.Active()
			infos := vaultInfoIndex(app)

			for _, v := range vaults {
				marker := "  "
				if v == active {
					marker = "* "
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s%s\n", marker, v, vaultAnnotation(infos[v]))
			}
			return nil
		},
	}
	cmd.Flags().Bool("deleted", false, "list soft-deleted vaults that can still be restored")
	return cmd
}

func runVaultListDeleted(app *App, cmd *cobra.Command) error {
	admin, err := app.admin()
	if err != nil {
		return err
	}

	archived, err := admin.ListArchived()
	if err != nil {
		return err
	}
	if len(archived) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No deleted vaults awaiting restore.")
		return nil
	}

	retention := admin.ArchiveRetentionDays()
	fmt.Fprintln(cmd.OutOrStdout(), "VAULT\tDELETED\tKEYS\tRESTORABLE UNTIL")
	for _, record := range archived {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d\t%s\n",
			record.Name,
			record.DeletedAt.Local().Format("2006-01-02 15:04"),
			record.Keys,
			restorableUntil(record.DeletedAt, retention))
	}
	return nil
}

func restorableUntil(deletedAt time.Time, retentionDays int) string {
	if retentionDays <= 0 {
		return "no expiry"
	}
	return deletedAt.AddDate(0, 0, retentionDays).Local().Format("2006-01-02")
}

func newVaultCreateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a new vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			description, _ := cmd.Flags().GetString("description")
			tags, _ := cmd.Flags().GetStringSlice("tag")
			requireProtection, _ := cmd.Flags().GetBool("require-protection")

			if err := app.Vaults.Create(name); err != nil {
				return fmt.Errorf("failed to create vault %q: %w", name, err)
			}

			if description != "" || len(tags) > 0 {
				admin, err := app.admin()
				if err != nil {
					return err
				}
				if err := admin.Describe(name, &description, &tags); err != nil {
					return err
				}
			}
			if requireProtection {
				admin, err := app.admin()
				if err != nil {
					return err
				}
				if err := admin.RequireProtection(name, true); err != nil {
					return err
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created vault %q.\n", name)
			return nil
		},
	}
	cmd.Flags().String("description", "", "human-readable purpose of the vault")
	cmd.Flags().StringSlice("tag", nil, "tag the vault (repeatable, e.g. --tag env:prod)")
	cmd.Flags().Bool("require-protection", false, "reject secrets stored without Touch ID protection")
	return cmd
}

func newVaultSwitchCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:               "switch NAME",
		Short:             "Set the active vault",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeVaultArg(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := app.Vaults.Switch(name); err != nil {
				return fmt.Errorf("failed to switch to vault %q: %w", name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Switched to vault %q.\n", name)

			// A directory marker or KC_VAULT outranks the persisted vault; say so
			// instead of letting the next command silently use something else.
			if resolved, source := app.activeVaultContext(); resolved != name &&
				(source == VaultSourceDir || source == VaultSourceEnv) {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: %s still selects vault %q here\n", vaultSourceLabel(source), resolved)
			}
			return nil
		},
	}
}

func newVaultDeleteCmd(app *App) *cobra.Command {
	var force bool
	var purge bool
	cmd := &cobra.Command{
		Use:               "delete NAME",
		Aliases:           []string{"rm"},
		Short:             "Delete a vault (its keys stay restorable)",
		Long:              "Deletes a vault. Its keys are archived first — see `kc vault list --deleted`\nand `kc vault restore` — unless --purge destroys them outright.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeVaultArg(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if purge {
				admin, err := app.admin()
				if err != nil {
					return err
				}
				if err := admin.DeleteWithOptions(name, force, true); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Deleted vault %q permanently.\n", name)
				return nil
			}

			if err := app.Vaults.Delete(name, force); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted vault %q. Restore it with `kc vault restore %s`.\n", name, name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "delete vault even if it contains keys")
	cmd.Flags().BoolVar(&purge, "purge", false, "destroy the keys instead of archiving them")
	return cmd
}

func newVaultRenameCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:               "rename OLD NEW",
		Short:             "Rename a vault, keys and history included",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeVaultArg(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}
			if err := admin.Rename(args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Renamed vault %q to %q.\n", args[0], args[1])
			return nil
		},
	}
}

func newVaultCloneCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:               "clone SOURCE DEST",
		Short:             "Copy every secret of a vault into a new one",
		Long:              "Creates DEST with a copy of every secret in SOURCE, protection levels included.\nHistory is not copied: the clone starts fresh.",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeVaultArg(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}
			count, err := admin.Clone(args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Cloned %d keys from %q into %q.\n", count, args[0], args[1])
			return nil
		},
	}
}

func newVaultInfoCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "info [NAME]",
		Short:             "Show a vault's metadata",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeVaultArg(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}

			name := ""
			if len(args) == 1 {
				name = args[0]
			} else if name, err = app.resolveVault(cmd); err != nil {
				return err
			}

			info, err := admin.Info(name)
			if err != nil {
				return err
			}
			keys, err := app.Store.List(name)
			if err != nil {
				return fmt.Errorf("vault info: list keys of %q: %w", name, err)
			}

			jsonOutput, _ := cmd.Flags().GetBool("json")
			if jsonOutput {
				return output.WriteJSON(cmd.OutOrStdout(), output.VaultInfoResult{
					Name:              info.Name,
					Description:       info.Description,
					Tags:              info.Tags,
					Created:           info.Created,
					RequireProtection: info.RequireProtection,
					Keys:              len(keys),
					Service:           "kc:" + info.Name,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Vault:\t%s\n", info.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Keys:\t%d\n", len(keys))
			fmt.Fprintf(cmd.OutOrStdout(), "Service:\tkc:%s\n", info.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Description:\t%s\n", orDash(info.Description))
			fmt.Fprintf(cmd.OutOrStdout(), "Tags:\t%s\n", orDash(strings.Join(info.Tags, ", ")))
			fmt.Fprintf(cmd.OutOrStdout(), "Created:\t%s\n", orDash(info.Created))
			fmt.Fprintf(cmd.OutOrStdout(), "Protection:\t%s\n", protectionPolicyLabel(info.RequireProtection))
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output structured JSON")
	return cmd
}

func newVaultDescribeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "describe NAME",
		Short:             "Set a vault's description or tags",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeVaultArg(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}

			var description *string
			var tags *[]string
			if cmd.Flags().Changed("description") {
				value, _ := cmd.Flags().GetString("description")
				description = &value
			}
			if cmd.Flags().Changed("tag") {
				value, _ := cmd.Flags().GetStringSlice("tag")
				tags = &value
			}
			if description == nil && tags == nil {
				return fmt.Errorf("describe: pass --description and/or --tag")
			}

			if err := admin.Describe(args[0], description, tags); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated vault %q.\n", args[0])
			return nil
		},
	}
	cmd.Flags().String("description", "", "human-readable purpose of the vault")
	cmd.Flags().StringSlice("tag", nil, "replace the vault's tags (repeatable)")
	return cmd
}

// newVaultProtectCmd builds `kc vault protect` and `kc vault unprotect`, which
// toggle whether a vault refuses secrets stored without Touch ID.
func newVaultProtectCmd(app *App, require bool) *cobra.Command {
	use, short := "unprotect NAME", "Allow secrets without Touch ID in a vault"
	if require {
		use, short = "protect NAME", "Require Touch ID protection for every secret in a vault"
	}

	return &cobra.Command{
		Use:   use,
		Short: short,
		Long: "Sets the vault's write policy. A protected vault rejects `kc set --no-protect`\n" +
			"outright, so production secrets cannot lose Touch ID by accident.\n" +
			"To protect secrets that already exist, use `kc protect --all`.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeVaultArg(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}
			if err := admin.RequireProtection(args[0], require); err != nil {
				return err
			}
			if require {
				fmt.Fprintf(cmd.OutOrStdout(), "Vault %q now requires Touch ID protected secrets.\n", args[0])
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Vault %q no longer requires Touch ID protected secrets.\n", args[0])
			}
			return nil
		},
	}
}

func newVaultRestoreCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "restore NAME",
		Short: "Restore a soft-deleted vault",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeArchivedVaults(app, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}
			count, err := admin.Restore(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Restored vault %q with %d keys.\n", args[0], count)
			return nil
		},
	}
}

func newVaultPurgeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purge [NAME]",
		Short: "Destroy a soft-deleted vault for good",
		Long:  "Removes an archived vault permanently, history included. With --expired it\npurges every archived vault past the retention window instead.",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeArchivedVaults(app, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}

			expired, _ := cmd.Flags().GetBool("expired")
			if expired {
				if len(args) > 0 {
					return fmt.Errorf("purge: --expired takes no vault name")
				}
				names, err := admin.GCArchived(admin.ArchiveRetentionDays())
				if err != nil {
					return err
				}
				if len(names) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Nothing past the retention window.")
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Purged %d expired vaults: %s\n", len(names), strings.Join(names, ", "))
				return nil
			}

			if len(args) != 1 {
				return fmt.Errorf("purge: name a vault, or pass --expired")
			}
			count, err := admin.PurgeArchived(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Purged archived vault %q (%d keys).\n", args[0], count)
			return nil
		},
	}
	cmd.Flags().Bool("expired", false, "purge every archived vault past the retention window")
	return cmd
}

func newVaultUseCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "use NAME",
		Short:             "Pin this directory to a vault",
		Long:              "Writes a .kc-vault marker so kc uses NAME for every command run in this\ndirectory and below — no shell hook, resolved by kc on each invocation.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeVaultArg(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}
			dir, _ := cmd.Flags().GetString("dir")
			if err := admin.UseDir(args[0], dir); err != nil {
				return err
			}
			where := dir
			if where == "" {
				where = "this directory"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Pinned %s to vault %q.\n", where, args[0])
			return nil
		},
	}
	cmd.Flags().String("dir", "", "directory to pin (default: working directory)")
	return cmd
}

func newVaultUnuseCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unuse",
		Short: "Remove this directory's vault pin",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}
			dir, _ := cmd.Flags().GetString("dir")
			if err := admin.ClearDir(dir); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Removed the vault pin.")
			return nil
		},
	}
	cmd.Flags().String("dir", "", "directory to unpin (default: working directory)")
	return cmd
}

func newVaultWhichCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "which",
		Short: "Explain which vault kc will use here, and why",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, source := app.activeVaultContext()
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t(%s)\n", name, vaultSourceLabel(source))
			if !app.vaultKnown(name) {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: vault %q does not exist\n", name)
			}
			return nil
		},
	}
}

func completeVaultArg(app *App) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeVaults(app, toComplete)
	}
}

func completeArchivedVaults(app *App, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 || app.Admin == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	archived, err := app.Admin.ListArchived()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, 0, len(archived))
	for _, record := range archived {
		if strings.HasPrefix(record.Name, toComplete) {
			names = append(names, record.Name)
		}
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

func vaultInfoIndex(app *App) map[string]VaultInfo {
	index := make(map[string]VaultInfo)
	if app.Admin == nil {
		return index
	}
	infos, err := app.Admin.Infos()
	if err != nil {
		return index
	}
	for _, info := range infos {
		index[info.Name] = info
	}
	return index
}

// vaultAnnotation renders the short suffix `kc vault list` shows after a name.
func vaultAnnotation(info VaultInfo) string {
	parts := make([]string, 0, 3)
	if info.RequireProtection {
		parts = append(parts, "protected")
	}
	if len(info.Tags) > 0 {
		parts = append(parts, strings.Join(info.Tags, ","))
	}
	if info.Description != "" {
		parts = append(parts, info.Description)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  — " + strings.Join(parts, " · ")
}

func protectionPolicyLabel(require bool) string {
	if require {
		return "Touch ID required for every secret"
	}
	return "per-secret (default: protected)"
}
