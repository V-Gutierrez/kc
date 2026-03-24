package cli

import "github.com/v-gutierrez/kc/internal/auth"

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
