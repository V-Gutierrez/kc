package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(max(20, msg.Width/2), max(10, msg.Height-8))
		return m, nil
	case loadedMsg:
		m.loading = false
		m.err = msg.err
		m.status = ""
		m.vaults = append([]string{allVaultsLabel}, msg.vaults...)
		m.activeVault = msg.activeVault
		m.entries = append([]entry(nil), msg.items...)
		if m.currentFilter == "" {
			m.currentFilter = allVaultsLabel
		}
		if m.deps.InitialFilter != "" {
			m.currentFilter = m.deps.InitialFilter
		}
		m.applyFilters()
		return m, nil
	case revealedMsg:
		m.revealToken++
		m.preview = previewState{vault: msg.entry.Vault, key: msg.entry.Key, value: msg.value, revealed: true}
		m.status = fmt.Sprintf("Revealed %s from %s", msg.entry.Key, msg.entry.Vault)
		return m, tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
			return hideMsg{entry: msg.entry, token: m.revealToken}
		})
	case hideMsg:
		if m.preview.revealed && msg.token == m.revealToken && m.preview.vault == msg.entry.Vault && m.preview.key == msg.entry.Key {
			m.clearPreview()
			m.status = "Value hidden"
		}
		return m, nil
	case copiedMsg:
		m.flashToken++
		m.flashMessage = "✓ Copied to clipboard, auto-clears in 30s"
		return m, tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return clearFlashMsg{token: m.flashToken}
		})
	case clearFlashMsg:
		if msg.token == m.flashToken {
			m.flashMessage = ""
		}
		return m, nil
	case savedMsg:
		m.upsertEntry(msg.entry)
		m.clearPreview()
		m.mode = modeBrowse
		m.flashToken++
		m.flashMessage = fmt.Sprintf("✓ Saved %s to vault:%s", msg.entry.Key, msg.entry.Vault)
		m.applyFilters()
		return m, tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return clearFlashMsg{token: m.flashToken}
		})
	case deletedMsg:
		m.removeEntry(msg.entry)
		m.clearPreview()
		m.mode = modeBrowse
		m.status = fmt.Sprintf("Deleted %s from %s", msg.entry.Key, msg.entry.Vault)
		m.applyFilters()
		return m, nil
	case vaultCreatedMsg:
		m.vaults = append(m.vaults, msg.name)
		m.currentFilter = msg.name
		m.mode = modeBrowse
		m.flashToken++
		m.flashMessage = fmt.Sprintf("✓ Created vault %s", msg.name)
		m.clearPreview()
		m.applyFilters()
		return m, tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return clearFlashMsg{token: m.flashToken}
		})
	case errMsg:
		m.err = msg.err
		m.status = ""
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}

	switch m.mode {
	case modeSearch:
		return m.handleSearchKey(msg)
	case modeAdd, modeEdit:
		return m.handleFormKey(msg)
	case modeConfirmDelete:
		return m.handleDeleteConfirm(msg)
	case modeHelp:
		return m.handleHelpKey(msg)
	case modeCreateVault:
		return m.handleCreateVaultKey(msg)
	}

	switch {
	case key.Matches(msg, m.keys.Help):
		m.mode = modeHelp
		return m, nil
	case key.Matches(msg, m.keys.Search):
		m.mode = modeSearch
		m.search.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.VaultNext):
		m.cycleVaultFilter()
		return m, nil
	case key.Matches(msg, m.keys.VaultPrev):
		m.cycleVaultFilterReverse()
		return m, nil
	case key.Matches(msg, m.keys.CreateVault):
		m.mode = modeCreateVault
		m.vaultNameInput.SetValue("")
		m.vaultNameInput.Focus()
		return m, textinput.Blink
	case msg.String() >= "1" && msg.String() <= "9":
		idx := int(msg.String()[0] - '1')
		m.selectVaultByIndex(idx)
		return m, nil
	case key.Matches(msg, m.keys.Add):
		m.mode = modeAdd
		m.form = newFormState(m.activeVault, "", "")
		return m, textinput.Blink
	case key.Matches(msg, m.keys.Edit):
		selected, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.mode = modeEdit
		value := ""
		if m.preview.revealed && m.preview.vault == selected.Vault && m.preview.key == selected.Key {
			value = m.preview.value
		}
		m.form = newFormState(selected.Vault, selected.Key, value)
		m.form.isProtected = selected.Protection != protectionUnprotected
		return m, textinput.Blink
	case key.Matches(msg, m.keys.Delete):
		if _, ok := m.selectedEntry(); ok {
			m.mode = modeConfirmDelete
			return m, nil
		}
	case key.Matches(msg, m.keys.Copy):
		selected, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.clearPreview()
		return m, copyCmd(m.deps, selected)
	case key.Matches(msg, m.keys.Confirm):
		selected, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		if m.preview.revealed && m.preview.vault == selected.Vault && m.preview.key == selected.Key {
			return m, copyKnownCmd(m.deps, selected, m.preview.value)
		}
		return m, revealCmd(m.deps, selected)
	case key.Matches(msg, m.keys.Top):
		if len(m.list.Items()) > 0 {
			m.list.Select(0)
			m.clearPreview()
		}
		return m, nil
	case key.Matches(msg, m.keys.Bottom):
		if len(m.list.Items()) > 0 {
			m.list.Select(len(m.list.Items()) - 1)
			m.clearPreview()
		}
		return m, nil
	}

	var cmd tea.Cmd
	previous := m.list.Index()
	m.list, cmd = m.list.Update(msg)
	if previous != m.list.Index() {
		m.clearPreview()
	}
	return m, cmd
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Cancel) || key.Matches(msg, m.keys.Confirm) {
		m.mode = modeBrowse
		m.search.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.clearPreview()
	m.applyFilters()
	return m, cmd
}

