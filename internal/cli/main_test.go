package cli_test

import (
	"os"
	"testing"

	"github.com/v-gutierrez/kc/internal/auth"
)

func TestMain(m *testing.M) {
	// Force interactive mode in tests so Touch ID auth is not skipped.
	// Without this, the test runner (which is non-interactive) would
	// trigger the auto-skip path in shouldSkipAuth.
	auth.IsInteractive = func() bool { return true }

	os.Exit(m.Run())
}
