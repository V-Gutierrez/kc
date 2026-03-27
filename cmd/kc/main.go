package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/v-gutierrez/kc/internal/auth"
	"github.com/v-gutierrez/kc/internal/cli"
	"github.com/v-gutierrez/kc/internal/clipboard"
	"github.com/v-gutierrez/kc/internal/keychain"
	"github.com/v-gutierrez/kc/internal/vault"
)

func main() {
	handled, err := clipboard.RunClearIfRequested()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if handled {
		return
	}

	kc := keychain.New()
	vm := vault.New(kc)
	cb := clipboard.New()

	app := &cli.App{
		Store:     &storeAdapter{vm: vm},
		Bulk:      &bulkAdapter{vm: vm},
		Vaults:    &vaultAdapter{vm: vm},
		Clipboard: cb,
		Auth:      auth.NewTouchIDAuthorizer(),
		Runner:    execRunner,
	}

	root := cli.NewRootCmd(app)
	if err := root.Execute(); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execRunner(name string, args []string, env []string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
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
		result = append(result, cli.SecretMetadata{Key: item.Key, Vault: vaultName, Protection: item.Protection})
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
		result = append(result, cli.SecretMetadata{Key: item.Key, Vault: vaultName, Protection: item.Protection})
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
