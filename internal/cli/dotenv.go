package cli

import (
	"io"
	"strings"

	"github.com/v-gutierrez/kc/internal/envutil"
)

func parseEnvReader(r io.Reader) map[string]string {
	return envutil.ParseEnvReader(r)
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


func dotenvQuote(s string) string {
	return envutil.DotenvQuote(s)
}
