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

type stubAdmin struct {
	VaultAdmin
	name   string
	source string
	err    error
}

func (s stubAdmin) Context() (string, string, error) { return s.name, s.source, s.err }

type stubSettings struct {
	Settings
	values map[string]string
}

func (s stubSettings) Get(key string) string { return s.values[key] }

// Only a .kc-vault marker is a pin. An active vault chosen by `kc vault switch`
// is not one, and telling the user the directory decided it would be a claim
// about the filesystem that is not true.
func TestPinnedVaultOnlyReportsADirectoryMarker(t *testing.T) {
	cases := []struct {
		source string
		want   string
	}{
		{VaultSourceDir, "acme"},
		{VaultSourceFile, ""},
		{VaultSourceDefault, ""},
		{VaultSourceEnv, ""},
	}
	for _, tc := range cases {
		app := &App{Admin: stubAdmin{name: "acme", source: tc.source}}
		if got := pinnedVault(app); got != tc.want {
			t.Fatalf("source %q → %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestPinnedVaultIsEmptyWithoutAnAdmin(t *testing.T) {
	if got := pinnedVault(&App{}); got != "" {
		t.Fatalf("pinnedVault = %q, want empty", got)
	}
}

// The TUI badge and `kc audit` must agree, or the same key is stale in one
// place and fine in the other.
func TestRotationDaysComesFromTheAuditSetting(t *testing.T) {
	app := &App{Config: stubSettings{values: map[string]string{"audit.rotation_days": "90"}}}
	if got := configuredRotationDays(app); got != 90 {
		t.Fatalf("rotation days = %d, want 90", got)
	}
	if got := configuredRotationDays(&App{}); got != 0 {
		t.Fatalf("rotation days without config = %d, want 0", got)
	}
	unset := &App{Config: stubSettings{values: map[string]string{}}}
	if got := configuredRotationDays(unset); got != 0 {
		t.Fatalf("rotation days when unset = %d, want 0", got)
	}
}
