package vault

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ArchivePrefix namespaces the Keychain service holding a soft-deleted vault.
const ArchivePrefix = "kc:__archive__:"

const (
	attrDeleted = "deleted"
	attrKeys    = "keys"
)

// Archived is a vault that was deleted but is still restorable.
type Archived struct {
	Name      string
	DeletedAt time.Time
	Keys      int
	Info      Info
}

// DeleteOptions controls how a vault is removed.
type DeleteOptions struct {
	// Force allows deleting a vault that still holds keys.
	Force bool
	// Purge destroys the keys immediately instead of archiving them,
	// and drops the vault's recorded history along with them.
	Purge bool
}

func archiveServiceName(vault string) string {
	return ArchivePrefix + vault
}

// DeleteVault removes a vault, archiving its keys so they stay restorable.
func (m *Manager) DeleteVault(name string, force bool) error {
	return m.DeleteVaultWithOptions(name, DeleteOptions{Force: force})
}

// DeleteVaultWithOptions removes a vault. By default the keys are moved to the
// archive (see RestoreVault); with Purge they are destroyed outright.
func (m *Manager) DeleteVaultWithOptions(name string, opts DeleteOptions) error {
	if err := validateName(name); err != nil {
		return err
	}
	if name == DefaultVault {
		return ErrDefaultVault
	}
	if err := m.requireVault(name); err != nil {
		return err
	}

	service := ServiceName(name)
	keys, err := m.KC.List(service)
	if err != nil {
		return fmt.Errorf("vault: list keys for %q: %w", name, err)
	}
	if len(keys) > 0 && !opts.Force {
		return fmt.Errorf("vault has %d keys: delete them first or use --force", len(keys))
	}

	info, err := m.VaultInfo(name)
	if err != nil {
		return err
	}

	if opts.Purge {
		if _, err := m.PurgeArchived(name); err != nil && !isNotFound(err) {
			return err
		}
		if _, err := m.History().PurgeVault(name); err != nil {
			return err
		}
	} else if err := m.archiveKeys(name, keys, info); err != nil {
		return err
	}

	for _, key := range keys {
		if err := m.KC.Delete(service, key); err != nil {
			return fmt.Errorf("vault: delete key %q from %q: %w", key, name, err)
		}
	}

	infos, err := m.ListVaultInfos()
	if err != nil {
		return err
	}
	remaining := make([]Info, 0, len(infos))
	for _, candidate := range infos {
		if candidate.Name != name {
			remaining = append(remaining, candidate)
		}
	}
	if err := m.saveInfos(remaining); err != nil {
		return err
	}

	if m.activeVaultFromFile() == name {
		return m.Switch(DefaultVault)
	}
	return nil
}

func (m *Manager) archiveKeys(name string, keys []string, info Info) error {
	// A previous archive of the same name would be shadowed by this one.
	if _, err := m.PurgeArchived(name); err != nil && !isNotFound(err) {
		return err
	}

	service := ServiceName(name)
	target := archiveServiceName(name)
	protectionOf := m.protectionLookup(name)

	for _, key := range keys {
		value, err := m.KC.Get(service, key)
		if err != nil {
			return fmt.Errorf("vault: archive read %q: %w", key, err)
		}
		if err := m.KC.SetWithProtection(target, key, value, protectionOf(key)); err != nil {
			return fmt.Errorf("vault: archive write %q: %w", key, err)
		}
	}

	records, err := m.loadArchive()
	if err != nil {
		return err
	}
	records = append(records, Archived{Name: name, DeletedAt: time.Now().UTC(), Keys: len(keys), Info: info})
	return m.saveArchive(records)
}

// ListArchived returns every restorable vault, newest deletion first.
func (m *Manager) ListArchived() ([]Archived, error) {
	records, err := m.loadArchive()
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].DeletedAt.After(records[j].DeletedAt) })
	return records, nil
}

