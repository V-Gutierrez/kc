// Package app wires the storage layer to the CLI.
//
// The adapters live here rather than in main so the whole assembled
// application — CLI commands, vault manager, history recorder, config — can be
// exercised end to end by tests against an in-memory Keychain.
package app

import (
	"github.com/v-gutierrez/kc/internal/auth"
	"github.com/v-gutierrez/kc/internal/cli"
	"github.com/v-gutierrez/kc/internal/config"
	"github.com/v-gutierrez/kc/internal/vault"
)

// Deps are the pieces main (or a test) supplies.
type Deps struct {
	Vaults    *vault.Manager
	Clipboard cli.Clipboard
	Auth      auth.Authorizer
	Runner    cli.CommandRunner
}

// Build assembles the CLI application around a vault manager.
func Build(deps Deps) (*cli.App, error) {
	vm := deps.Vaults
	settings, err := vm.Settings()
	if err != nil {
		return nil, err
	}

	return &cli.App{
		Store:     &storeAdapter{vm: vm},
		Bulk:      &bulkAdapter{vm: vm},
		Vaults:    &vaultAdapter{vm: vm},
		Clipboard: deps.Clipboard,
		Auth:      deps.Auth,
		Runner:    deps.Runner,
		Admin:     &adminAdapter{vm: vm},
		History:   &historyAdapter{vm: vm},
		Config:    &settingsAdapter{cfg: settings},
	}, nil
}

// storeAdapter bridges vault.Manager to the cli.KeychainStore interface.
type storeAdapter struct {
	vm *vault.Manager
}

func (s *storeAdapter) Get(vaultName, key string) (string, error) {
	return s.vm.Get(key, vaultName)
}

func (s *storeAdapter) Set(vaultName, key, value string) error {
	return s.vm.Set(key, value, vaultName)
}

func (s *storeAdapter) SetWithProtection(vaultName, key, value string, protected bool) error {
	return s.vm.SetWithProtection(key, value, vaultName, protected)
}

func (s *storeAdapter) Delete(vaultName, key string) error {
	return s.vm.Delete(key, vaultName)
}

func (s *storeAdapter) List(vaultName string) ([]string, error) {
	return s.vm.ListKeys(vaultName)
}

func (s *storeAdapter) ListMetadata(vaultName string) ([]cli.SecretMetadata, error) {
	items, err := s.vm.ListKeyMetadata(vaultName)
	if err != nil {
		return nil, err
	}
	result := make([]cli.SecretMetadata, 0, len(items))
	for _, item := range items {
		result = append(result, cli.SecretMetadata{Key: item.Key, Vault: vaultName, Protection: item.Protection, Modified: item.Modified})
	}
	return result, nil
}

func (s *storeAdapter) ProtectAll(vaultName string) (int, error) {
	return s.vm.ProtectAllKeys(vaultName)
}

// vaultAdapter bridges vault.Manager to the cli.VaultManager interface.
type vaultAdapter struct {
	vm *vault.Manager
}

func (v *vaultAdapter) List() ([]string, error) {
	return v.vm.ListVaults()
}

func (v *vaultAdapter) Create(name string) error {
	return v.vm.Create(name)
}

func (v *vaultAdapter) Active() (string, error) {
	return v.vm.ActiveVault(), nil
}

func (v *vaultAdapter) Switch(name string) error {
	return v.vm.Switch(name)
}

func (v *vaultAdapter) Delete(name string, force bool) error {
	return v.vm.DeleteVault(name, force)
}

// bulkAdapter bridges vault.Manager to the cli.BulkStore interface.
type bulkAdapter struct {
	vm *vault.Manager
}

func (b *bulkAdapter) Get(vaultName, key string) (string, error) {
	return b.vm.Get(key, vaultName)
}

func (b *bulkAdapter) Set(vaultName, key, value string) error {
	return b.vm.Set(key, value, vaultName)
}

func (b *bulkAdapter) SetWithProtection(vaultName, key, value string, protected bool) error {
	return b.vm.SetWithProtection(key, value, vaultName, protected)
}

func (b *bulkAdapter) Delete(vaultName, key string) error {
	return b.vm.Delete(key, vaultName)
}

func (b *bulkAdapter) List(vaultName string) ([]string, error) {
	return b.vm.ListKeys(vaultName)
}

func (b *bulkAdapter) ListMetadata(vaultName string) ([]cli.SecretMetadata, error) {
	items, err := b.vm.ListKeyMetadata(vaultName)
	if err != nil {
		return nil, err
	}
	result := make([]cli.SecretMetadata, 0, len(items))
	for _, item := range items {
		result = append(result, cli.SecretMetadata{Key: item.Key, Vault: vaultName, Protection: item.Protection, Modified: item.Modified})
	}
	return result, nil
}

func (b *bulkAdapter) ProtectAll(vaultName string) (int, error) {
	return b.vm.ProtectAllKeys(vaultName)
}

func (b *bulkAdapter) BulkSet(entries map[string]string, vaultName string) (int, error) {
	return b.vm.BulkSet(entries, vaultName)
}

func (b *bulkAdapter) BulkSetWithProtection(entries map[string]string, vaultName string, protected bool) (int, error) {
	return b.vm.BulkSetWithProtection(entries, vaultName, protected)
}

func (b *bulkAdapter) GetAll(vaultName string) (map[string]string, error) {
	return b.vm.GetAllKeys(vaultName)
}

func (b *bulkAdapter) ReadRawService(service string) (map[string]string, error) {
	return b.vm.ReadRawService(service)
}

// SetWithOptions implements cli.OptionStore so `kc set` can skip or bound history.
func (s *storeAdapter) SetWithOptions(vaultName, key, value string, protected, skipHistory bool, retention int) error {
	return s.vm.SetWithOptions(key, value, vaultName, vault.SetOptions{
		Protected:   protected,
		SkipHistory: skipHistory,
		Retention:   retention,
	})
}

