package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/v-gutierrez/kc/internal/auth"
)

func authSession(app *App) *auth.Session {
	if app == nil {
		return auth.NewSession(nil)
	}
	return auth.NewSession(app.Auth)
}

func isProtected(metadata []SecretMetadata, key string) bool {
	for _, item := range metadata {
		if item.Key == key {
			return item.Protection == ProtectionProtected
		}
	}
	return false
}

// shouldSkipAuth reports whether Touch ID authentication should be
// bypassed for this command invocation.
//
// Returns true if:
//   - The --no-touch-id flag was explicitly set, OR
//   - stdin is not a terminal (non-interactive context: pipe, cron, CI, Consi exec).
//
// In the non-interactive case, a warning is printed to stderr.
func shouldSkipAuth(cmd *cobra.Command) bool {
	noTouchID, _ := cmd.Flags().GetBool("no-touch-id")
	if noTouchID {
		return true
	}
	if !auth.IsInteractive() {
		fmt.Fprintln(cmd.ErrOrStderr(), "⚠️  Non-interactive session detected. Touch ID disabled.")
		return true
	}
	return false
}
