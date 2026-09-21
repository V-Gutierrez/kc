package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMoveCmd(app *App) *cobra.Command {
	return newTransferCmd(app, false)
}

func newCopyCmd(app *App) *cobra.Command {
	return newTransferCmd(app, true)
}

// newTransferCmd builds `kc mv` and `kc cp`, which differ only in whether the
// source key survives.
func newTransferCmd(app *App, copyOnly bool) *cobra.Command {
	use, short := "mv KEY", "Move a secret to another vault or name"
	if copyOnly {
		use, short = "cp KEY", "Copy a secret to another vault or name"
	}

	cmd := &cobra.Command{
		Use:     use,
		Aliases: transferAliases(copyOnly),
		Short:   short,
		Long: "Transfers one secret between vaults, or renames it in place with --as.\n" +
			"The secret's Touch ID protection travels with it; on a move so does its\n" +
			"recorded history. An existing destination key is never overwritten silently.",
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completeKeys(app, cmd, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			admin, err := app.admin()
			if err != nil {
				return err
			}

			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			newKey, _ := cmd.Flags().GetString("as")
			force, _ := cmd.Flags().GetBool("force")

			if from == "" {
				if from, err = app.resolveVault(cmd); err != nil {
					return err
				}
			}
			if to == "" {
				to = from
			}
			if to == from && newKey == "" {
				return fmt.Errorf("nothing to do: pass --to for another vault or --as for a new name")
			}

			key := args[0]
			if err := admin.MoveKey(key, from, to, newKey, copyOnly, force); err != nil {
				return err
			}

			destination := newKey
			if destination == "" {
				destination = key
			}
			verb := "Moved"
			if copyOnly {
				verb = "Copied"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %q from vault %q to %q in vault %q.\n", verb, key, from, destination, to)
			return nil
		},
	}

	cmd.Flags().String("from", "", "source vault (default: active vault)")
	cmd.Flags().String("to", "", "destination vault (default: same as source)")
	cmd.Flags().String("as", "", "store under a different key name")
	cmd.Flags().Bool("force", false, "overwrite the destination key if it exists")
	registerVaultFlagCompletion(app, cmd, "from", "to")
	return cmd
}

func transferAliases(copyOnly bool) []string {
	if copyOnly {
		return []string{"copy"}
	}
	return []string{"move"}
}

func registerVaultFlagCompletion(app *App, cmd *cobra.Command, flags ...string) {
	for _, flag := range flags {
		_ = cmd.RegisterFlagCompletionFunc(flag, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeVaults(app, toComplete)
		})
	}
}
