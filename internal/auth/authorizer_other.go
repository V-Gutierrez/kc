//go:build !darwin

package auth

import "fmt"

type UnsupportedAuthorizer struct{}

func NewTouchIDAuthorizer() Authorizer {
	return UnsupportedAuthorizer{}
}

func (UnsupportedAuthorizer) Authorize(reason string) error {
	return fmt.Errorf("authentication is only supported on darwin")
}
