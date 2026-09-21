package envutil_test

import (
	"testing"

	"github.com/v-gutierrez/kc/internal/envutil"
)

// An empty value has no characters to force quoting, but the empty token still
// has to survive as a token: without quotes it disappears from the command line
// instead of becoming an empty string.
func TestShellQuoteEmptyStringIsQuoted(t *testing.T) {
	if got := envutil.ShellQuote(""); got != "''" {
		t.Fatalf("ShellQuote(\"\") = %q, want %q", got, "''")
	}
}

func TestShellQuoteLeavesSimpleValuesBare(t *testing.T) {
	if got := envutil.ShellQuote("simple-value.1/ok:x"); got != "simple-value.1/ok:x" {
		t.Fatalf("ShellQuote quoted a safe value: %q", got)
	}
}