// adminAdapter bridges vault.Manager to the cli.VaultAdmin interface.
type adminAdapter struct {
	vm *vault.Manager
}

func (a *adminAdapter) Rename(oldName, newName string) error {
	return a.vm.RenameVault(oldName, newName)
}

func (a *adminAdapter) Clone(src, dst string) (int, error) {
	return a.vm.CloneVault(src, dst)
}

func (a *adminAdapter) Infos() ([]cli.VaultInfo, error) {
	infos, err := a.vm.ListVaultInfos()
	if err != nil {
		return nil, err
	}
	result := make([]cli.VaultInfo, 0, len(infos))
	for _, info := range infos {
		result = append(result, toCLIInfo(info))
	}
	return result, nil
}

func (a *adminAdapter) Info(name string) (cli.VaultInfo, error) {
	info, err := a.vm.VaultInfo(name)
	if err != nil {
		return cli.VaultInfo{}, err
	}
	return toCLIInfo(info), nil
}

func (a *adminAdapter) Describe(name string, description *string, tags *[]string) error {
	return a.vm.SetVaultInfo(name, func(info *vault.Info) {
		if description != nil {
			info.Description = *description
		}
		if tags != nil {
			info.Tags = append([]string(nil), (*tags)...)
		}
	})
}

func (a *adminAdapter) RequireProtection(name string, require bool) error {
	return a.vm.SetVaultInfo(name, func(info *vault.Info) {
		info.RequireProtection = require
	})
}

func (a *adminAdapter) DeleteWithOptions(name string, force, purge bool) error {
	return a.vm.DeleteVaultWithOptions(name, vault.DeleteOptions{Force: force, Purge: purge})
}

func (a *adminAdapter) ListArchived() ([]cli.ArchivedVault, error) {
	records, err := a.vm.ListArchived()
	if err != nil {
		return nil, err
	}
	result := make([]cli.ArchivedVault, 0, len(records))
	for _, record := range records {
		result = append(result, cli.ArchivedVault{
			Name:      record.Name,
			DeletedAt: record.DeletedAt,
			Keys:      record.Keys,
			Info:      toCLIInfo(record.Info),
		})
	}
	return result, nil
}

func (a *adminAdapter) Restore(name string) (int, error) {
	return a.vm.RestoreVault(name)
}

func (a *adminAdapter) PurgeArchived(name string) (int, error) {
	return a.vm.PurgeArchived(name)
}

func (a *adminAdapter) GCArchived(days int) ([]string, error) {
	return a.vm.GCArchived(days)
}

func (a *adminAdapter) ArchiveRetentionDays() int {
	return a.vm.ArchiveRetentionDays()
}

func (a *adminAdapter) Context() (string, string, error) {
	return a.vm.ActiveVaultContext()
}

func (a *adminAdapter) UseDir(name, dir string) error {
	return a.vm.UseDir(name, dir)
}

func (a *adminAdapter) ClearDir(dir string) error {
	return a.vm.ClearDir(dir)
}

func (a *adminAdapter) MoveKey(key, from, to, newKey string, copyOnly, force bool) error {
	return a.vm.MoveKeyWithOptions(key, from, to, newKey, vault.MoveOptions{Copy: copyOnly, Force: force})
}

func toCLIInfo(info vault.Info) cli.VaultInfo {
	return cli.VaultInfo{
		Name:              info.Name,
		Description:       info.Description,
		Tags:              append([]string(nil), info.Tags...),
		Created:           info.Created,
		RequireProtection: info.RequireProtection,
	}
}

// historyAdapter bridges the version recorder to the cli.HistoryStore interface.
type historyAdapter struct {
	vm *vault.Manager
}

func (h *historyAdapter) Versions(vaultName, key string) ([]cli.HistoryVersion, error) {
	versions, err := h.vm.History().Versions(vaultName, key)
	if err != nil {
		return nil, err
	}
	result := make([]cli.HistoryVersion, 0, len(versions))
	for _, version := range versions {
		result = append(result, cli.HistoryVersion{
			Seq:       version.Seq,
			Key:       version.Key,
			Vault:     version.Vault,
			Recorded:  version.Recorded,
			Protected: version.Protected,
			Digest:    version.Digest,
		})
	}
	return result, nil
}

func (h *historyAdapter) Value(vaultName, key string, seq int) (string, error) {
	return h.vm.History().Value(vaultName, key, seq)
}

func (h *historyAdapter) Keys(vaultName string) ([]string, error) {
	return h.vm.History().Keys(vaultName)
}

func (h *historyAdapter) Rollback(key, vaultName string, seq int) (int, error) {
	return h.vm.Rollback(key, vaultName, seq)
}

func (h *historyAdapter) PurgeKey(vaultName, key string) (int, error) {
	return h.vm.History().PurgeKey(vaultName, key)
}

// settingsAdapter bridges the config file to the cli.Settings interface.
type settingsAdapter struct {
	cfg *config.Config
}

func (s *settingsAdapter) Get(key string) string       { return s.cfg.Get(key) }
func (s *settingsAdapter) All() map[string]string      { return s.cfg.All() }
func (s *settingsAdapter) Set(key, value string) error { return s.cfg.Set(key, value) }
func (s *settingsAdapter) Unset(key string) error      { return s.cfg.Unset(key) }
func (s *settingsAdapter) Path() string                { return s.cfg.Path() }
func (s *settingsAdapter) Known(key string) bool       { return config.Known(key) }
func (s *settingsAdapter) Keys() []string              { return config.Keys() }
func (s *settingsAdapter) Describe(key string) string  { return config.Describe(key) }
