package tui

import (
	"fmt"
	"os"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/v-gutierrez/kc/internal/envutil"
)

func loadEntriesCmd(deps Deps) tea.Cmd {
	return func() tea.Msg {
		vaults, err := deps.Vaults.List()
		if err != nil {
			return loadedMsg{err: err}
		}
		active, err := deps.Vaults.Active()
		if err != nil {
			active = "default"
		}
		items := make([]entry, 0)
		for _, vault := range vaults {
			metas, err := deps.Store.ListMetadata(vault)
			if err != nil {
				return loadedMsg{err: err}
			}
			sort.Slice(metas, func(i, j int) bool {
				return metas[i].Key < metas[j].Key
			})
			for _, m := range metas {
				protection := m.Protection
				if protection == "" {
					protection = protectionProtected
				}
				items = append(items, entry{Vault: vault, Key: m.Key, Protection: protection, Modified: m.Modified})
			}
		}
		return loadedMsg{vaults: vaults, activeVault: active, items: items}
	}
}

func revealCmd(deps Deps, item entry) tea.Cmd {
	return func() tea.Msg {
		value, err := deps.Store.Get(item.Vault, item.Key)
		if err != nil {
			return errMsg{err: err}
		}
		return revealedMsg{entry: item, value: value}
	}
}

func copyCmd(deps Deps, item entry) tea.Cmd {
	return func() tea.Msg {
		value, err := deps.Store.Get(item.Vault, item.Key)
		if err != nil {
			return errMsg{err: err}
		}
		if deps.Clipboard != nil {
			if err := deps.Clipboard.Copy(value); err != nil {
				return errMsg{err: err}
			}
		}
		return copiedMsg{entry: item, value: value}
	}
}

func copyKnownCmd(deps Deps, item entry, value string) tea.Cmd {
	return func() tea.Msg {
		if deps.Clipboard != nil {
			if err := deps.Clipboard.Copy(value); err != nil {
				return errMsg{err: err}
			}
		}
		return copiedMsg{entry: item, value: value}
	}
}

func saveCmd(deps Deps, item entry, value string) tea.Cmd {
	return func() tea.Msg {
		protected := item.Protection == protectionProtected
		if err := deps.Store.SetWithProtection(item.Vault, item.Key, value, protected); err != nil {
			return errMsg{err: err}
		}
		return savedMsg{entry: item, value: value}
	}
}

// saveEntryCmd writes an edited secret. When keepValue is set the value is read
// back from the origin first, so an untouched form field never blanks a secret.
// When the target moved (renamed key or different vault) the origin is removed
// only after the new location is written — a failed second step leaves a
// duplicate, never a hole.
func saveEntryCmd(deps Deps, target entry, value string, origin *entry, keepValue bool) tea.Cmd {
	return func() tea.Msg {
		if keepValue && origin != nil {
			current, err := deps.Store.Get(origin.Vault, origin.Key)
			if err != nil {
				return errMsg{err: fmt.Errorf("read current value of %s: %w", origin.Key, err)}
			}
			value = current
		}
		protected := target.Protection == protectionProtected
		if err := deps.Store.SetWithProtection(target.Vault, target.Key, value, protected); err != nil {
			return errMsg{err: err}
		}
		if origin == nil || (origin.Vault == target.Vault && origin.Key == target.Key) {
			return savedMsg{entry: target, value: value}
		}
		if err := deps.Store.Delete(origin.Vault, origin.Key); err != nil {
			return errMsg{err: fmt.Errorf("saved %s but could not remove %s from %s: %w", target.Key, origin.Key, origin.Vault, err)}
		}
		removed := *origin
		return savedMsg{entry: target, value: value, removed: &removed}
	}
}

func deleteCmd(deps Deps, item entry) tea.Cmd {
	return func() tea.Msg {
		if err := deps.Store.Delete(item.Vault, item.Key); err != nil {
			return errMsg{err: err}
		}
		return deletedMsg{entry: item}
	}
}

func createVaultCmd(deps Deps, name string) tea.Cmd {
	return func() tea.Msg {
		if err := deps.Vaults.Create(name); err != nil {
			return errMsg{err: err}
		}
		return vaultCreatedMsg{name: name}
	}
}

func exportVaultCmd(deps Deps, vault, path string) tea.Cmd {
	return func() tea.Msg {
		metadata, err := deps.Store.ListMetadata(vault)
		if err != nil {
			return errMsg{err: err}
		}
		sort.Slice(metadata, func(i, j int) bool {
			return metadata[i].Key < metadata[j].Key
		})
		lines := make([]string, 0, len(metadata))
		for _, item := range metadata {
			value, err := deps.Store.Get(vault, item.Key)
			if err != nil {
				return errMsg{err: err}
			}
			lines = append(lines, fmt.Sprintf("%s=%s", item.Key, envutil.DotenvQuote(value)))
		}
		if err := os.WriteFile(path, []byte(envutil.JoinLines(lines)), 0o600); err != nil {
			return errMsg{err: err}
		}
		return exportCompletedMsg{vault: vault, path: path, count: len(lines)}
	}
}

func importVaultCmd(deps Deps, vault, path string) tea.Cmd {
	return func() tea.Msg {
		file, err := os.Open(path)
		if err != nil {
			return errMsg{err: err}
		}
		defer file.Close()

		entries := envutil.ParseEnvReader(file)
		count := 0
		for _, key := range envutil.SortedKeys(entries) {
			if err := deps.Store.SetWithProtection(vault, key, entries[key], true); err != nil {
				return errMsg{err: err}
			}
			count++
		}
		return importCompletedMsg{vault: vault, path: path, count: count}
	}
}

// loadHistoryCmd fetches the recorded versions of one secret. Listing versions
// reads digests and timestamps only, never the values.
func loadHistoryCmd(deps Deps, item entry) tea.Cmd {
	return func() tea.Msg {
		versions, err := deps.History.Versions(item.Vault, item.Key)
		return historyLoadedMsg{vault: item.Vault, key: item.Key, versions: versions, err: err}
	}
}

// revealVersionCmd copies one recorded value to the clipboard. A previous
// secret is still a secret, so it goes to the clipboard like a live one and is
// never rendered.
func revealVersionCmd(deps Deps, vault, key string, seq int) tea.Cmd {
	return func() tea.Msg {
		value, err := deps.History.Value(vault, key, seq)
		if err != nil {
			return errMsg{err: err}
		}
		if deps.Clipboard != nil {
			if err := deps.Clipboard.Copy(value); err != nil {
				return errMsg{err: err}
			}
		}
		return historyValueMsg{seq: seq, value: value}
	}
}

// rollbackCmd restores a recorded version as the live value. The restore is
// itself recorded, so it can be undone the same way.
func rollbackCmd(deps Deps, vault, key string, seq int) tea.Cmd {
	return func() tea.Msg {
		if err := deps.History.Rollback(vault, key, seq); err != nil {
			return errMsg{err: err}
		}
		return rolledBackMsg{vault: vault, key: key, seq: seq}
	}
}
