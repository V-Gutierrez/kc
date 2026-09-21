package vault

import (
	"errors"
	"fmt"
)

// ErrKeyExists is returned when a move or copy would overwrite a key.
var ErrKeyExists = errors.New("vault: key already exists at destination")

// RenameVault moves a vault's keys, protection flags, metadata and recorded
// history to a new name, and follows the rename if it was the active vault.
func (m *Manager) RenameVault(oldName, newName string) error {
	if err := validateName(oldName); err != nil {
		return err
	}
	if err := validateName(newName); err != nil {
		return err
	}
	if oldName == DefaultVault {
		return ErrDefaultVault
	}
	if oldName == newName {
		return nil
	}
	if err := m.requireVault(oldName); err != nil {
		return err
	}
	if m.VaultExists(newName) {
		return ErrAlreadyExists
	}

	info, err := m.VaultInfo(oldName)
	if err != nil {
		return err
	}

	source := ServiceName(oldName)
	target := ServiceName(newName)
	items, err := m.KC.ListMetadata(source)
	if err != nil {
		return fmt.Errorf("vault: read %q: %w", oldName, err)
	}

	info.Name = newName
	if err := m.CreateWithInfo(info); err != nil {
		return err
	}

	for _, item := range items {
		value, err := m.KC.Get(source, item.Account)
		if err != nil {
			return fmt.Errorf("vault: rename read %q: %w", item.Account, err)
		}
		if err := m.KC.SetWithProtection(target, item.Account, value, item.Protected); err != nil {
			return fmt.Errorf("vault: rename write %q: %w", item.Account, err)
		}
		if err := m.KC.Delete(source, item.Account); err != nil {
			return fmt.Errorf("vault: rename drop %q: %w", item.Account, err)
		}
	}

	if err := m.History().RenameVault(oldName, newName); err != nil {
		return err
	}

	infos, err := m.ListVaultInfos()
	if err != nil {
		return err
	}
	remaining := make([]Info, 0, len(infos))
	for _, candidate := range infos {
		if candidate.Name != oldName {
			remaining = append(remaining, candidate)
		}
	}
	if err := m.saveInfos(remaining); err != nil {
		return err
	}

	if m.activeVaultFromFile() == oldName {
		return m.Switch(newName)
	}
	return nil
}

// CloneVault copies every key of src into a brand new vault, preserving each
// secret's protection level. History is not copied: the clone starts fresh.
func (m *Manager) CloneVault(src, dst string) (int, error) {
	if err := validateName(src); err != nil {
		return 0, err
	}
	if err := validateName(dst); err != nil {
		return 0, err
	}
	if err := m.requireVault(src); err != nil {
		return 0, err
	}
	if m.VaultExists(dst) {
		return 0, ErrAlreadyExists
	}

	source := ServiceName(src)
	items, err := m.KC.ListMetadata(source)
	if err != nil {
		return 0, fmt.Errorf("vault: read %q: %w", src, err)
	}

	info, err := m.VaultInfo(src)
	if err != nil {
		return 0, err
	}
	clone := Info{
		Name:              dst,
		Description:       info.Description,
		Tags:              append([]string(nil), info.Tags...),
		RequireProtection: info.RequireProtection,
	}
	if err := m.CreateWithInfo(clone); err != nil {
		return 0, err
	}

	target := ServiceName(dst)
	copied := 0
	for _, item := range items {
		value, err := m.KC.Get(source, item.Account)
		if err != nil {
			return copied, fmt.Errorf("vault: clone read %q: %w", item.Account, err)
		}
		if err := m.KC.SetWithProtection(target, item.Account, value, item.Protected); err != nil {
			return copied, fmt.Errorf("vault: clone write %q: %w", item.Account, err)
		}
		copied++
	}
	return copied, nil
}

// MoveKey moves (or with copyOnly, copies) a single key between vaults,
// optionally renaming it. The secret's protection level travels with it, and on
// a move so does its recorded history. Existing destination keys are never
// overwritten silently — use Force through MoveKeyWithOptions.
func (m *Manager) MoveKey(key, srcVault, dstVault, newKey string, copyOnly bool) error {
	return m.MoveKeyWithOptions(key, srcVault, dstVault, newKey, MoveOptions{Copy: copyOnly})
}

// MoveOptions controls MoveKeyWithOptions.
type MoveOptions struct {
	// Copy leaves the source key in place.
	Copy bool
	// Force allows overwriting an existing destination key.
	Force bool
}

// MoveKeyWithOptions implements MoveKey with explicit overwrite control.
func (m *Manager) MoveKeyWithOptions(key, srcVault, dstVault, newKey string, opts MoveOptions) error {
	src, err := m.resolveVault(srcVault)
	if err != nil {
		return err
	}
	dst, err := m.resolveVault(dstVault)
	if err != nil {
		return err
	}
	if newKey == "" {
		newKey = key
	}
	if src == dst && key == newKey {
		return nil
	}

	value, err := m.KC.Get(ServiceName(src), key)
	if err != nil {
		return fmt.Errorf("vault: read %q from %q: %w", key, src, err)
	}

	if !opts.Force {
		if _, err := m.KC.Get(ServiceName(dst), newKey); err == nil {
			return fmt.Errorf("%w: %q in vault %q", ErrKeyExists, newKey, dst)
		}
	}

	protected := m.protectionLookup(src)(key)
	if err := m.SetWithOptions(newKey, value, dst, SetOptions{Protected: protected}); err != nil {
		return err
	}

	if opts.Copy {
		return nil
	}

	if src == dst {
		if err := m.History().RenameKey(src, key, newKey); err != nil {
			return err
		}
	} else if err := m.History().MoveKey(src, key, dst, newKey); err != nil {
		return err
	}

	// Drop the source without recording another version: the value now lives
	// at the destination, and its history moved with it.
	if err := m.KC.Delete(ServiceName(src), key); err != nil {
		return fmt.Errorf("vault: drop %q from %q: %w", key, src, err)
	}
	return nil
}

func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