// RestoreVault brings a soft-deleted vault back with its keys, protection
// flags and metadata intact.
func (m *Manager) RestoreVault(name string) (int, error) {
	if err := validateName(name); err != nil {
		return 0, err
	}

	records, err := m.loadArchive()
	if err != nil {
		return 0, err
	}
	index := -1
	for i, record := range records {
		if record.Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		return 0, fmt.Errorf("%w: no archived vault %q", ErrNotFound, name)
	}
	if m.VaultExists(name) {
		return 0, fmt.Errorf("%w: vault %q is in use again", ErrAlreadyExists, name)
	}

	source := archiveServiceName(name)
	target := ServiceName(name)
	items, err := m.KC.ListMetadata(source)
	if err != nil {
		return 0, fmt.Errorf("vault: read archive of %q: %w", name, err)
	}

	restored := 0
	for _, item := range items {
		value, err := m.KC.Get(source, item.Account)
		if err != nil {
			return restored, fmt.Errorf("vault: restore read %q: %w", item.Account, err)
		}
		if err := m.KC.SetWithProtection(target, item.Account, value, item.Protected); err != nil {
			return restored, fmt.Errorf("vault: restore write %q: %w", item.Account, err)
		}
		if err := m.KC.Delete(source, item.Account); err != nil {
			return restored, fmt.Errorf("vault: drop archived %q: %w", item.Account, err)
		}
		restored++
	}

	info := records[index].Info
	info.Name = name
	if err := m.CreateWithInfo(info); err != nil {
		return restored, err
	}

	records = append(records[:index], records[index+1:]...)
	return restored, m.saveArchive(records)
}

// PurgeArchived destroys an archived vault for good, history included.
func (m *Manager) PurgeArchived(name string) (int, error) {
	records, err := m.loadArchive()
	if err != nil {
		return 0, err
	}

	index := -1
	for i, record := range records {
		if record.Name == name {
			index = i
			break
		}
	}

	service := archiveServiceName(name)
	items, err := m.KC.ListMetadata(service)
	if err != nil {
		return 0, fmt.Errorf("vault: read archive of %q: %w", name, err)
	}
	if index < 0 && len(items) == 0 {
		return 0, fmt.Errorf("%w: no archived vault %q", ErrNotFound, name)
	}

	removed := 0
	for _, item := range items {
		if err := m.KC.Delete(service, item.Account); err != nil {
			return removed, fmt.Errorf("vault: purge %q: %w", item.Account, err)
		}
		removed++
	}
	if _, err := m.History().PurgeVault(name); err != nil {
		return removed, err
	}

	if index >= 0 {
		records = append(records[:index], records[index+1:]...)
		if err := m.saveArchive(records); err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// GCArchived purges archived vaults deleted more than retentionDays ago and
// returns their names. A retention of zero keeps everything.
func (m *Manager) GCArchived(retentionDays int) ([]string, error) {
	if retentionDays <= 0 {
		return nil, nil
	}

	records, err := m.loadArchive()
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	expired := make([]string, 0)
	for _, record := range records {
		if record.DeletedAt.Before(cutoff) {
			expired = append(expired, record.Name)
		}
	}

	for _, name := range expired {
		if _, err := m.PurgeArchived(name); err != nil {
			return expired, err
		}
	}
	sort.Strings(expired)
	return expired, nil
}

func (m *Manager) archivePath() string {
	return filepath.Join(m.DataDir, "archive")
}

func (m *Manager) loadArchive() ([]Archived, error) {
	data, err := readFileIfExists(m.archivePath())
	if err != nil {
		return nil, err
	}

	records := make([]Archived, 0, 2)
	for _, line := range strings.Split(string(data), "\n") {
		if record, ok := parseArchiveLine(line); ok {
			records = append(records, record)
		}
	}
	return records, nil
}

func (m *Manager) saveArchive(records []Archived) error {
	if err := m.ensureDataDir(); err != nil {
		return err
	}

	var buf strings.Builder
	for _, record := range records {
		buf.WriteString(archiveLine(record))
		buf.WriteByte('\n')
	}
	return writeFile600(m.archivePath(), buf.String())
}

func archiveLine(record Archived) string {
	info := record.Info
	info.Name = record.Name
	line := info.Line()
	line += "\t" + attrDeleted + "=" + record.DeletedAt.UTC().Format(time.RFC3339)
	line += "\t" + attrKeys + "=" + strconv.Itoa(record.Keys)
	return line
}

func parseArchiveLine(line string) (Archived, bool) {
	info, ok := parseInfoLine(line)
	if !ok {
		return Archived{}, false
	}

	record := Archived{Name: info.Name, Info: info}
	for _, field := range strings.Split(line, "\t")[1:] {
		key, value, found := strings.Cut(strings.TrimSpace(field), "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case attrDeleted:
			if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
				record.DeletedAt = parsed
			}
		case attrKeys:
			if count, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				record.Keys = count
			}
		}
	}
	return record, true
}
