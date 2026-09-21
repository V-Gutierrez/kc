package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActiveVault_FallsBackToFileWhenNoDirMarker(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.WorkDir = t.TempDir()
	seedVault(t, mgr, "prod", nil)
	if err := mgr.Switch("prod"); err != nil {
		t.Fatal(err)
	}

	name, source, err := mgr.ActiveVaultContext()
	if err != nil {
		t.Fatal(err)
	}
	if name != "prod" || source != SourceFile {
		t.Fatalf("resolved %q from %q, want prod from file", name, source)
	}
}

func TestActiveVault_DirMarkerWins(t *testing.T) {
	mgr, _ := newTestManager(t)
	dir := t.TempDir()
	mgr.WorkDir = dir
	seedVault(t, mgr, "prod", nil)
	seedVault(t, mgr, "sandbox", nil)
	if err := mgr.Switch("prod"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, DirMarkerFile), []byte("sandbox\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	name, source, err := mgr.ActiveVaultContext()
	if err != nil {
		t.Fatal(err)
	}
	if name != "sandbox" || source != SourceDir {
		t.Fatalf("resolved %q from %q, want sandbox from dir", name, source)
	}
	if got := mgr.ActiveVault(); got != "sandbox" {
		t.Fatalf("ActiveVault = %q", got)
	}
}

func TestActiveVault_DirMarkerFoundInParentDirectory(t *testing.T) {
	mgr, _ := newTestManager(t)
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DirMarkerFile), []byte("sandbox"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr.WorkDir = nested
	seedVault(t, mgr, "sandbox", nil)

	name, source, err := mgr.ActiveVaultContext()
	if err != nil {
		t.Fatal(err)
	}
	if name != "sandbox" || source != SourceDir {
		t.Fatalf("resolved %q from %q", name, source)
	}
}

func TestActiveVault_EnvWinsOverDirMarker(t *testing.T) {
	mgr, _ := newTestManager(t)
	dir := t.TempDir()
	mgr.WorkDir = dir
	seedVault(t, mgr, "sandbox", nil)
	seedVault(t, mgr, "prod", nil)
	if err := os.WriteFile(filepath.Join(dir, DirMarkerFile), []byte("sandbox"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVaultVar, "prod")

	name, source, err := mgr.ActiveVaultContext()
	if err != nil {
		t.Fatal(err)
	}
	if name != "prod" || source != SourceEnv {
		t.Fatalf("resolved %q from %q, want prod from env", name, source)
	}
}

func TestActiveVault_DefaultWhenNothingIsSet(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.WorkDir = t.TempDir()

	name, source, err := mgr.ActiveVaultContext()
	if err != nil {
		t.Fatal(err)
	}
	if name != DefaultVault || source != SourceDefault {
		t.Fatalf("resolved %q from %q", name, source)
	}
}

func TestUseDir_WritesMarkerAndValidatesTheVault(t *testing.T) {
	mgr, _ := newTestManager(t)
	dir := t.TempDir()
	mgr.WorkDir = dir
	seedVault(t, mgr, "sandbox", nil)

	if err := mgr.UseDir("sandbox", dir); err != nil {
		t.Fatalf("UseDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, DirMarkerFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sandbox\n" {
		t.Fatalf("marker = %q", string(data))
	}
	if err := mgr.UseDir("ghost", dir); err == nil {
		t.Fatal("expected error for unknown vault")
	}
}

func TestClearDir_RemovesMarkerAndIsIdempotent(t *testing.T) {
	mgr, _ := newTestManager(t)
	dir := t.TempDir()
	mgr.WorkDir = dir
	seedVault(t, mgr, "sandbox", nil)
	if err := mgr.UseDir("sandbox", dir); err != nil {
		t.Fatal(err)
	}

	if err := mgr.ClearDir(dir); err != nil {
		t.Fatalf("ClearDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, DirMarkerFile)); !os.IsNotExist(err) {
		t.Fatal("marker still present")
	}
	if err := mgr.ClearDir(dir); err != nil {
		t.Fatalf("second ClearDir should be a no-op: %v", err)
	}
}

func TestActiveVaultContext_FlagsAnUnknownVaultInsteadOfHidingIt(t *testing.T) {
	mgr, _ := newTestManager(t)
	dir := t.TempDir()
	mgr.WorkDir = dir
	if err := os.WriteFile(filepath.Join(dir, DirMarkerFile), []byte("ghost"), 0o600); err != nil {
		t.Fatal(err)
	}

	name, source, err := mgr.ActiveVaultContext()
	if err != nil {
		t.Fatal(err)
	}
	if name != "ghost" || source != SourceDir {
		t.Fatalf("resolved %q from %q", name, source)
	}
	if mgr.VaultExists("ghost") {
		t.Fatal("VaultExists lied about an unregistered vault")
	}
}
