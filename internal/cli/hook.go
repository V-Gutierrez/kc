package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newHookCmd prints the directory hook that keeps the shell environment in step
// with the vault pinned by a .kc-vault marker.
//
// It is deliberately separate from `kc init`: init installs a wrapper that only
// reacts to commands the user types, while this changes what `cd` does. Opting
// in is an explicit act, and the Touch ID prompt that fires when the vault
// actually changes is the point, not a surprise.
func newHookCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:       "hook SHELL",
		Short:     "Print the cd hook that reloads secrets when the directory's vault changes",
		Long:      "Prints a shell hook that runs `kc env --sync` on every directory change. When the resolved vault is unchanged the hook is a no-op; when it changes, the previous vault's exports are unset and the new vault is loaded.\n\nzsh:  eval \"$(kc hook zsh)\"\nbash: eval \"$(kc hook bash)\"\nfish: kc hook fish | source",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"zsh", "bash", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			shell, err := normalizeShell(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), hookSnippet(shell))
			return nil
		},
	}
}

func hookSnippet(shell string) string {
	if shell == shellFish {
		return strings.Join([]string{
			"function __kc_sync --on-variable PWD",
			"    set -l kc_sync_output (command kc env --sync 2>/dev/null)",
			"    if test -n \"$kc_sync_output\"",
			"        printf '%s\\n' $kc_sync_output | source",
			"    end",
			"end",
			"__kc_sync",
		}, "\n")
	}

	lines := []string{
		"__kc_sync() {",
		"  local kc_sync_output",
		"  kc_sync_output=\"$(command kc env --sync 2>/dev/null)\"",
		"  [ -n \"$kc_sync_output\" ] && eval \"$kc_sync_output\"",
		"  return 0",
		"}",
	}
	if shell == shellZsh {
		lines = append(lines,
			"autoload -Uz add-zsh-hook 2>/dev/null",
			"add-zsh-hook chpwd __kc_sync 2>/dev/null || chpwd_functions+=(__kc_sync)",
		)
	} else {
		lines = append(lines,
			"case \"$PROMPT_COMMAND\" in",
			"  *__kc_sync*) ;;",
			"  \"\") PROMPT_COMMAND=\"__kc_sync\" ;;",
			"  *) PROMPT_COMMAND=\"__kc_sync;$PROMPT_COMMAND\" ;;",
			"esac",
		)
	}
	lines = append(lines, "__kc_sync")
	return strings.Join(lines, "\n")
}
