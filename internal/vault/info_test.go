package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListVaults_ReadsLegacyNameOnlyFile(t *testing.T) {
	mgr, _ := newTestManager(t)
	writeVaultsFile(t, mgr, "default\nprod\nstaging\n")

	vaults, err := mgr.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(vaults, ",") != "default,prod,staging" {
		t.Fatalf("ListVaults = %v", vaults)
	}
}

func TestListVaults_ReadsAttributesWithoutLosingNames(t *testing.T) {
	mgr, _ := newTestManager(t)
	writeVaultsFile(t, mgr, "default\nprod\tdesc=Production\ttags=env:prod,core\trequire-protection=true\n")

	vaults, err := mgr.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(vaults, ",") != "default,prod" {
		t.Fatalf("ListVaults = %v", vaults)
	}

	info, err := mgr.VaultInfo("prod")
	if err != nil {
		t.Fatal(err)
	}
	if info.Description != "Production" {
		t.Fatalf("Description = %q", info.Description)
	}
	if strings.Join(info.Tags, ",") != "env:prod,core" {
		t.Fatalf("Tags = %v", info.Tags)
	}
	if !info.RequireProtection {
		t.Fatal("RequireProtection = false, want true")
	}
}

func TestSetVaultInfo_PersistsAndKeepsNameOnlyVaultsLegacy(t *testing.T) {
	mgr, _ := newTestManager(t)
	if err := mgr.Create("prod"); err != nil {
		t.Fatal(err)
	}

	if err := mgr.SetVaultInfo("prod", func(info *Info) {
		info.Description = "Production secrets"
		info.Tags = []string{"env:prod"}
	}); err != nil {
		t.Fatal(err)
	}

	raw := readVaultsFile(t, mgr)
	if !strings.Contains(raw, "prod\tdesc=Production secrets\ttags=env:prod") {
		t.Fatalf("vaults file = %q", raw)
	}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "default" {
			return
		}
	}
	t.Fatalf("plain vault line was rewritten with attributes: %q", raw)
}

func TestVaultInfo_UnknownVault(t *testing.T) {
	mgr, _ := newTestManager(t)
	if _, err := mgr.VaultInfo("ghost"); err == nil {
		t.Fatal("expected ErrNotFound for unknown vault")
	}
}

func TestCreateWithInfo_RecordsCreationDate(t *testing.T) {
	mgr, _ := newTestManager(t)
	if err := mgr.CreateWithInfo(Info{Name: "prod", Description: "Prod", RequireProtection: true}); err != nil {
		t.Fatal(err)
	}
	info, err := mgr.VaultInfo("prod")
	if err != nil {
		t.Fatal(err)
	}
	if info.Created == "" {
		t.Fatal("Created was not recorded")
	}
	if !info.RequireProtection {
		t.Fatal("RequireProtection was not persisted")
	}
}

func TestAttributesSurviveARoundTripWithTabsAndEquals(t *testing.T) {
	mgr, _ := newTestManager(t)
	if err := mgr.Create("prod"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetVaultInfo("prod", func(info *Info) {
		info.Description = "a=b and\tsplit"
	}); err != nil {
		t.Fatal(err)
	}
	info, err := mgr.VaultInfo("prod")
	if err != nil {
		t.Fatal(err)
	}
	if info.Description != "a=b and split" {
		t.Fatalf("Description = %q, want tabs sanitized and equals preserved", info.Description)
	}
}

func writeVaultsFile(t *testing.T, mgr *Manager, body string) {
	t.Helper()
	if err := os.MkdirAll(mgr.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mgr.DataDir, "vaults"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readVaultsFile(t *testing.T, mgr *Manager) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(mgr.DataDir, "vaults"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
