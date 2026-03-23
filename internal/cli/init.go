package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInitCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "init SHELL",
		Short: "Print the shell snippet needed to load kc secrets on startup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell, err := normalizeShell(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), initSnippet(shell))
			return nil
		},
	}
}
