package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type bootCapableAuthorizer struct {
	fakeAuthorizer
}

func (bootCapableAuthorizer) bootSessionEnabled() bool {
	return true
}

func TestSessionUsesBootSessionTokenBeforeAuthorizer(t *testing.T) {
	configureBootTestEnv(t)

	authorizer := &bootCapableAuthorizer{}
	session := NewSession(authorizer)

	if err := writeBootSessionToken(); err != nil {
		t.Fatalf("writeBootSessionToken error: %v", err)
	}

	if err := session.Authorize("Unlock kc secrets"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if authorizer.calls != 0 {
		t.Fatalf("authorizer calls = %d, want 0", authorizer.calls)
	}
	if !session.approved {
		t.Fatal("session should be approved after valid boot token")
	}
}

func TestSessionWritesBootSessionTokenAfterAuthorizeSuccess(t *testing.T) {
	configureBootTestEnv(t)

	authorizer := &bootCapableAuthorizer{}
	session := NewSession(authorizer)

	if err := session.Authorize("Unlock kc secrets"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}

	tokenPath, err := bootSessionTokenPath()
	if err != nil {
		t.Fatalf("bootSessionTokenPath error: %v", err)
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "12345" {
		t.Fatalf("token contents = %q, want %q", got, "12345")
	}

	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("token mode = %#o, want %#o", got, 0o600)
	}
}

func TestSessionDoesNotCacheFailuresInBootToken(t *testing.T) {
	configureBootTestEnv(t)

	authorizer := &bootCapableAuthorizer{fakeAuthorizer: fakeAuthorizer{err: errors.New("denied")}}
	session := NewSession(authorizer)

	if err := session.Authorize("Unlock kc secrets"); err == nil {
		t.Fatal("expected authorize error")
	}

	tokenPath, err := bootSessionTokenPath()
	if err != nil {
		t.Fatalf("bootSessionTokenPath error: %v", err)
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat error = %v, want not exist", err)
	}
}

func TestIsBootSessionValidDeletesStaleToken(t *testing.T) {
	configureBootTestEnv(t)

	tokenPath, err := bootSessionTokenPath()
	if err != nil {
		t.Fatalf("bootSessionTokenPath error: %v", err)
	}
	if err := os.WriteFile(tokenPath, []byte("99999\n"), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	valid := isBootSessionValid()
	if valid {
		t.Fatal("expected stale boot token to be invalid")
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat error = %v, want not exist", err)
	}
}

func TestIsBootSessionValidReturnsFalseForMalformedToken(t *testing.T) {
	configureBootTestEnv(t)

	tokenPath, err := bootSessionTokenPath()
	if err != nil {
		t.Fatalf("bootSessionTokenPath error: %v", err)
	}
	if err := os.WriteFile(tokenPath, []byte("not-a-boot-time\n"), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if isBootSessionValid() {
		t.Fatal("expected malformed boot token to be invalid")
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat error = %v, want not exist", err)
	}
}

func configureBootTestEnv(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	previousTmpDir := bootSessionTempDir
	previousUID := bootSessionUID
	previousBootTime := bootTimeValue
	t.Cleanup(func() {
		bootSessionTempDir = previousTmpDir
		bootSessionUID = previousUID
		bootTimeValue = previousBootTime
	})

	bootSessionTempDir = func() string {
		return tempDir
	}
	bootSessionUID = func() int {
		return 501
	}
	bootTimeValue = func() (string, error) {
		return "12345", nil
	}
}

func TestBootSessionTokenPathUsesTmpDirAndUID(t *testing.T) {
	configureBootTestEnv(t)

	tokenPath, err := bootSessionTokenPath()
	if err != nil {
		t.Fatalf("bootSessionTokenPath error: %v", err)
	}
	if want := filepath.Join(bootSessionTempDir(), "kc-session-501"); tokenPath != want {
		t.Fatalf("tokenPath = %q, want %q", tokenPath, want)
	}
}
