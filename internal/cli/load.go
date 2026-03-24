package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLoadCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:    "load [VAULT]",
		Short:  "Load a vault into the current shell",
		Args:   cobra.MaximumNArgs(1),
		Hidden: true,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completeVaults(app, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), loadHint(args))
			return nil
		},
	}
}

func loadHint(args []string) string {
	message := "`kc load` works through shell integration because a subprocess cannot update your current shell environment. Install the wrapper from `kc init <shell>` in your shell config.\n\nFor zsh or bash:\n  eval \"$(kc init zsh)\"\nFor fish:\n  kc init fish | source\n\nThen reload your shell and run `kc load` again."
	if len(args) == 0 {
		return message + "\n\nFor a one-off load without shell integration, run:\n  eval \"$(kc env)\""
	}
	return message + "\n\nFor a one-off load of that vault without shell integration, run:\n  eval \"$(kc env --vault " + shellQuote(args[0]) + ")\""
}
