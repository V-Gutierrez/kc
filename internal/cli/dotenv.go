package cli

import (
	"io"

	"github.com/v-gutierrez/kc/internal/envutil"
)

func parseEnvReader(r io.Reader) map[string]string {
	return envutil.ParseEnvReader(r)
}

func stripInlineComment(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] == '\'' || s[0] == '"' {
		return s
	}
	if idx := strings.Index(s, " #"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

func unquoteValue(s string) string {
	if len(s) >= 2 {
		if s[0] == '\'' && s[len(s)-1] == '\'' {
			return s[1 : len(s)-1]
		}
		if s[0] == '"' && s[len(s)-1] == '"' {
			inner := s[1 : len(s)-1]
			inner = strings.ReplaceAll(inner, `\"`, `"`)
			inner = strings.ReplaceAll(inner, `\\`, `\`)
			return inner
		}
	}
	return s
}

func shellQuote(s string) string {
	return envutil.ShellQuote(s)
}

func needsQuoting(s string) bool {
	return envutil.NeedsQuoting(s)
}

func dotenvQuote(s string) string {
	return envutil.DotenvQuote(s)
}
