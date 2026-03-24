package auth

import (
	"errors"
	"testing"
)

type fakeAuthorizer struct {
	calls int
	err   error
}

func (f *fakeAuthorizer) Authorize(reason string) error {
	f.calls++
	return f.err
}

func TestSessionAuthorizesOnlyOnce(t *testing.T) {
	authorizer := &fakeAuthorizer{}
	session := NewSession(authorizer)

	if err := session.Authorize("Unlock kc secrets"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := session.Authorize("Unlock kc secrets"); err != nil {
		t.Fatalf("unexpected error on second authorize: %v", err)
	}

	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}
}

func TestSessionDoesNotCacheFailures(t *testing.T) {
	authorizer := &fakeAuthorizer{err: errors.New("denied")}
	session := NewSession(authorizer)

	if err := session.Authorize("Unlock kc secrets"); err == nil {
		t.Fatal("expected authorize error")
	}
	if err := session.Authorize("Unlock kc secrets"); err == nil {
		t.Fatal("expected second authorize error")
	}

	if authorizer.calls != 2 {
		t.Fatalf("authorizer calls = %d, want 2", authorizer.calls)
	}
}
