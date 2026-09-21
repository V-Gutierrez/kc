package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	internalaudit "github.com/v-gutierrez/kc/internal/audit"
)

func newAuditCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Scan vaults for weak, duplicate, stale, or suspicious secrets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			vaultsToScan, err := resolveAuditVaults(app, cmd)
			if err != nil {
				return err
			}

			referenceKeys, err := loadReferenceKeys(cmd)
			if err != nil {
				return err
			}

			rotationDays := resolveRotationDays(app, cmd)

			inputs := make([]internalaudit.ScanInput, 0, len(vaultsToScan))
			for _, vault := range vaultsToScan {
				entries, err := app.Bulk.GetAll(vault)
				if err != nil {
					return fmt.Errorf("audit: read vault %q: %w", vault, err)
				}
				inputs = append(inputs, internalaudit.ScanInput{
					Vault:         vault,
					Entries:       entries,
					ReferenceKeys: referenceKeys,
					MinLength:     16,
					Modified:      modifiedIndex(app, vault),
					RotationDays:  rotationDays,
				})
			}

			findings := internalaudit.Scan(inputs)
			if len(findings) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No issues found.")
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), "SEVERITY\tVAULT\tKEY\tRULE\tDETAIL")
			for _, finding := range findings {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", finding.Severity, finding.Vault, finding.Key, finding.Rule, finding.Detail)
			}
			return nil
		},
	}
	cmd.Flags().StringSlice("env-file", nil, "reference .env file(s) for stale-key detection")
	cmd.Flags().Int("rotation-days", 0, "flag credentials unchanged for this many days (default: config)")
	return cmd
}

// resolveRotationDays prefers the flag, then the config, then off.
func resolveRotationDays(app *App, cmd *cobra.Command) int {
	if cmd.Flags().Changed("rotation-days") {
		days, _ := cmd.Flags().GetInt("rotation-days")
		return days
	}
	if app.Config == nil {
		return 0
	}
	days, err := strconv.Atoi(app.Config.Get("audit.rotation_days"))
	if err != nil {
		return 0
	}
	return days
}

// modifiedIndex maps each key to when it was last written, for the rotation rule.
func modifiedIndex(app *App, vault string) map[string]string {
	metadata, err := app.Store.ListMetadata(vault)
	if err != nil {
		return nil
	}
	index := make(map[string]string, len(metadata))
	for _, item := range metadata {
		index[item.Key] = item.Modified
	}
	return index
}

func resolveAuditVaults(app *App, cmd *cobra.Command) ([]string, error) {
	if cmd.Flags().Changed("vault") {
		vault, err := app.resolveVault(cmd)
		if err != nil {
			return nil, err
		}
		return []string{vault}, nil
	}
	return app.Vaults.List()
}

func loadReferenceKeys(cmd *cobra.Command) (map[string]struct{}, error) {
	referenceKeys := make(map[string]struct{})
	for _, env := range os.Environ() {
		key, _, found := strings.Cut(env, "=")
		if found && strings.TrimSpace(key) != "" {
			referenceKeys[key] = struct{}{}
		}
	}

	paths, _ := cmd.Flags().GetStringSlice("env-file")
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("audit: cannot open %q: %w", path, err)
		}
		entries := parseEnvReader(file)
		file.Close()
		for key := range entries {
			referenceKeys[key] = struct{}{}
		}
	}

	return referenceKeys, nil
}
