//go:build !darwin

package auth

import (
	"strings"
	"testing"
)

func TestUnsupportedAuthorizerReturnsError(t *testing.T) {
	err := UnsupportedAuthorizer{}.Authorize("Unlock kc secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "darwin") {
		t.Fatalf("error = %q, want darwin message", err.Error())
	}
}
