package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completion SHELL",
		Short:     "Generate shell completion script",
		Long:      "Generate a completion script for bash, zsh, fish, or powershell.\nOutput is written to stdout so it can be sourced or redirected.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			out := cmd.OutOrStdout()

			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			default:
				return fmt.Errorf("unsupported shell %q (valid: bash, zsh, fish, powershell)", args[0])
			}
		},
	}
}