func (m Model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.form.confirming {
		if key.Matches(msg, m.keys.Confirm) {
			return m.submitForm()
		}
		if key.Matches(msg, m.keys.Cancel) {
			m.form.confirming = false
			return m, nil
		}
		return m, nil
	}

	if key.Matches(msg, m.keys.Cancel) {
		m.mode = modeBrowse
		m.clearPreview()
		return m, nil
	}
	if key.Matches(msg, m.keys.Confirm) {
		m.form.confirming = true
		return m, nil
	}
	if msg.String() == "tab" {
		m.form.focus = (m.form.focus + 1) % 4
		m.focusForm()
		return m, nil
	}
	if msg.String() == "f2" && m.form.focus == 2 {
		if m.form.value.EchoMode == textinput.EchoPassword {
			m.form.value.EchoMode = textinput.EchoNormal
		} else {
			m.form.value.EchoMode = textinput.EchoPassword
		}
		return m, nil
	}
	if msg.String() == " " && m.form.focus == 3 {
		m.form.isProtected = !m.form.isProtected
		return m, nil
	}
	if m.form.focus == 0 && strings.TrimSpace(m.form.vault.Value()) == "" {
		if vault, ok := m.quickSelectVault(msg.String()); ok {
			m.form.vault.SetValue(vault)
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.form.focus {
	case 0:
		m.form.vault, cmd = m.form.vault.Update(msg)
	case 1:
		m.form.key, cmd = m.form.key.Update(msg)
	case 2:
		m.form.value, cmd = m.form.value.Update(msg)
	}
	return m, cmd
}

func (m Model) handleDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if strings.EqualFold(msg.String(), "y") {
		selected, ok := m.selectedEntry()
		if !ok {
			m.mode = modeBrowse
			return m, nil
		}
		return m, deleteCmd(m.deps, selected)
	}
	m.mode = modeBrowse
	return m, nil
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Cancel) {
		m.mode = modeBrowse
		return m, nil
	}
	return m, nil
}

func (m Model) handleCreateVaultKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Cancel) {
		m.mode = modeBrowse
		m.vaultNameInput.Blur()
		return m, nil
	}
	if key.Matches(msg, m.keys.Confirm) {
		name := strings.TrimSpace(m.vaultNameInput.Value())
		if name == "" {
			m.mode = modeBrowse
			m.vaultNameInput.Blur()
			return m, nil
		}
		m.vaultNameInput.Blur()
		return m, createVaultCmd(m.deps, name)
	}
	var cmd tea.Cmd
	m.vaultNameInput, cmd = m.vaultNameInput.Update(msg)
	return m, cmd
}

func (m Model) submitForm() (Model, tea.Cmd) {
	vault := strings.TrimSpace(m.form.vault.Value())
	keyName := strings.TrimSpace(m.form.key.Value())
	value := m.form.value.Value()
	if vault == "" {
		vault = m.activeVault
	}
	protection := protectionUnprotected
	if m.form.isProtected {
		protection = protectionProtected
	}
	entry := entry{Vault: vault, Key: keyName, Protection: protection}
	return m, saveCmd(m.deps, entry, value)
}
