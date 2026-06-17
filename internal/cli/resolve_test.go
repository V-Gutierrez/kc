package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/v-gutierrez/kc/internal/cli"
)

func pipeStdin(t *testing.T, input string) func() {
	t.Helper()
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	w.Write([]byte(input))
	w.Close()
	return func() { os.Stdin = oldStdin }
}

func executeResolve(app *cli.App, args ...string) (stdout string, err error) {
	root := cli.NewRootCmd(app)
	outBuf := &readAllBuf{}
	errBuf := &readAllBuf{}
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), err
}

type resolveResponse struct {
	ProtocolVersion int                `json:"protocolVersion"`
	Values          map[string]*string `json:"values"`
}

func parseResolve(t *testing.T, stdout string) resolveResponse {
	t.Helper()
	var resp resolveResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("json.Unmarshal response: %v\nstdout: %q", err, stdout)
	}
	return resp
}

func assertValue(t *testing.T, values map[string]*string, key string, want *string) {
	t.Helper()
	got, ok := values[key]
	if !ok {
		t.Fatalf("missing key %q in response", key)
	}
	if want == nil {
		if got != nil {
			t.Errorf("%s = %q, want nil", key, *got)
		}
		return
	}
	if got == nil {
		t.Errorf("%s = nil, want %q", key, *want)
		return
	}
	if *got != *want {
		t.Errorf("%s = %q, want %q", key, *got, *want)
	}
}

func TestResolve_SingleKey(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "OPENAI_API_KEY", "sk-test123")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["OPENAI_API_KEY"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "sk-test123"
	assertValue(t, resp.Values, "OPENAI_API_KEY", &want)
	if resp.ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d, want 1", resp.ProtocolVersion)
	}
}

func TestResolve_MultipleKeys(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "A", "1")
	store.Set("default", "B", "2")
	store.Set("default", "C", "3")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["A","B","C"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	wantA, wantB, wantC := "1", "2", "3"
	assertValue(t, resp.Values, "A", &wantA)
	assertValue(t, resp.Values, "B", &wantB)
	assertValue(t, resp.Values, "C", &wantC)
}

func TestResolve_UnknownKeyReturnsNull(t *testing.T) {
	app, _, _, _ := newTestApp()

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["INEXISTENTE"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	assertValue(t, resp.Values, "INEXISTENTE", nil)
}

func TestResolve_MixedKnownUnknown(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "REAL", "real-value")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["REAL","FAKE"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "real-value"
	assertValue(t, resp.Values, "REAL", &want)
	assertValue(t, resp.Values, "FAKE", nil)
}

func TestResolve_EmptyIDs(t *testing.T) {
	app, _, _, _ := newTestApp()

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":[]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	if len(resp.Values) != 0 {
		t.Errorf("len(values) = %d, want 0", len(resp.Values))
	}
}

func TestResolve_EmptyStringID(t *testing.T) {
	app, _, _, _ := newTestApp()

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":[""]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	assertValue(t, resp.Values, "", nil)
}

func TestResolve_EmptyStdin(t *testing.T) {
	app, _, _, _ := newTestApp()

	revert := pipeStdin(t, ``)
	defer revert()

	_, err := executeResolve(app, "resolve")
	if err == nil {
		t.Fatal("expected error for empty stdin")
	}
	// In JSON parse path, empty input gives "invalid JSON"; check it mentions resolve.
	if !strings.Contains(err.Error(), "resolve") {
		t.Errorf("error = %q, want it to mention resolve", err.Error())
	}
}

func TestResolve_InvalidJSON(t *testing.T) {
	app, _, _, _ := newTestApp()

	revert := pipeStdin(t, `not json`)
	defer revert()

	_, err := executeResolve(app, "resolve")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "resolve") {
		t.Errorf("error = %q, want it to mention resolve", err.Error())
	}
}

