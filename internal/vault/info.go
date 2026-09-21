package vault

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Info is everything kc knows about a vault beyond its name.
//
// It is stored in ~/.kc/vaults as tab-separated `key=value` attributes after
// the name. A vault with no attributes is written as a bare name, which is
// exactly the pre-v0.5.0 format — old files load unchanged and vaults nobody
// annotated stay readable by older builds.
type Info struct {
	Name              string
	Description       string
	Tags              []string
	Created           string
	RequireProtection bool
}

const (
	attrDescription       = "desc"
	attrTags              = "tags"
	attrCreated           = "created"
	attrRequireProtection = "require-protection"
)

// HasAttributes reports whether anything beyond the name is set.
func (i Info) HasAttributes() bool {
	return i.Description != "" || len(i.Tags) > 0 || i.Created != "" || i.RequireProtection
}

// Line renders the vault as one line of the vaults file.
func (i Info) Line() string {
	parts := []string{i.Name}
	if i.Description != "" {
		parts = append(parts, attrDescription+"="+sanitizeAttr(i.Description))
	}
	if len(i.Tags) > 0 {
		parts = append(parts, attrTags+"="+sanitizeAttr(strings.Join(i.Tags, ",")))
	}
	if i.Created != "" {
		parts = append(parts, attrCreated+"="+sanitizeAttr(i.Created))
	}
	if i.RequireProtection {
		parts = append(parts, attrRequireProtection+"=true")
	}
	return strings.Join(parts, "\t")
}

// parseInfoLine reads one vaults-file line. Empty names yield ok=false.
func parseInfoLine(line string) (Info, bool) {
	fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
	name := strings.TrimSpace(fields[0])
	if name == "" {
		return Info{}, false
	}

	info := Info{Name: name}
	for _, field := range fields[1:] {
		key, value, found := strings.Cut(strings.TrimSpace(field), "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case attrDescription:
			info.Description = value
		case attrTags:
			info.Tags = splitTags(value)
		case attrCreated:
			info.Created = value
		case attrRequireProtection:
			enabled, err := strconv.ParseBool(value)
			if err == nil {
				info.RequireProtection = enabled
			}
		}
	}
	return info, true
}

func splitTags(value string) []string {
	raw := strings.Split(value, ",")
	tags := make([]string, 0, len(raw))
	for _, tag := range raw {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

// sanitizeAttr keeps the single-line, tab-delimited file format intact.
func sanitizeAttr(value string) string {
	replacer := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return strings.TrimSpace(replacer.Replace(value))
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// --- Manager operations over vault metadata ---

// ListVaultInfos returns every registered vault with its metadata, sorted by name.
func (m *Manager) ListVaultInfos() ([]Info, error) {
	if err := m.ensureDataDir(); err != nil {
		return nil, err
	}

	infos, err := m.loadInfos()
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		infos = []Info{{Name: DefaultVault}}
		if err := m.saveInfos(infos); err != nil {
			return nil, err
		}
	}
	return infos, nil
}

// VaultInfo returns the metadata of a single vault.
func (m *Manager) VaultInfo(name string) (Info, error) {
	infos, err := m.ListVaultInfos()
	if err != nil {
		return Info{}, err
	}
	for _, info := range infos {
		if info.Name == name {
			return info, nil
		}
	}
	return Info{}, fmt.Errorf("%w: vault %q", ErrNotFound, name)
}

// VaultExists reports whether a vault is registered.
func (m *Manager) VaultExists(name string) bool {
	infos, err := m.ListVaultInfos()
	if err != nil {
		return false
	}
	for _, info := range infos {
		if info.Name == name {
			return true
		}
	}
	return false
}

// SetVaultInfo mutates a vault's metadata in place and persists it.
func (m *Manager) SetVaultInfo(name string, mutate func(*Info)) error {
	if err := validateName(name); err != nil {
		return err
	}

	infos, err := m.ListVaultInfos()
	if err != nil {
		return err
	}
	for i := range infos {
		if infos[i].Name != name {
			continue
		}
		mutate(&infos[i])
		infos[i].Name = name // a mutator must not be able to rename by accident
		return m.saveInfos(infos)
	}
	return fmt.Errorf("%w: vault %q", ErrNotFound, name)
}

// CreateWithInfo registers a new vault along with its metadata.
func (m *Manager) CreateWithInfo(info Info) error {
	if err := validateName(info.Name); err != nil {
		return err
	}

	infos, err := m.ListVaultInfos()
	if err != nil {
		return err
	}
	for _, existing := range infos {
		if existing.Name == info.Name {
			return ErrAlreadyExists
		}
	}
	if info.Created == "" {
		info.Created = nowStamp()
	}

	infos = append(infos, info)
	return m.saveInfos(infos)
}

func (m *Manager) loadInfos() ([]Info, error) {
	data, err := readFileIfExists(m.vaultsPath())
	if err != nil {
		return nil, err
	}

	infos := make([]Info, 0, 4)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		info, ok := parseInfoLine(line)
		if !ok {
			continue
		}
		if _, duplicate := seen[info.Name]; duplicate {
			continue
		}
		seen[info.Name] = struct{}{}
		infos = append(infos, info)
	}

	sortInfos(infos)
	return infos, nil
}

func (m *Manager) saveInfos(infos []Info) error {
	if err := m.ensureDataDir(); err != nil {
		return err
	}

	sortInfos(infos)
	var buf strings.Builder
	for _, info := range infos {
		buf.WriteString(info.Line())
		buf.WriteByte('\n')
	}
	return writeFile600(m.vaultsPath(), buf.String())
}

func sortInfos(infos []Info) {
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
}
