package keychain

import (
	"errors"
	"fmt"
	"testing"
)

// fakeRunner records calls and returns preset responses.
type fakeRunner struct {
	calls   []fakeCall
	results []fakeResult
}

type fakeCall struct {
	Name string
	Args []string
}

type fakeResult struct {
	Output []byte
	Err    error
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{Name: name, Args: args})
	if len(f.results) == 0 {
		return nil, fmt.Errorf("fakeRunner: no results queued")
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r.Output, r.Err
}

// --- Get tests ---

func TestGet_Success(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{
			{Output: []byte("my-secret-value\n"), Err: nil},
		},
	}
	kc := &Keychain{Runner: runner}

	val, err := kc.Get("kc:default", "API_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "my-secret-value" {
		t.Fatalf("got %q, want %q", val, "my-secret-value")
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
	c := runner.calls[0]
	if c.Name != "security" {
		t.Fatalf("expected security command, got %q", c.Name)
	}
	wantArgs := []string{"find-generic-password", "-s", "kc:default", "-a", "API_KEY", "-w"}
	if fmt.Sprintf("%v", c.Args) != fmt.Sprintf("%v", wantArgs) {
		t.Fatalf("args = %v, want %v", c.Args, wantArgs)
	}
}

func TestGet_NotFound(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{
			{
				Output: []byte("security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.\n"),
				Err:    errors.New("exit status 44"),
			},
		},
	}
	kc := &Keychain{Runner: runner}

	_, err := kc.Get("kc:default", "MISSING")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- Set tests ---

func TestSet_Success(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{
			// Add succeeds
			{Output: nil, Err: nil},
		},
	}
	kc := &Keychain{Runner: runner}

	err := kc.Set("kc:default", "API_KEY", "new-value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
	if runner.calls[0].Args[0] != "add-generic-password" {
		t.Fatalf("call should be add, got %v", runner.calls[0].Args)
	}
	if fmt.Sprintf("%v", runner.calls[0].Args) != fmt.Sprintf("%v", []string{"add-generic-password", "-s", "kc:default", "-a", "API_KEY", "-w", "new-value", "-j", protectedComment, "-U"}) {
		t.Fatalf("args = %v", runner.calls[0].Args)
	}
}

func TestSet_UnprotectedStoresEmptyComment(t *testing.T) {
	runner := &fakeRunner{results: []fakeResult{{Output: nil, Err: nil}}}
	kc := &Keychain{Runner: runner}

	if err := kc.SetWithProtection("kc:default", "API_KEY", "new-value", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := fmt.Sprintf("%v", runner.calls[0].Args); got != fmt.Sprintf("%v", []string{"add-generic-password", "-s", "kc:default", "-a", "API_KEY", "-w", "new-value", "-j", "", "-U"}) {
		t.Fatalf("args = %v", runner.calls[0].Args)
	}
}

// --- Delete tests ---

func TestDelete_Success(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{
			{Output: []byte("password has been deleted.\n"), Err: nil},
		},
	}
	kc := &Keychain{Runner: runner}

	err := kc.Delete("kc:default", "API_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{
			{
				Output: []byte("security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.\n"),
				Err:    errors.New("exit status 44"),
			},
		},
	}
	kc := &Keychain{Runner: runner}

	err := kc.Delete("kc:default", "MISSING")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- List tests ---

func TestList_ParsesAccounts(t *testing.T) {
	dumpOutput := `keychain: "/Users/test/Library/Keychains/login.keychain-db"
version: 512
class: "genp"
    0x00000007 <blob>="kc:default"
    "svce"<blob>="kc:default"
    "acct"<blob>="DB_HOST"
class: "genp"
    0x00000007 <blob>="kc:default"
    "svce"<blob>="kc:default"
    "acct"<blob>="DB_PASS"
class: "genp"
    0x00000007 <blob>="kc:staging"
    "svce"<blob>="kc:staging"
    "acct"<blob>="OTHER_KEY"
`

	runner := &fakeRunner{
		results: []fakeResult{
			{Output: []byte(dumpOutput), Err: nil},
		},
	}
	kc := &Keychain{Runner: runner}

	accounts, err := kc.List("kc:default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d: %v", len(accounts), accounts)
	}
	if accounts[0] != "DB_HOST" || accounts[1] != "DB_PASS" {
		t.Fatalf("unexpected accounts: %v", accounts)
	}
}

