package vault

import (
	"errors"
	"testing"
	"time"
)

func TestDeleteVault_ArchivesKeysInsteadOfDestroyingThem(t *testing.T) {
	mgr, kc := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "abc"})
	if err := mgr.SetWithProtection("PLAIN", "p", "staging", false); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetVaultInfo("staging", func(info *Info) { info.Description = "Staging" }); err != nil {
		t.Fatal(err)
	}

	if err := mgr.DeleteVaultWithOptions("staging", DeleteOptions{Force: true}); err != nil {
		t.Fatalf("DeleteVaultWithOptions: %v", err)
	}

	if len(kc.store[ServiceName("staging")]) != 0 {
		t.Fatal("live service still holds keys")
	}
	vaults, _ := mgr.ListVaults()
	for _, v := range vaults {
		if v == "staging" {
			t.Fatal("deleted vault still listed")
		}
	}

	archived, err := mgr.ListArchived()
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 {
		t.Fatalf("got %d archived vaults, want 1", len(archived))
	}
	if archived[0].Name != "staging" || archived[0].Keys != 2 {
		t.Fatalf("archive record = %+v", archived[0])
	}
	if archived[0].DeletedAt.IsZero() {
		t.Fatal("archive record has no timestamp")
	}
}

func TestRestoreVault_BringsBackKeysProtectionAndMetadata(t *testing.T) {
	mgr, kc := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "abc"})
	if err := mgr.SetWithProtection("PLAIN", "p", "staging", false); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetVaultInfo("staging", func(info *Info) {
		info.Description = "Staging"
		info.RequireProtection = true
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteVaultWithOptions("staging", DeleteOptions{Force: true}); err != nil {
		t.Fatal(err)
	}

	count, err := mgr.RestoreVault("staging")
	if err != nil {
		t.Fatalf("RestoreVault: %v", err)
	}
	if count != 2 {
		t.Fatalf("restored %d keys, want 2", count)
	}
	if value, err := mgr.Get("API_KEY", "staging"); err != nil || value != "abc" {
		t.Fatalf("restored value = %q, %v", value, err)
	}
	if kc.protected[ServiceName("staging")]["PLAIN"] {
		t.Fatal("restore upgraded an unprotected secret")
	}
	info, err := mgr.VaultInfo("staging")
	if err != nil {
		t.Fatal(err)
	}
	if info.Description != "Staging" || !info.RequireProtection {
		t.Fatalf("restored metadata = %+v", info)
	}
	if archived, _ := mgr.ListArchived(); len(archived) != 0 {
		t.Fatalf("archive record survived restore: %+v", archived)
	}
}

func TestRestoreVault_RefusesWhenNameIsTakenAgain(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"A": "1"})
	if err := mgr.DeleteVaultWithOptions("staging", DeleteOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Create("staging"); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.RestoreVault("staging"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
}

func TestRestoreVault_UnknownArchive(t *testing.T) {
	mgr, _ := newTestManager(t)
	if _, err := mgr.RestoreVault("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteVault_PurgeSkipsTheArchiveAndDropsHistory(t *testing.T) {
	mgr, kc := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "v1"})
	if err := mgr.SetWithProtection("API_KEY", "v2", "staging", true); err != nil {
		t.Fatal(err)
	}

	if err := mgr.DeleteVaultWithOptions("staging", DeleteOptions{Force: true, Purge: true}); err != nil {
		t.Fatal(err)
	}

	if archived, _ := mgr.ListArchived(); len(archived) != 0 {
		t.Fatalf("purge still archived: %+v", archived)
	}
	if len(kc.store[archiveServiceName("staging")]) != 0 {
		t.Fatal("purge left archive items behind")
	}
	versions, _ := mgr.History().Versions("staging", "API_KEY")
	if len(versions) != 0 {
		t.Fatalf("purge left %d history versions behind", len(versions))
	}
}

func TestDeleteVault_SoftDeleteKeepsHistoryForRestore(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "v1"})
	if err := mgr.SetWithProtection("API_KEY", "v2", "staging", true); err != nil {
		t.Fatal(err)
	}

	if err := mgr.DeleteVaultWithOptions("staging", DeleteOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.RestoreVault("staging"); err != nil {
		t.Fatal(err)
	}

	versions, _ := mgr.History().Versions("staging", "API_KEY")
	if len(versions) != 1 {
		t.Fatalf("history lost across archive/restore: %d versions", len(versions))
	}
}

func TestPurgeArchived_RemovesItPermanently(t *testing.T) {
	mgr, kc := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "v1"})
	if err := mgr.DeleteVaultWithOptions("staging", DeleteOptions{Force: true}); err != nil {
		t.Fatal(err)
	}

	removed, err := mgr.PurgeArchived("staging")
	if err != nil {
		t.Fatalf("PurgeArchived: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed %d keys, want 1", removed)
	}
	if len(kc.store[archiveServiceName("staging")]) != 0 {
		t.Fatal("archive items survived purge")
	}
	if archived, _ := mgr.ListArchived(); len(archived) != 0 {
		t.Fatal("archive record survived purge")
	}
}

func TestGCArchived_DropsOnlyExpiredEntries(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "old", map[string]string{"A": "1"})
	seedVault(t, mgr, "fresh", map[string]string{"B": "1"})
	if err := mgr.DeleteVaultWithOptions("old", DeleteOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteVaultWithOptions("fresh", DeleteOptions{Force: true}); err != nil {
		t.Fatal(err)
	}

	records, err := mgr.loadArchive()
	if err != nil {
		t.Fatal(err)
	}
	for i := range records {
		if records[i].Name == "old" {
			records[i].DeletedAt = time.Now().AddDate(0, 0, -45)
		}
	}
	if err := mgr.saveArchive(records); err != nil {
		t.Fatal(err)
	}

	expired, err := mgr.GCArchived(30)
	if err != nil {
		t.Fatalf("GCArchived: %v", err)
	}
	if len(expired) != 1 || expired[0] != "old" {
		t.Fatalf("expired = %v, want [old]", expired)
	}
	remaining, _ := mgr.ListArchived()
	if len(remaining) != 1 || remaining[0].Name != "fresh" {
		t.Fatalf("remaining = %+v", remaining)
	}
}

func TestGCArchived_ZeroDaysKeepsEverything(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"A": "1"})
	if err := mgr.DeleteVaultWithOptions("staging", DeleteOptions{Force: true}); err != nil {
		t.Fatal(err)
	}

	expired, err := mgr.GCArchived(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 0 {
		t.Fatalf("retention 0 expired %v", expired)
	}
}

func TestDeleteVault_LegacySignatureStillArchives(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"A": "1"})

	if err := mgr.DeleteVault("staging", true); err != nil {
		t.Fatal(err)
	}
	archived, _ := mgr.ListArchived()
	if len(archived) != 1 {
		t.Fatalf("legacy delete did not archive: %+v", archived)
	}
}