func TestResolve_NoTouchID_ProtectedKeysResolved(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	store.SetWithProtection("default", "SECRET", "s3cr3t", true)
	store.SetWithProtection("default", "OPEN", "public", false)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["SECRET","OPEN"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve", "--no-touch-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	wantSecret := "s3cr3t"
	wantOpen := "public"
	assertValue(t, resp.Values, "SECRET", &wantSecret)
	assertValue(t, resp.Values, "OPEN", &wantOpen)
	if authorizer.calls != 0 {
		t.Errorf("authorizer calls = %d, want 0 (no-touch-id skips auth)", authorizer.calls)
	}
}

func TestResolve_NoTouchID_AuthDeniedIgnored(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	authorizer.err = fmt.Errorf("denied")
	store.SetWithProtection("default", "SECRET", "s3cr3t", true)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["SECRET"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve", "--no-touch-id")
	if err != nil {
		t.Fatalf("unexpected error even when auth would be denied: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "s3cr3t"
	assertValue(t, resp.Values, "SECRET", &want)
	if authorizer.calls != 0 {
		t.Errorf("authorizer calls = %d, want 0 (no-touch-id skips auth entirely)", authorizer.calls)
	}
}

func TestResolve_NoTouchID_UnprotectedStillWorks(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	store.SetWithProtection("default", "KEY", "val", false)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["KEY"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve", "--no-touch-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "val"
	assertValue(t, resp.Values, "KEY", &want)
	if authorizer.calls != 0 {
		t.Errorf("authorizer calls = %d, want 0", authorizer.calls)
	}
}

func TestResolve_NoTouchID_WithVaultFlag(t *testing.T) {
	app, store, _, vaults, authorizer := newAuthorizedTestAppWithBulk()
	vaults.Create("prod")
	store.SetWithProtection("prod", "API_KEY", "prod-secret", true)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["API_KEY"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve", "--vault", "prod", "--no-touch-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "prod-secret"
	assertValue(t, resp.Values, "API_KEY", &want)
	if authorizer.calls != 0 {
		t.Errorf("authorizer calls = %d, want 0", authorizer.calls)
	}
}

func TestResolve_WithVaultFlag(t *testing.T) {
	app, store, vaults, _ := newTestApp()
	vaults.Create("staging")
	store.Set("staging", "DB_PASS", "pg123")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["DB_PASS"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve", "--vault", "staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "pg123"
	assertValue(t, resp.Values, "DB_PASS", &want)
}

func TestResolve_NonexistentVault(t *testing.T) {
	app, _, _, _ := newTestApp()

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["X"]}`)
	defer revert()

	_, err := executeResolve(app, "resolve", "--vault", "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent vault")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestResolve_ProtectedKeyTriggersAuthOnce(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	store.SetWithProtection("default", "SECRET", "s3cr3t", true)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["SECRET"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "s3cr3t"
	assertValue(t, resp.Values, "SECRET", &want)
	if authorizer.calls != 1 {
		t.Errorf("authorizer calls = %d, want 1", authorizer.calls)
	}
}

func TestResolve_MultipleProtectedKeysTriggersAuthOnce(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	store.SetWithProtection("default", "S1", "one", true)
	store.SetWithProtection("default", "S2", "two", true)
	store.SetWithProtection("default", "S3", "three", true)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["S1","S2","S3"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want1, want2, want3 := "one", "two", "three"
	assertValue(t, resp.Values, "S1", &want1)
	assertValue(t, resp.Values, "S2", &want2)
	assertValue(t, resp.Values, "S3", &want3)
	if authorizer.calls != 1 {
		t.Errorf("authorizer calls = %d, want 1", authorizer.calls)
	}
}

func TestResolve_AuthDeniedReturnsError(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	authorizer.err = fmt.Errorf("denied")
	store.SetWithProtection("default", "SECRET", "s3cr3t", true)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["SECRET"]}`)
	defer revert()

	_, err := executeResolve(app, "resolve")
	if err == nil {
		t.Fatal("expected error when auth denied")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Errorf("error = %q, want 'denied'", err.Error())
	}
}

func TestResolve_UnprotectedKeySkipsAuth(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	store.SetWithProtection("default", "PUBLIC", "value", false)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["PUBLIC"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "value"
	assertValue(t, resp.Values, "PUBLIC", &want)
	if authorizer.calls != 0 {
		t.Errorf("authorizer calls = %d, want 0 (unprotected)", authorizer.calls)
	}
}

func TestResolve_MixedProtectedAndUnprotectedTriggersAuthOnce(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	store.SetWithProtection("default", "PROT", "secret", true)
	store.SetWithProtection("default", "OPEN", "public", false)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["PROT","OPEN"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	wantProt := "secret"
	wantOpen := "public"
	assertValue(t, resp.Values, "PROT", &wantProt)
	assertValue(t, resp.Values, "OPEN", &wantOpen)
	if authorizer.calls != 1 {
		t.Errorf("authorizer calls = %d, want 1", authorizer.calls)
	}
}

func TestResolve_PreservesHTMLEntities(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "HTML", "<>&\"'")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["HTML"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "<>&\"'"
	assertValue(t, resp.Values, "HTML", &want)

	if strings.Contains(stdout, "&lt;") || strings.Contains(stdout, "&gt;") || strings.Contains(stdout, "&amp;") {
		t.Errorf("stdout = %q, values should not be HTML-escaped", stdout)
	}
}

func TestResolve_PreservesSpecialCharacters(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "JSON", "{\"nested\":\"value\"}")
	store.Set("default", "UNICODE", "caf\u00e9")
	store.Set("default", "NEWLINES", "line1\nline2\r\nline3")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["JSON","UNICODE","NEWLINES"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	wantJSON := `{"nested":"value"}`
	assertValue(t, resp.Values, "JSON", &wantJSON)
	wantUnicode := "café"
	assertValue(t, resp.Values, "UNICODE", &wantUnicode)
	wantNewlines := "line1\nline2\r\nline3"
	assertValue(t, resp.Values, "NEWLINES", &wantNewlines)
}

func TestResolve_AcceptsWithoutProviderField(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "K", "V")

	revert := pipeStdin(t, `{"protocolVersion":1,"ids":["K"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "V"
	assertValue(t, resp.Values, "K", &want)
}

func TestResolve_TimestampIsIgnored(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "K", "V")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["K"],"timestamp":"2026-06-17T12:00:00Z"}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "V"
	assertValue(t, resp.Values, "K", &want)
}

func TestResolve_OutputIsSingleLine(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "A", "1")
	store.Set("default", "B", "2")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["A","B"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 1 {
		t.Errorf("got %d lines, want exactly 1 line of JSON", len(lines))
	}
}

func TestResolve_EmptyValue(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "EMPTY", "")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["EMPTY"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := ""
	assertValue(t, resp.Values, "EMPTY", &want)
}

func TestResolve_DuplicateIDs(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "X", "42")

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["X","X","X"]}`)
	defer revert()

	stdout, err := executeResolve(app, "resolve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := parseResolve(t, stdout)
	want := "42"
	assertValue(t, resp.Values, "X", &want)
}

func TestResolve_ProtectedKeyInNonexistentVault(t *testing.T) {
	app, store, _, _, _ := newAuthorizedTestAppWithBulk()
	store.SetWithProtection("default", "S", "val", true)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["S"]}`)
	defer revert()

	_, err := executeResolve(app, "resolve", "--vault", "nope")
	if err == nil {
		t.Fatal("expected error for non-existent vault")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestResolve_ProtectedKeyHidesValueFromError(t *testing.T) {
	app, store, _, _, authorizer := newAuthorizedTestAppWithBulk()
	authorizer.err = fmt.Errorf("touchid declined")
	store.SetWithProtection("default", "API_SECRET", "super-secret-key-123", true)

	revert := pipeStdin(t, `{"protocolVersion":1,"provider":"kc","ids":["API_SECRET"]}`)
	defer revert()

	_, err := executeResolve(app, "resolve")
	if err == nil {
		t.Fatal("expected error when auth denied")
	}
	if strings.Contains(err.Error(), "super-secret-key-123") {
		t.Errorf("error message = %q, must not contain secret value", err.Error())
	}
	if !strings.Contains(err.Error(), "touchid declined") {
		t.Errorf("error = %q, want it to mention auth denial reason", err.Error())
	}
}

type readAllBuf struct {
	data []byte
}

func (b *readAllBuf) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *readAllBuf) String() string {
	return string(b.data)
}