func TestList_Empty(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{
			{Output: []byte("keychain: \"/Users/test/Library/Keychains/login.keychain-db\"\n"), Err: nil},
		},
	}
	kc := &Keychain{Runner: runner}

	accounts, err := kc.List("kc:nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected 0 accounts, got %d", len(accounts))
	}
}

// --- parseAccounts unit tests ---

func TestParseAccounts(t *testing.T) {
	dump := `class: "genp"
    "svce"<blob>="kc:prod"
    "acct"<blob>="SECRET_A"
class: "genp"
    "svce"<blob>="kc:prod"
    "acct"<blob>="SECRET_B"
class: "genp"
    "svce"<blob>="kc:dev"
    "acct"<blob>="DEV_ONLY"
`
	got := parseAccounts(dump, "kc:prod")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(got), got)
	}
	if got[0] != "SECRET_A" || got[1] != "SECRET_B" {
		t.Fatalf("unexpected: %v", got)
	}
}

func TestParseMetadata(t *testing.T) {
	dump := `class: "genp"
    "svce"<blob>="kc:prod"
    "acct"<blob>="SECRET_A"
    "icmt"<blob>="kc-meta:v1:protected"
class: "genp"
    "svce"<blob>="kc:prod"
    "acct"<blob>="SECRET_B"
    "icmt"<blob>=""
`
	got := parseMetadata(dump, "kc:prod")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(got), got)
	}
	if !got[0].Protected {
		t.Fatalf("first item should be protected: %#v", got[0])
	}
	if got[1].Protected {
		t.Fatalf("second item should be unprotected: %#v", got[1])
	}
}

func TestProtection(t *testing.T) {
	dumpOutput := `class: "genp"
    "svce"<blob>="kc:default"
    "acct"<blob>="DB_HOST"
    "icmt"<blob>="kc-meta:v1:protected"
class: "genp"
    "svce"<blob>="kc:default"
    "acct"<blob>="DB_PASS"
    "icmt"<blob>=""
`
	runner := &fakeRunner{results: []fakeResult{{Output: []byte(dumpOutput), Err: nil}, {Output: []byte(dumpOutput), Err: nil}}}
	kc := &Keychain{Runner: runner}

	protected, err := kc.Protection("kc:default", "DB_HOST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !protected {
		t.Fatal("DB_HOST should be protected")
	}

	protected, err = kc.Protection("kc:default", "DB_PASS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if protected {
		t.Fatal("DB_PASS should be unprotected")
	}
}

func TestParseAccounts_IgnoresMatchingValueOutsideServiceField(t *testing.T) {
	dump := `class: "genp"
    "acct"<blob>="SECRET_A"
    "svce"<blob>="kc:prod"
class: "genp"
    "acct"<blob>="SECRET_B"
    "labl"<blob>="kc:prod"
    "svce"<blob>="kc:other"
`
	got := parseAccounts(dump, "kc:prod")
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d: %v", len(got), got)
	}
	if got[0] != "SECRET_A" {
		t.Fatalf("unexpected: %v", got)
	}
}

func TestDigest(t *testing.T) {
	first := Digest("value")
	second := Digest("value")
	if first != second {
		t.Fatal("expected stable digest")
	}
	if first == Digest("other") {
		t.Fatal("expected distinct digests")
	}
}

func TestExtractQuotedValue(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{`    "acct"<blob>="MY_KEY"`, "MY_KEY"},
		{`    "svce"<blob>="kc:default"`, "kc:default"},
		{`    "acct"<blob>=<NULL>`, ""},
	}
	for _, tt := range tests {
		got := extractQuotedValue(tt.line)
		if got != tt.want {
			t.Errorf("extractQuotedValue(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}
