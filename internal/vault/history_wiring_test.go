package vault

import (
	"errors"
	"testing"
)

func TestSet_SnapshotsPreviousValue(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "v1"})
	if err := mgr.SetWithProtection("API_KEY", "v2", "prod", true); err != nil {
		t.Fatal(err)
	}

	versions, err := mgr.History().Versions("prod", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("got %d versions, want 1", len(versions))
	}
	value, err := mgr.History().Value("prod", "API_KEY", versions[0].Seq)
	if err != nil {
		t.Fatal(err)
	}
	if value != "v1" {
		t.Fatalf("recorded value = %q, want v1", value)
	}
	if live, _ := mgr.Get("API_KEY", "prod"); live != "v2" {
		t.Fatalf("live value = %q, want v2", live)
	}
}

func TestSet_FirstWriteRecordsNothing(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "v1"})

	versions, err := mgr.History().Versions("prod", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("got %d versions for a brand new key, want 0", len(versions))
	}
}

func TestSet_SkipHistoryOption(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "v1"})

	if err := mgr.SetWithOptions("API_KEY", "v2", "prod", SetOptions{Protected: true, SkipHistory: true}); err != nil {
		t.Fatal(err)
	}
	versions, _ := mgr.History().Versions("prod", "API_KEY")
	if len(versions) != 0 {
		t.Fatalf("SkipHistory still recorded %d versions", len(versions))
	}
}

func TestSet_RetentionOverrideKeepsOnlyRequestedVersions(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "v1"})
	for _, value := range []string{"v2", "v3", "v4"} {
		if err := mgr.SetWithOptions("API_KEY", value, "prod", SetOptions{Protected: true, Retention: 2}); err != nil {
			t.Fatal(err)
		}
	}

	versions, _ := mgr.History().Versions("prod", "API_KEY")
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
}

func TestSet_HistoryDisabledByConfig(t *testing.T) {
	mgr, _ := newTestManager(t)
	settings, err := mgr.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Set("history.enabled", "false"); err != nil {
		t.Fatal(err)
	}

	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "v1"})
	if err := mgr.SetWithProtection("API_KEY", "v2", "prod", true); err != nil {
		t.Fatal(err)
	}
	versions, _ := mgr.History().Versions("prod", "API_KEY")
	if len(versions) != 0 {
		t.Fatalf("history.enabled=false still recorded %d versions", len(versions))
	}
}

func TestDelete_SnapshotsSoRollbackCanBringItBack(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "v1"})

	if err := mgr.Delete("API_KEY", "prod"); err != nil {
		t.Fatal(err)
	}
	versions, err := mgr.History().Versions("prod", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("delete recorded %d versions, want 1", len(versions))
	}
	if value, _ := mgr.History().Value("prod", "API_KEY", versions[0].Seq); value != "v1" {
		t.Fatalf("recorded value = %q, want v1", value)
	}
}

func TestRollback_RestoresAVersionAndRecordsTheCurrentOne(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "v1"})
	if err := mgr.SetWithProtection("API_KEY", "v2", "prod", true); err != nil {
		t.Fatal(err)
	}

	restored, err := mgr.Rollback("API_KEY", "prod", 0)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if restored != 1 {
		t.Fatalf("restored seq %d, want 1", restored)
	}
	if value, _ := mgr.Get("API_KEY", "prod"); value != "v1" {
		t.Fatalf("live value after rollback = %q, want v1", value)
	}

	versions, _ := mgr.History().Versions("prod", "API_KEY")
	if len(versions) != 2 {
		t.Fatalf("rollback left %d versions, want 2 (v1 and the replaced v2)", len(versions))
	}
	if value, _ := mgr.History().Value("prod", "API_KEY", versions[0].Seq); value != "v2" {
		t.Fatalf("rollback did not record the replaced value: %q", value)
	}
}

func TestRollback_RestoresDeletedKey(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "v1"})
	if err := mgr.Delete("API_KEY", "prod"); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.Rollback("API_KEY", "prod", 0); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if value, err := mgr.Get("API_KEY", "prod"); err != nil || value != "v1" {
		t.Fatalf("restored value = %q, %v", value, err)
	}
}

func TestRollback_UnknownVersion(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "v1"})

	if _, err := mgr.Rollback("API_KEY", "prod", 9); err == nil {
		t.Fatal("expected error for unknown version")
	}
	if _, err := mgr.Rollback("API_KEY", "prod", 0); err == nil {
		t.Fatal("expected error when there is no history at all")
	}
}

func TestRequireProtection_RejectsUnprotectedWrites(t *testing.T) {
	mgr, _ := newTestManager(t)
	if err := mgr.CreateWithInfo(Info{Name: "prod", RequireProtection: true}); err != nil {
		t.Fatal(err)
	}

	err := mgr.SetWithProtection("API_KEY", "v1", "prod", false)
	if !errors.Is(err, ErrProtectionRequired) {
		t.Fatalf("err = %v, want ErrProtectionRequired", err)
	}
	if err := mgr.SetWithProtection("API_KEY", "v1", "prod", true); err != nil {
		t.Fatalf("protected write rejected: %v", err)
	}
}

func TestRequireProtection_AppliesToBulkWrites(t *testing.T) {
	mgr, _ := newTestManager(t)
	if err := mgr.CreateWithInfo(Info{Name: "prod", RequireProtection: true}); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.BulkSetWithProtection(map[string]string{"A": "1"}, "prod", false); !errors.Is(err, ErrProtectionRequired) {
		t.Fatalf("err = %v, want ErrProtectionRequired", err)
	}
}

func TestBulkSet_SnapshotsExistingKeysOnly(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"OLD": "v1"})

	if _, err := mgr.BulkSetWithProtection(map[string]string{"OLD": "v2", "NEW": "n1"}, "prod", true); err != nil {
		t.Fatal(err)
	}

	oldVersions, _ := mgr.History().Versions("prod", "OLD")
	if len(oldVersions) != 1 {
		t.Fatalf("OLD history = %d versions, want 1", len(oldVersions))
	}
	newVersions, _ := mgr.History().Versions("prod", "NEW")
	if len(newVersions) != 0 {
		t.Fatalf("NEW history = %d versions, want 0", len(newVersions))
	}
}
