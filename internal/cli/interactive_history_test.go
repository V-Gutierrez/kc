package cli

import (
	"errors"
	"testing"
)

type recordingHistory struct {
	rollbackKey   string
	rollbackVault string
	rollbackSeq   int
	versions      []HistoryVersion
	value         string
	err           error
}

func (r *recordingHistory) Versions(vault, key string) ([]HistoryVersion, error) {
	return r.versions, r.err
}

func (r *recordingHistory) Value(vault, key string, seq int) (string, error) {
	return r.value, r.err
}

func (r *recordingHistory) Keys(vault string) ([]string, error) { return nil, nil }

func (r *recordingHistory) Rollback(key, vault string, seq int) (int, error) {
	r.rollbackKey = key
	r.rollbackVault = vault
	r.rollbackSeq = seq
	return seq, r.err
}

func (r *recordingHistory) PurgeKey(vault, key string) (int, error) { return 0, nil }

// The TUI calls (vault, key); HistoryStore takes (key, vault). Swapping them
// would restore the wrong secret — or silently restore nothing — so the
// adapter's argument order is worth pinning down.
func TestTUIHistoryAdapterRollbackArgumentOrder(t *testing.T) {
	backend := &recordingHistory{}
	adapter := tuiHistoryAdapter{store: backend}

	if err := adapter.Rollback("prod", "STRIPE_TOKEN", 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.rollbackVault != "prod" {
		t.Fatalf("vault = %q, want %q", backend.rollbackVault, "prod")
	}
	if backend.rollbackKey != "STRIPE_TOKEN" {
		t.Fatalf("key = %q, want %q", backend.rollbackKey, "STRIPE_TOKEN")
	}
	if backend.rollbackSeq != 3 {
		t.Fatalf("seq = %d, want 3", backend.rollbackSeq)
	}
}

func TestTUIHistoryAdapterSurfacesRollbackError(t *testing.T) {
	adapter := tuiHistoryAdapter{store: &recordingHistory{err: errors.New("keychain busy")}}
	if err := adapter.Rollback("prod", "TOKEN", 1); err == nil {
		t.Fatal("expected the backend error to surface")
	}
}

// Versions must carry the digest across, not the value: the TUI renders it.
func TestTUIHistoryAdapterMapsVersions(t *testing.T) {
	backend := &recordingHistory{versions: []HistoryVersion{
		{Seq: 2, Recorded: "2026-09-20 11:00", Protected: true, Digest: "deadbeef1234"},
	}}
	adapter := tuiHistoryAdapter{store: backend}

	got, err := adapter.Versions("prod", "TOKEN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("versions = %#v, want 1", got)
	}
	if got[0].Seq != 2 || got[0].Digest != "deadbeef1234" || !got[0].Protected || got[0].Recorded != "2026-09-20 11:00" {
		t.Fatalf("mapped version = %#v", got[0])
	}
}
