package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/v-gutierrez/kc/internal/output"
)

func newConfigCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and write kc settings",
		Long:  "kc settings live in ~/.kc/config. Unknown keys are rejected so a typo fails loudly.",
	}
	cmd.AddCommand(
		newConfigListCmd(app),
		newConfigGetCmd(app),
		newConfigSetCmd(app),
		newConfigUnsetCmd(app),
		newConfigPathCmd(app),
	)
	return cmd
}

func newConfigListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "Show every setting with its effective value",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := app.settings()
			if err != nil {
				return err
			}

			values := settings.All()
			keys := make([]string, 0, len(values))
			for key := range values {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			jsonOutput, _ := cmd.Flags().GetBool("json")
			if jsonOutput {
				return output.WriteJSON(cmd.OutOrStdout(), values)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "KEY\tVALUE\tPURPOSE")
			for _, key := range keys {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", key, values[key], settings.Describe(key))
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output structured JSON")
	return cmd
}

func newConfigGetCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:               "get KEY",
		Short:             "Print the effective value of one setting",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeSettings(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := app.settings()
			if err != nil {
				return err
			}
			if !settings.Known(args[0]) {
				return fmt.Errorf("config: unknown key %q", args[0])
			}
			fmt.Fprintln(cmd.OutOrStdout(), settings.Get(args[0]))
			return nil
		},
	}
}

func newConfigSetCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:               "set KEY VALUE",
		Short:             "Change a setting",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeSettings(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := app.settings()
			if err != nil {
				return err
			}
			if err := settings.Set(args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", args[0], args[1])
			return nil
		},
	}
}

func newConfigUnsetCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:               "unset KEY",
		Short:             "Drop an override so the default applies again",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeSettings(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := app.settings()
			if err != nil {
				return err
			}
			if err := settings.Unset(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unset %s (now %s).\n", args[0], settings.Get(args[0]))
			return nil
		},
	}
}

func newConfigPathCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config file location",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := app.settings()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), settings.Path())
			return nil
		},
	}
}

func completeSettings(app *App) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 || app.Config == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return app.Config.Keys(), cobra.ShellCompDirectiveNoFileComp
	}
}
