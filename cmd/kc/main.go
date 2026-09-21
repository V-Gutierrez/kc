package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/v-gutierrez/kc/internal/app"
	"github.com/v-gutierrez/kc/internal/auth"
	"github.com/v-gutierrez/kc/internal/cli"
	"github.com/v-gutierrez/kc/internal/clipboard"
	"github.com/v-gutierrez/kc/internal/keychain"
	"github.com/v-gutierrez/kc/internal/vault"
)

func main() {
	handled, err := clipboard.RunClearIfRequested()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if handled {
		return
	}

	application, err := app.Build(app.Deps{
		Vaults:    vault.New(keychain.New()),
		Clipboard: clipboard.New(),
		Auth:      auth.NewTouchIDAuthorizer(),
		Runner:    execRunner,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	root := cli.NewRootCmd(application)
	if err := root.Execute(); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execRunner(name string, args []string, env []string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}
