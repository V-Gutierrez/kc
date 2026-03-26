package envutil

import (
	"strings"
	"testing"
)

func TestParseEnvReader(t *testing.T) {
	input := strings.NewReader(`
# comment
FOO=bar
SPACED = baz # trailing comment
QUOTED="hello world"
SINGLE='two words'
EMPTY=
INVALID
`)

	got := ParseEnvReader(input)
	if got["FOO"] != "bar" {
		t.Fatalf("FOO = %q, want bar", got["FOO"])
	}
	if got["SPACED"] != "baz" {
		t.Fatalf("SPACED = %q, want baz", got["SPACED"])
	}
	if got["QUOTED"] != "hello world" {
		t.Fatalf("QUOTED = %q, want hello world", got["QUOTED"])
	}
	if got["SINGLE"] != "two words" {
		t.Fatalf("SINGLE = %q, want two words", got["SINGLE"])
	}
	if got["EMPTY"] != "" {
		t.Fatalf("EMPTY = %q, want empty string", got["EMPTY"])
	}
	if _, exists := got["INVALID"]; exists {
		t.Fatalf("did not expect INVALID entry in %#v", got)
	}
}

func TestShellQuote(t *testing.T) {
	if got := ShellQuote("simple_value"); got != "simple_value" {
		t.Fatalf("ShellQuote(simple_value) = %q", got)
	}
	if got := ShellQuote("two words"); got != "'two words'" {
		t.Fatalf("ShellQuote(two words) = %q", got)
	}
	if got := ShellQuote("has'quote"); got != "'has'\\''quote'" {
		t.Fatalf("ShellQuote(has'quote) = %q", got)
	}
}

func TestDotenvQuote(t *testing.T) {
	if got := DotenvQuote("simple_value"); got != "simple_value" {
		t.Fatalf("DotenvQuote(simple_value) = %q", got)
	}
	if got := DotenvQuote("two words"); got != "\"two words\"" {
		t.Fatalf("DotenvQuote(two words) = %q", got)
	}
	if got := DotenvQuote(`say "hi" \\ now`); got != `"say \"hi\" \\\\ now"` {
		t.Fatalf("DotenvQuote(escaped) = %q", got)
	}
}

func TestSortedKeysAndJoinLines(t *testing.T) {
	keys := SortedKeys(map[string]string{"B": "2", "A": "1"})
	if len(keys) != 2 || keys[0] != "A" || keys[1] != "B" {
		t.Fatalf("SortedKeys() = %#v, want [A B]", keys)
	}
	if got := JoinLines([]string{"A=1", "B=2"}); got != "A=1\nB=2\n" {
		t.Fatalf("JoinLines() = %q", got)
	}
}
