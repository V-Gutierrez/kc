package envutil

import (
	"os"
	"path/filepath"
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

func TestUpsertEnvFile_UpdatesExistingKeyInPlace(t *testing.T) {
	path := writeTempEnvFile(t, "# before\nFOO=old\nBAR=keep\n")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"FOO": "new value"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 1 || appended != 0 {
		t.Fatalf("counts = (%d, %d), want (1, 0)", updated, appended)
	}
	assertFileContent(t, path, "# before\nFOO=\"new value\"\nBAR=keep\n")
}

func TestUpsertEnvFile_UncommentsSpacedCommentedKey(t *testing.T) {
	path := writeTempEnvFile(t, "# FOO=old\n")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"FOO": "new"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 1 || appended != 0 {
		t.Fatalf("counts = (%d, %d), want (1, 0)", updated, appended)
	}
	assertFileContent(t, path, "FOO=new\n")
}

func TestUpsertEnvFile_UncommentsCommentedKeyWithoutSpace(t *testing.T) {
	path := writeTempEnvFile(t, "#FOO=old\n")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"FOO": "new"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 1 || appended != 0 {
		t.Fatalf("counts = (%d, %d), want (1, 0)", updated, appended)
	}
	assertFileContent(t, path, "FOO=new\n")
}

func TestUpsertEnvFile_AppendsMissingKeysAtEnd(t *testing.T) {
	path := writeTempEnvFile(t, "FOO=old\n")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"BAR": "new"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 0 || appended != 1 {
		t.Fatalf("counts = (%d, %d), want (0, 1)", updated, appended)
	}
	assertFileContent(t, path, "FOO=old\nBAR=new\n")
}

func TestUpsertEnvFile_AppendsToEmptyFile(t *testing.T) {
	path := writeTempEnvFile(t, "")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"FOO": "new"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 0 || appended != 1 {
		t.Fatalf("counts = (%d, %d), want (0, 1)", updated, appended)
	}
	assertFileContent(t, path, "FOO=new\n")
}

func TestUpsertEnvFile_AppendsAfterFileWithoutTrailingNewline(t *testing.T) {
	path := writeTempEnvFile(t, "FOO=old")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"BAR": "new"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 0 || appended != 1 {
		t.Fatalf("counts = (%d, %d), want (0, 1)", updated, appended)
	}
	assertFileContent(t, path, "FOO=old\nBAR=new\n")
}

func TestUpsertEnvFile_PreservesWindowsLineEndings(t *testing.T) {
	path := writeTempEnvFile(t, "FOO=old\r\nBAR=keep\r\n")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"FOO": "new", "BAZ": "added"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 1 || appended != 1 {
		t.Fatalf("counts = (%d, %d), want (1, 1)", updated, appended)
	}
	assertFileContent(t, path, "FOO=new\r\nBAR=keep\r\nBAZ=added\r\n")
}

func TestUpsertEnvFile_UpdatesOnlyFirstDuplicateKey(t *testing.T) {
	path := writeTempEnvFile(t, "FOO=old\nFOO=local-override\n")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"FOO": "new"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 1 || appended != 0 {
		t.Fatalf("counts = (%d, %d), want (1, 0)", updated, appended)
	}
	assertFileContent(t, path, "FOO=new\nFOO=local-override\n")
}

func TestUpsertEnvFile_HandlesEqualsInValueAndEmptyValue(t *testing.T) {
	path := writeTempEnvFile(t, "URL=old\nEMPTY=old\n")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"URL": "postgres://u:p@host/db?sslmode=require", "EMPTY": ""})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 2 || appended != 0 {
		t.Fatalf("counts = (%d, %d), want (2, 0)", updated, appended)
	}
	assertFileContent(t, path, "URL=\"postgres://u:p@host/db?sslmode=require\"\nEMPTY=\n")
}

func TestUpsertEnvFile_MixedUpdateUncommentAppend(t *testing.T) {
	path := writeTempEnvFile(t, "# header\nFOO=old\n# BAR=old\nBAZ=keep\n")

	updated, appended, err := UpsertEnvFile(path, map[string]string{
		"FOO": "new foo",
		"BAR": "newbar",
		"QUX": "newqux",
	})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 2 || appended != 1 {
		t.Fatalf("counts = (%d, %d), want (2, 1)", updated, appended)
	}
	assertFileContent(t, path, "# header\nFOO=\"new foo\"\nBAR=newbar\nBAZ=keep\nQUX=newqux\n")
}

func TestUpsertEnvFile_CleansTempFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := UpsertEnvFile(path, map[string]string{"FOO": "new"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".env.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp files left behind: %#v", matches)
	}
}

func TestUpsertEnvFile_CreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")

	updated, appended, err := UpsertEnvFile(path, map[string]string{"FOO": "new"})
	if err != nil {
		t.Fatalf("UpsertEnvFile() error = %v", err)
	}
	if updated != 0 || appended != 1 {
		t.Fatalf("counts = (%d, %d), want (0, 1)", updated, appended)
	}
	assertFileContent(t, path, "FOO=new\n")
}

func writeTempEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
}
