package auth

import (
	"os"

	"golang.org/x/term"
)

// IsInteractive is a function that reports whether stdin is connected to a
// terminal (TTY). It is a package-level variable so that tests can override
// it to simulate interactive/non-interactive contexts.
//
// In production, IsInteractive returns true only when stdin is a real TTY.
// Use this to detect non-interactive contexts (e.g., shell redirection,
// cron, CI, Consi exec provider) where Touch ID would hang forever.
var IsInteractive = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
