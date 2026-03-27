package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newRunCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [--vault NAME] -- COMMAND [ARGS...]",
		Short: "Run a command with vault secrets injected into its environment",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dashIdx := cmd.ArgsLenAtDash()
			if dashIdx < 0 {
				return fmt.Errorf("usage: kc run [--vault NAME] -- COMMAND [ARGS...] (-- is required)")
			}
			childArgs := args[dashIdx:]
			if len(childArgs) == 0 {
				return fmt.Errorf("no command specified after --")
			}

			if app.Runner == nil {
				return fmt.Errorf("run: no command runner configured")
			}
			if app.Bulk == nil {
				return fmt.Errorf("run: bulk store not configured")
			}

			vault, err := app.resolveVault(cmd)
			if err != nil {
				return err
			}

			metadata, err := app.Store.ListMetadata(vault)
			if err != nil {
				return fmt.Errorf("run: %w", err)
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

			secrets, err := app.Bulk.GetAll(vault)
			if err != nil {
				return fmt.Errorf("run: %w", err)
			}

			env := mergeEnv(os.Environ(), secrets)

			exitCode, err := app.Runner(childArgs[0], childArgs[1:], env)
			if err != nil {
				return fmt.Errorf("run: %w", err)
			}
			if exitCode != 0 {
				return &ExitError{Code: exitCode}
			}
			return nil
		},
	}
	return cmd
}

func mergeEnv(parent []string, secrets map[string]string) []string {
	merged := make(map[string]string, len(parent)+len(secrets))
	order := make([]string, 0, len(parent)+len(secrets))

	for _, entry := range parent {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, exists := merged[k]; !exists {
			order = append(order, k)
		}
		merged[k] = v
	}

	for k, v := range secrets {
		if _, exists := merged[k]; !exists {
			order = append(order, k)
		}
		merged[k] = v
	}

	result := make([]string, 0, len(order))
	for _, k := range order {
		result = append(result, k+"="+merged[k])
	}
	return result
}
