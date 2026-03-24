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

func listItemsWithProtection(items []SecretMetadata) []map[string]string {
	result := make([]map[string]string, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]string{
			"key":        item.Key,
			"vault":      item.Vault,
			"protection": item.Protection,
		})
	}
	return result
}
