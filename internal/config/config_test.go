package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Int(HistoryRetention); got != 5 {
		t.Fatalf("history.retention = %d, want 5", got)
	}
	if !cfg.Bool(HistoryEnabled) {
		t.Fatalf("history.enabled = false, want true")
	}
	if got := cfg.Int(ArchiveRetentionDays); got != 30 {
		t.Fatalf("archive.retention_days = %d, want 30", got)
	}
	if got := cfg.Int(AuditRotationDays); got != 180 {
		t.Fatalf("audit.rotation_days = %d, want 180", got)
	}
}

func TestLoad_ReadsFileOverridingDefaults(t *testing.T) {
	dir := t.TempDir()
	body := "# kc config\nhistory.retention = 12\nhistory.enabled=false\n\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Int(HistoryRetention); got != 12 {
		t.Fatalf("history.retention = %d, want 12", got)
	}
	if cfg.Bool(HistoryEnabled) {
		t.Fatalf("history.enabled = true, want false")
	}
}

func TestSet_PersistsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Set(HistoryRetention, "9"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Int(HistoryRetention); got != 9 {
		t.Fatalf("history.retention = %d, want 9", got)
	}

	info, err := os.Stat(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config perm = %o, want 600", perm)
	}
}

func TestSet_RejectsUnknownKey(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Set("nope.at.all", "1"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestSet_RejectsInvalidValue(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Set(HistoryRetention, "-3"); err == nil {
		t.Fatal("expected error for negative retention")
	}
	if err := cfg.Set(HistoryRetention, "abc"); err == nil {
		t.Fatal("expected error for non-numeric retention")
	}
	if err := cfg.Set(HistoryEnabled, "maybe"); err == nil {
		t.Fatal("expected error for non-boolean flag")
	}
}

func TestAll_MergesDefaultsWithOverrides(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Set(AuditRotationDays, "90"); err != nil {
		t.Fatal(err)
	}

	all := cfg.All()
	if all[AuditRotationDays] != "90" {
		t.Fatalf("audit.rotation_days = %q, want 90", all[AuditRotationDays])
	}
	if all[HistoryRetention] != "5" {
		t.Fatalf("history.retention = %q, want default 5", all[HistoryRetention])
	}
	if len(all) != len(Defaults()) {
		t.Fatalf("All() returned %d keys, want %d", len(all), len(Defaults()))
	}
}
