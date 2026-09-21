package vault

import (
	"errors"
	"strings"
	"testing"
)

func seedVault(t *testing.T, mgr *Manager, name string, entries map[string]string) {
	t.Helper()
	if name != DefaultVault {
		if err := mgr.Create(name); err != nil && !errors.Is(err, ErrAlreadyExists) {
			t.Fatal(err)
		}
	}
	for key, value := range entries {
		if err := mgr.SetWithProtection(key, value, name, true); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRenameVault_MovesKeysProtectionAndMetadata(t *testing.T) {
	mgr, kc := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "abc", "DB_PASS": "xyz"})
	if err := mgr.SetWithProtection("PLAIN", "p", "staging", false); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetVaultInfo("staging", func(info *Info) { info.Description = "Staging env" }); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RenameVault("staging", "stage2"); err != nil {
		t.Fatalf("RenameVault: %v", err)
	}

	vaults, _ := mgr.ListVaults()
	if strings.Join(vaults, ",") != "default,stage2" {
		t.Fatalf("ListVaults = %v", vaults)
	}
	if value, err := mgr.Get("API_KEY", "stage2"); err != nil || value != "abc" {
		t.Fatalf("Get after rename = %q, %v", value, err)
	}
	if len(kc.store[ServiceName("staging")]) != 0 {
		t.Fatalf("old service still holds %v", kc.store[ServiceName("staging")])
	}
	if kc.protected[ServiceName("stage2")]["PLAIN"] {
		t.Fatal("protection flag was not preserved across rename")
	}
	info, err := mgr.VaultInfo("stage2")
	if err != nil {
		t.Fatal(err)
	}
	if info.Description != "Staging env" {
		t.Fatalf("metadata lost on rename: %+v", info)
	}
}

func TestRenameVault_MovesHistory(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "v1"})
	if err := mgr.SetWithProtection("API_KEY", "v2", "staging", true); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RenameVault("staging", "stage2"); err != nil {
		t.Fatal(err)
	}

	versions, err := mgr.History().Versions("stage2", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("history after rename = %d versions, want 1", len(versions))
	}
}

func TestRenameVault_UpdatesActiveVault(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"A": "1"})
	if err := mgr.Switch("staging"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RenameVault("staging", "stage2"); err != nil {
		t.Fatal(err)
	}
	if got := mgr.ActiveVault(); got != "stage2" {
		t.Fatalf("ActiveVault = %q, want stage2", got)
	}
}

func TestRenameVault_RejectsDefaultAndCollisions(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"A": "1"})
	seedVault(t, mgr, "prod", map[string]string{"B": "2"})

	if err := mgr.RenameVault(DefaultVault, "other"); !errors.Is(err, ErrDefaultVault) {
		t.Fatalf("renaming default returned %v", err)
	}
	if err := mgr.RenameVault("staging", "prod"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("collision returned %v", err)
	}
	if err := mgr.RenameVault("ghost", "other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown vault returned %v", err)
	}
	if err := mgr.RenameVault("staging", "bad name"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("invalid name returned %v", err)
	}
}

func TestCloneVault_CopiesValuesAndProtectionLeavingSourceIntact(t *testing.T) {
	mgr, kc := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "abc"})
	if err := mgr.SetWithProtection("PLAIN", "p", "prod", false); err != nil {
		t.Fatal(err)
	}

	count, err := mgr.CloneVault("prod", "sandbox")
	if err != nil {
		t.Fatalf("CloneVault: %v", err)
	}
	if count != 2 {
		t.Fatalf("cloned %d keys, want 2", count)
	}
	if value, err := mgr.Get("API_KEY", "sandbox"); err != nil || value != "abc" {
		t.Fatalf("clone value = %q, %v", value, err)
	}
	if value, err := mgr.Get("API_KEY", "prod"); err != nil || value != "abc" {
		t.Fatalf("source damaged: %q, %v", value, err)
	}
	if kc.protected[ServiceName("sandbox")]["PLAIN"] {
		t.Fatal("clone upgraded an unprotected secret")
	}
	if !kc.protected[ServiceName("sandbox")]["API_KEY"] {
		t.Fatal("clone dropped protection")
	}
}

func TestCloneVault_RejectsExistingDestination(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"A": "1"})
	seedVault(t, mgr, "sandbox", nil)

	if _, err := mgr.CloneVault("prod", "sandbox"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
}

func TestMoveKey_MovesValueProtectionAndHistory(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "v1"})
	if err := mgr.SetWithProtection("API_KEY", "v2", "staging", true); err != nil {
		t.Fatal(err)
	}
	seedVault(t, mgr, "prod", nil)

	if err := mgr.MoveKey("API_KEY", "staging", "prod", "", false); err != nil {
		t.Fatalf("MoveKey: %v", err)
	}

	if value, err := mgr.Get("API_KEY", "prod"); err != nil || value != "v2" {
		t.Fatalf("destination value = %q, %v", value, err)
	}
	if _, err := mgr.Get("API_KEY", "staging"); err == nil {
		t.Fatal("source key survived a move")
	}
	versions, err := mgr.History().Versions("prod", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("history did not follow the move: %d versions", len(versions))
	}
}

func TestMoveKey_CopyLeavesSourceInPlace(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "v1"})
	seedVault(t, mgr, "prod", nil)

	if err := mgr.MoveKey("API_KEY", "staging", "prod", "", true); err != nil {
		t.Fatal(err)
	}
	if value, err := mgr.Get("API_KEY", "staging"); err != nil || value != "v1" {
		t.Fatalf("copy removed the source: %q, %v", value, err)
	}
	if value, err := mgr.Get("API_KEY", "prod"); err != nil || value != "v1" {
		t.Fatalf("copy value = %q, %v", value, err)
	}
}

func TestMoveKey_RenamesWithinTheSameVault(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "prod", map[string]string{"OLD_KEY": "v1"})

	if err := mgr.MoveKey("OLD_KEY", "prod", "prod", "NEW_KEY", false); err != nil {
		t.Fatal(err)
	}
	if value, err := mgr.Get("NEW_KEY", "prod"); err != nil || value != "v1" {
		t.Fatalf("renamed value = %q, %v", value, err)
	}
	if _, err := mgr.Get("OLD_KEY", "prod"); err == nil {
		t.Fatal("old key survived the rename")
	}
}

func TestMoveKey_RefusesToOverwriteWithoutForce(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", map[string]string{"API_KEY": "src"})
	seedVault(t, mgr, "prod", map[string]string{"API_KEY": "dst"})

	err := mgr.MoveKey("API_KEY", "staging", "prod", "", false)
	if !errors.Is(err, ErrKeyExists) {
		t.Fatalf("err = %v, want ErrKeyExists", err)
	}
	if value, _ := mgr.Get("API_KEY", "prod"); value != "dst" {
		t.Fatalf("destination was modified: %q", value)
	}
}

func TestMoveKey_MissingSourceKey(t *testing.T) {
	mgr, _ := newTestManager(t)
	seedVault(t, mgr, "staging", nil)
	seedVault(t, mgr, "prod", nil)

	if err := mgr.MoveKey("GHOST", "staging", "prod", "", false); err == nil {
		t.Fatal("expected error for missing source key")
	}
}
