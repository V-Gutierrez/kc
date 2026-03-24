//go:build !darwin

package auth

func isBootSessionValid() bool {
	return false
}

func writeBootSessionToken() error {
	return nil
}
