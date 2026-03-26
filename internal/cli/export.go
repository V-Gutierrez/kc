package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/v-gutierrez/kc/internal/envutil"
)

func newExportCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export all secrets from a vault as KEY=VALUE lines",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}
			outPath, _ := cmd.Flags().GetString("output")
			metadata, err := app.Store.ListMetadata(vault)
			if err != nil {
				return fmt.Errorf("export: %w", err)
			}
			for _, item := range metadata {
				if item.Protection == ProtectionProtected {
					session := authSession(app)
					if err := session.Authorize("Unlock kc secrets"); err != nil {
						return err
					}
					break
				}
			}

			entries, err := app.Bulk.GetAll(vault)
			if err != nil {
				return fmt.Errorf("export: %w", err)
			}

			keys := sortedKeys(entries)
			var lines []string
			for _, k := range keys {
				lines = append(lines, fmt.Sprintf("%s=%s", k, dotenvQuote(entries[k])))
			}

			output := joinLines(lines)
			if outPath != "" {
				if err := os.WriteFile(outPath, []byte(output), 0o600); err != nil {
					return fmt.Errorf("export: write file: %w", err)
				}
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), output)
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "", "write output to file instead of stdout")
	return cmd
}

func sortedKeys(m map[string]string) []string {
	return envutil.SortedKeys(m)
}

func joinLines(lines []string) string {
	return envutil.JoinLines(lines)
}
