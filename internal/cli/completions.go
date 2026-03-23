package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func completeKeys(app *App, cmd *cobra.Command, toComplete string) ([]string, cobra.ShellCompDirective) {
	vault, err := app.resolveVault(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	keys, err := app.Store.List(vault)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return filterCompletions(keys, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeVaults(app *App, toComplete string) ([]string, cobra.ShellCompDirective) {
	vaults, err := app.Vaults.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return filterCompletions(vaults, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func filterCompletions(values []string, toComplete string) []string {
	if toComplete == "" {
		return values
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, toComplete) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}
