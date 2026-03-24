//go:build darwin

package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	touchid "github.com/noamcohen97/touchid-go"
)

func TestTouchIDAuthorizerEmptyReasonUsesDefault(t *testing.T) {
	previous := touchIDAuthenticate
	defer func() { touchIDAuthenticate = previous }()

	touchIDAuthenticate = func(ctx context.Context, policy touchid.Policy, reason string) error {
		if reason != "Unlock kc secret" {
			t.Fatalf("reason = %q, want default", reason)
		}
		return errors.New("denied")
	}

	err := TouchIDAuthorizer{}.Authorize("")
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("error = %v, want wrapped authentication error", err)
	}
}
