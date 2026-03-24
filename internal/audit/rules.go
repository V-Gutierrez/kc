package audit

import (
	"strings"
	"unicode"
)

func weakReasons(value string, minLength int) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return []string{"empty value"}
	}

	reasons := make([]string, 0, 3)
	if len(trimmed) < minLength {
		reasons = append(reasons, "shorter than 16 characters")
	}
	if !hasSpecialCharacter(trimmed) {
		reasons = append(reasons, "missing special characters")
	}
	if isCommonWeakPattern(trimmed) {
		reasons = append(reasons, "matches common weak pattern")
	}
	return reasons
}

func hasSpecialCharacter(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return true
	}
	return false
}

func isCommonWeakPattern(value string) bool {
	lowered := strings.ToLower(strings.TrimSpace(value))
	common := []string{"password", "secret", "changeme", "abc123", "123456", "qwerty", "letmein", "admin", "default", "test"}
	for _, item := range common {
		if lowered == item {
			return true
		}
	}
	return false
}

func isSuspiciousName(key string) bool {
	lowered := strings.ToLower(strings.TrimSpace(key))
	return lowered == "test" || lowered == "temp" || strings.HasPrefix(lowered, "old_") || strings.HasPrefix(lowered, "old-")
}
