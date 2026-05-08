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
			envFilePath, _ := cmd.Flags().GetString("env-file")
			requestedKeys, _ := cmd.Flags().GetStringSlice("keys")
			if envFilePath != "" && outPath != "" {
				return fmt.Errorf("export: --env-file and --output are mutually exclusive")
			}
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
			entries, err = filterExportEntries(entries, requestedKeys)
			if err != nil {
				return err
			}

			if envFilePath != "" {
				updated, appended, err := envutil.UpsertEnvFile(envFilePath, entries)
				if err != nil {
					return fmt.Errorf("export: upsert env file: %w", err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "✓ %d keys updated, %d appended → %s\n", updated, appended, envFilePath)
				return nil
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
	cmd.Flags().String("env-file", "", "upsert secrets into existing .env file")
	cmd.Flags().StringSlice("keys", nil, "comma-separated list of keys to export (default: all)")
	return cmd
}

func filterExportEntries(entries map[string]string, requestedKeys []string) (map[string]string, error) {
	if len(requestedKeys) == 0 {
		return entries, nil
	}
	filtered := make(map[string]string, len(requestedKeys))
	for _, key := range requestedKeys {
		if key == "" {
			continue
		}
		value, ok := entries[key]
		if !ok {
			return nil, fmt.Errorf("export: requested key %q not found in vault", key)
		}
		filtered[key] = value
	}
	return filtered, nil
}

func sortedKeys(m map[string]string) []string {
	return envutil.SortedKeys(m)
}

func joinLines(lines []string) string {
	return envutil.JoinLines(lines)
}
