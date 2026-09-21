package history

import (
	"errors"
	"testing"

	"github.com/v-gutierrez/kc/internal/keychain"
)

type fakeStore struct {
	items     map[string]map[string]string // service -> account -> value
	protected map[string]map[string]bool
	modified  map[string]map[string]string
	setErr    error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		items:     make(map[string]map[string]string),
		protected: make(map[string]map[string]bool),
		modified:  make(map[string]map[string]string),
	}
}

func (f *fakeStore) Get(service, account string) (string, error) {
	svc, ok := f.items[service]
	if !ok {
		return "", keychain.ErrNotFound
	}
	value, ok := svc[account]
	if !ok {
		return "", keychain.ErrNotFound
	}
	return value, nil
}

func (f *fakeStore) SetWithProtection(service, account, password string, protected bool) error {
	if f.setErr != nil {
		return f.setErr
	}
	if f.items[service] == nil {
		f.items[service] = make(map[string]string)
		f.protected[service] = make(map[string]bool)
		f.modified[service] = make(map[string]string)
	}
	f.items[service][account] = password
	f.protected[service][account] = protected
	f.modified[service][account] = "2026-09-21 17:00"
	return nil
}

func (f *fakeStore) Delete(service, account string) error {
	svc, ok := f.items[service]
	if !ok {
		return keychain.ErrNotFound
	}
	if _, ok := svc[account]; !ok {
		return keychain.ErrNotFound
	}
	delete(svc, account)
	return nil
}

func (f *fakeStore) ListMetadata(service string) ([]keychain.ItemMetadata, error) {
	svc, ok := f.items[service]
	if !ok {
		return nil, nil
	}
	items := make([]keychain.ItemMetadata, 0, len(svc))
	for account := range svc {
		items = append(items, keychain.ItemMetadata{
			Account:   account,
			Protected: f.protected[service][account],
			Modified:  f.modified[service][account],
		})
	}
	return items, nil
}

func newRecorder() (*Recorder, *fakeStore) {
	store := newFakeStore()
	return &Recorder{Store: store, Retention: 3}, store
}

func TestServiceName(t *testing.T) {
	if got := ServiceName("prod"); got != "kc:prod:__history__" {
		t.Fatalf("ServiceName = %q", got)
	}
}

func TestSnapshot_StoresUnderHistoryServiceNotLiveVault(t *testing.T) {
	rec, store := newRecorder()
	if err := rec.Snapshot("prod", "API_KEY", "v1", true, 0); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	if _, ok := store.items["kc:prod"]; ok {
		t.Fatal("snapshot wrote into the live vault service")
	}
	svc := store.items[ServiceName("prod")]
	if len(svc) != 1 {
		t.Fatalf("history holds %d items, want 1", len(svc))
	}
	if svc["API_KEY~00001"] != "v1" {
		t.Fatalf("history item = %v, want API_KEY~00001=v1", svc)
	}
	if !store.protected[ServiceName("prod")]["API_KEY~00001"] {
		t.Fatal("snapshot lost the protection flag")
	}
}

