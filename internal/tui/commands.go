package tui

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"
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
				items = append(items, entry{Vault: vault, Key: m.Key, Protection: protection})
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
