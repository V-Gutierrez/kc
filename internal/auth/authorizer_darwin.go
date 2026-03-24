//go:build darwin

package auth

import (
	"context"
	"fmt"
	"time"

	touchid "github.com/noamcohen97/touchid-go"
)

type TouchIDAuthorizer struct{}

func NewTouchIDAuthorizer() Authorizer {
	return TouchIDAuthorizer{}
}

func (TouchIDAuthorizer) Authorize(reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if reason == "" {
		reason = "Unlock kc secret"
	}
	if err := touchid.Authenticate(ctx, touchid.PolicyDeviceOwnerAuthentication, reason); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	return nil
}