func TestSnapshot_IncrementsSequencePerKey(t *testing.T) {
	rec, _ := newRecorder()
	for _, value := range []string{"v1", "v2"} {
		if err := rec.Snapshot("prod", "API_KEY", value, true, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.Snapshot("prod", "OTHER", "o1", true, 0); err != nil {
		t.Fatal(err)
	}

	versions, err := rec.Versions("prod", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
	if versions[0].Seq != 2 || versions[1].Seq != 1 {
		t.Fatalf("versions not newest-first: %+v", versions)
	}

	other, err := rec.Versions("prod", "OTHER")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 1 || other[0].Seq != 1 {
		t.Fatalf("per-key sequence leaked: %+v", other)
	}
}

func TestSnapshot_PrunesToRetention(t *testing.T) {
	rec, _ := newRecorder() // retention 3
	for _, value := range []string{"v1", "v2", "v3", "v4", "v5"} {
		if err := rec.Snapshot("prod", "API_KEY", value, true, 0); err != nil {
			t.Fatal(err)
		}
	}

	versions, err := rec.Versions("prod", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("got %d versions, want 3", len(versions))
	}
	if versions[0].Seq != 5 || versions[2].Seq != 3 {
		t.Fatalf("pruned the wrong end: %+v", versions)
	}

	if _, err := rec.Value("prod", "API_KEY", 1); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("oldest version still readable: %v", err)
	}
	if value, err := rec.Value("prod", "API_KEY", 5); err != nil || value != "v5" {
		t.Fatalf("Value(5) = %q, %v", value, err)
	}
}

func TestSnapshot_RetentionOverride(t *testing.T) {
	rec, _ := newRecorder()
	for _, value := range []string{"v1", "v2", "v3"} {
		if err := rec.Snapshot("prod", "API_KEY", value, true, 1); err != nil {
			t.Fatal(err)
		}
	}
	versions, err := rec.Versions("prod", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Seq != 3 {
		t.Fatalf("override ignored: %+v", versions)
	}
}

func TestSnapshot_ZeroRetentionDisablesHistory(t *testing.T) {
	rec, store := newRecorder()
	rec.Retention = 0
	if err := rec.Snapshot("prod", "API_KEY", "v1", true, 0); err != nil {
		t.Fatal(err)
	}
	if len(store.items[ServiceName("prod")]) != 0 {
		t.Fatalf("retention 0 still wrote history: %v", store.items)
	}
}

func TestVersions_CarriesMetadataAndDigest(t *testing.T) {
	rec, _ := newRecorder()
	if err := rec.Snapshot("prod", "API_KEY", "v1", false, 0); err != nil {
		t.Fatal(err)
	}
	versions, err := rec.Versions("prod", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if versions[0].Recorded != "2026-09-21 17:00" {
		t.Fatalf("Recorded = %q", versions[0].Recorded)
	}
	if versions[0].Protected {
		t.Fatal("Protected should mirror the snapshot flag")
	}
	if versions[0].Digest != keychain.Digest("v1") {
		t.Fatalf("Digest = %q", versions[0].Digest)
	}
}

func TestVersions_KeyWithTildeInName(t *testing.T) {
	rec, _ := newRecorder()
	if err := rec.Snapshot("prod", "WEIRD~KEY", "v1", true, 0); err != nil {
		t.Fatal(err)
	}
	versions, err := rec.Versions("prod", "WEIRD~KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("got %d versions for tilde key, want 1", len(versions))
	}
	if other, _ := rec.Versions("prod", "WEIRD"); len(other) != 0 {
		t.Fatalf("prefix collision: %+v", other)
	}
}

func TestValue_UnknownVersion(t *testing.T) {
	rec, _ := newRecorder()
	if _, err := rec.Value("prod", "API_KEY", 7); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("err = %v, want ErrVersionNotFound", err)
	}
}

func TestKeys_ListsDistinctKeysWithHistory(t *testing.T) {
	rec, _ := newRecorder()
	for _, key := range []string{"B_KEY", "A_KEY", "A_KEY"} {
		if err := rec.Snapshot("prod", key, "value", true, 0); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := rec.Keys("prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "A_KEY" || keys[1] != "B_KEY" {
		t.Fatalf("Keys = %v, want [A_KEY B_KEY]", keys)
	}
}

func TestPurgeKey_RemovesOnlyThatKey(t *testing.T) {
	rec, _ := newRecorder()
	if err := rec.Snapshot("prod", "A", "1", true, 0); err != nil {
		t.Fatal(err)
	}
	if err := rec.Snapshot("prod", "B", "1", true, 0); err != nil {
		t.Fatal(err)
	}

	removed, err := rec.PurgeKey("prod", "A")
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed %d, want 1", removed)
	}
	if versions, _ := rec.Versions("prod", "A"); len(versions) != 0 {
		t.Fatal("A history survived purge")
	}
	if versions, _ := rec.Versions("prod", "B"); len(versions) != 1 {
		t.Fatal("B history was collateral damage")
	}
}

func TestPurgeVault_RemovesEverything(t *testing.T) {
	rec, store := newRecorder()
	if err := rec.Snapshot("prod", "A", "1", true, 0); err != nil {
		t.Fatal(err)
	}
	if err := rec.Snapshot("prod", "B", "1", true, 0); err != nil {
		t.Fatal(err)
	}
	if err := rec.Snapshot("staging", "C", "1", true, 0); err != nil {
		t.Fatal(err)
	}

	removed, err := rec.PurgeVault("prod")
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed %d, want 2", removed)
	}
	if len(store.items[ServiceName("prod")]) != 0 {
		t.Fatal("prod history survived")
	}
	if len(store.items[ServiceName("staging")]) != 1 {
		t.Fatal("staging history was collateral damage")
	}
}

func TestRenameVault_MovesHistory(t *testing.T) {
	rec, _ := newRecorder()
	if err := rec.Snapshot("old", "A", "1", true, 0); err != nil {
		t.Fatal(err)
	}
	if err := rec.Snapshot("old", "A", "2", true, 0); err != nil {
		t.Fatal(err)
	}

	if err := rec.RenameVault("old", "new"); err != nil {
		t.Fatalf("RenameVault: %v", err)
	}
	if versions, _ := rec.Versions("old", "A"); len(versions) != 0 {
		t.Fatal("history left behind in the old vault")
	}
	versions, err := rec.Versions("new", "A")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions after rename, want 2", len(versions))
	}
	if value, err := rec.Value("new", "A", 2); err != nil || value != "2" {
		t.Fatalf("Value after rename = %q, %v", value, err)
	}
}

func TestRenameKey_MovesHistoryWithinVault(t *testing.T) {
	rec, _ := newRecorder()
	if err := rec.Snapshot("prod", "OLD", "1", true, 0); err != nil {
		t.Fatal(err)
	}
	if err := rec.RenameKey("prod", "OLD", "NEW"); err != nil {
		t.Fatalf("RenameKey: %v", err)
	}
	if versions, _ := rec.Versions("prod", "OLD"); len(versions) != 0 {
		t.Fatal("old key history survived")
	}
	if versions, _ := rec.Versions("prod", "NEW"); len(versions) != 1 {
		t.Fatal("history did not follow the rename")
	}
}

func TestSnapshot_SkipsEmptyValue(t *testing.T) {
	rec, store := newRecorder()
	if err := rec.Snapshot("prod", "API_KEY", "", true, 0); err != nil {
		t.Fatal(err)
	}
	if len(store.items[ServiceName("prod")]) != 0 {
		t.Fatal("empty value should not create a version")
	}
}

func TestSnapshot_PropagatesStoreError(t *testing.T) {
	rec, store := newRecorder()
	store.setErr = errors.New("keychain exploded")
	if err := rec.Snapshot("prod", "API_KEY", "v1", true, 0); err == nil {
		t.Fatal("expected error from store")
	}
}
