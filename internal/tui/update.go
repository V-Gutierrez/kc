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
		m.recordCopy(msg.entry)
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
		if msg.removed != nil {
			m.removeEntry(*msg.removed)
		}
		m.upsertEntry(msg.entry)
		m.clearPreview()
		m.mode = modeBrowse
		m.formError = ""
		m.flashToken++
		if msg.removed != nil {
			m.flashMessage = fmt.Sprintf("✓ Moved %s:%s → %s:%s", msg.removed.Vault, msg.removed.Key, msg.entry.Vault, msg.entry.Key)
		} else {
			m.flashMessage = fmt.Sprintf("✓ Saved %s to vault:%s", msg.entry.Key, msg.entry.Vault)
		}
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
	case historyLoadedMsg:
		m.history.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// A successful read clears whatever failed before it, or a stale error
		// would keep hiding a perfectly good list.
		m.err = nil
		m.history.vault = msg.vault
		m.history.key = msg.key
		m.history.versions = msg.versions
		m.history.cursor = 0
		return m, nil
	case historyValueMsg:
		m.flashToken++
		m.flashMessage = fmt.Sprintf("✓ Copied version %d to clipboard, auto-clears in 30s", msg.seq)
		token := m.flashToken
		return m, tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return clearFlashMsg{token: token}
		})
	case rolledBackMsg:
		m.mode = modeBrowse
		m.history = historyState{}
		m.clearPreview()
		m.flashToken++
		m.flashMessage = fmt.Sprintf("✓ Restored %s from version %d", msg.key, msg.seq)
		token := m.flashToken
		return m, tea.Batch(loadEntriesCmd(m.deps), tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return clearFlashMsg{token: token}
		}))
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
	case vimTimeoutMsg:
		if msg.token == m.pendingVimToken && msg.key == m.pendingVimKey {
			m.pendingVimKey = ""
			return m.executeSingleVim(msg.key)
		}
		return m, nil
	case exportCompletedMsg:
		m.mode = modeBrowse
		m.flashToken++
		m.flashMessage = fmt.Sprintf("✓ Exported %d keys from %s to %s", msg.count, msg.vault, msg.path)
		return m, tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return clearFlashMsg{token: m.flashToken}
		})
	case importCompletedMsg:
		m.mode = modeBrowse
		m.flashToken++
		m.flashMessage = fmt.Sprintf("✓ Imported %d keys into %s from %s", msg.count, msg.vault, msg.path)
		return m, tea.Batch(loadEntriesCmd(m.deps), tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return clearFlashMsg{token: m.flashToken}
		}))
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
	case modeVaultPicker:
		return m.handleVaultPickerKey(msg)
	case modeCommandPalette:
		return m.handleCommandPaletteKey(msg)
	case modeHistory:
		return m.handleHistoryKey(msg)
	}

	if m.pendingVimKey != "" {
		if msg.String() == m.pendingVimKey {
			keyName := m.pendingVimKey
			m.pendingVimKey = ""
			return m.executeDoubleVim(keyName)
		}

		pending := m.pendingVimKey
		m.pendingVimKey = ""
		if msg.String() == "ctrl+/" || key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Command) || key.Matches(msg, m.keys.Search) || key.Matches(msg, m.keys.VaultNext) || key.Matches(msg, m.keys.VaultPrev) || key.Matches(msg, m.keys.CreateVault) || key.Matches(msg, m.keys.VaultPicker) || key.Matches(msg, m.keys.Add) || key.Matches(msg, m.keys.Edit) || key.Matches(msg, m.keys.Delete) || key.Matches(msg, m.keys.Copy) || key.Matches(msg, m.keys.Confirm) || key.Matches(msg, m.keys.Top) || key.Matches(msg, m.keys.Bottom) || key.Matches(msg, m.keys.Quit) {
			nextModel, cmd1 := m.executeSingleVim(pending)
			nextConcrete := nextModel.(Model)
			nextTea, cmd2 := nextConcrete.handleKey(msg)
			return nextTea, tea.Batch(cmd1, cmd2)
		}

		m.pendingVimKey = pending
	}

	switch {
	case msg.String() == "y":
		m.pendingVimToken++
		m.pendingVimKey = msg.String()
		token := m.pendingVimToken
		keyName := m.pendingVimKey
		return m, tea.Tick(250*time.Millisecond, func(_ time.Time) tea.Msg {
			return vimTimeoutMsg{token: token, key: keyName}
		})
	case msg.String() == "c":
		selected, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.pendingVimToken++
		m.pendingVimKey = "c"
		m.clearPreview()
		return m, copyCmd(m.deps, selected)
	case msg.String() == "d":
		m.pendingVimToken++
		m.pendingVimKey = "d"
		if _, ok := m.selectedEntry(); ok {
			m.mode = modeConfirmDelete
		}
		return m, nil
	case key.Matches(msg, m.keys.Help):
		m.mode = modeHelp
		return m, nil
	case key.Matches(msg, m.keys.Command):
		m.mode = modeCommandPalette
		m.commandInput.SetValue("")
		m.commandInput.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.Search):
		m.mode = modeSearch
		m.search.Focus()
		return m, textinput.Blink
	case msg.String() == "ctrl+/":
		m.currentFilter = allVaultsLabel
		m.mode = modeSearch
		m.search.Focus()
		m.search.SetValue("")
		m.applyFilters()
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
	case key.Matches(msg, m.keys.VaultPicker):
		m.mode = modeVaultPicker
		m.vaultPickerInput.SetValue("")
		m.vaultPickerInput.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.Bookmark):
		selected, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		if err := m.toggleBookmark(selected); err != nil {
			m.err = err
			return m, nil
		}
		m.applyFilters()
		if m.isBookmarked(selected) {
			m.flashMessage = fmt.Sprintf("✓ Bookmarked %s", selected.Key)
		} else {
			m.flashMessage = fmt.Sprintf("✓ Removed bookmark for %s", selected.Key)
		}
		m.flashToken++
		return m, tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return clearFlashMsg{token: m.flashToken}
		})
	case msg.String() >= "1" && msg.String() <= "9":
		idx := int(msg.String()[0] - '1')
		m.selectVaultByIndex(idx)
		return m, nil
	case key.Matches(msg, m.keys.Add):
		m.mode = modeAdd
		m.formError = ""
		m.form = newFormState(m.activeVault, "", "")
		return m, textinput.Blink
	case key.Matches(msg, m.keys.Edit):
		selected, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.mode = modeEdit
		m.formError = ""
		// Pre-fill with the live value so the form shows what is actually
		// stored. A failed read (declined Touch ID) leaves it empty, which the
		// submit then treats as "unchanged" rather than as an instruction to
		// blank the secret.
		value, err := m.deps.Store.Get(selected.Vault, selected.Key)
		if err != nil {
			if m.preview.revealed && m.preview.vault == selected.Vault && m.preview.key == selected.Key {
				value = m.preview.value
			}
		}
		m.form = newEditFormState(selected, value)
		return m, textinput.Blink
	case key.Matches(msg, m.keys.History):
		selected, ok := m.selectedEntry()
		if !ok || m.deps.History == nil {
			return m, nil
		}
		m.mode = modeHistory
		m.clearPreview()
		m.history = historyState{vault: selected.Vault, key: selected.Key, loading: true}
		return m, loadHistoryCmd(m.deps, selected)
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

func (m Model) executeSingleVim(keyName string) (tea.Model, tea.Cmd) {
	switch keyName {
	case "y":
		selected, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.clearPreview()
		return m, copyCmd(m.deps, selected)
	case "d":
		if _, ok := m.selectedEntry(); ok {
			m.mode = modeConfirmDelete
		}
	}
	return m, nil
}

func (m Model) executeDoubleVim(keyName string) (tea.Model, tea.Cmd) {
	switch keyName {
	case "c":
		selected, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.mode = modeEdit
		m.formError = ""
		// Pre-fill with the live value so the form shows what is actually
		// stored. A failed read (declined Touch ID) leaves it empty, which the
		// submit then treats as "unchanged" rather than as an instruction to
		// blank the secret.
		value, err := m.deps.Store.Get(selected.Vault, selected.Key)
		if err != nil {
			if m.preview.revealed && m.preview.vault == selected.Vault && m.preview.key == selected.Key {
				value = m.preview.value
			}
		}
		m.form = newEditFormState(selected, value)
		return m, textinput.Blink
	case "d":
		if _, ok := m.selectedEntry(); ok {
			m.mode = modeConfirmDelete
		}
		return m, nil
	case "y":
		selected, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.clearPreview()
		return m, copyCmd(m.deps, selected)
	default:
		return m, nil
	}
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
		m.formError = ""
		m.clearPreview()
		return m, nil
	}
	if key.Matches(msg, m.keys.Confirm) {
		m.formError = ""
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

func (m Model) handleVaultPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Cancel) {
		m.mode = modeBrowse
		m.vaultPickerInput.Blur()
		return m, nil
	}
	if key.Matches(msg, m.keys.Confirm) {
		query := strings.TrimSpace(m.vaultPickerInput.Value())
		m.vaultPickerInput.Blur()
		m.mode = modeBrowse
		if query == "" {
			return m, nil
		}
		matched := m.fuzzyMatchVault(query)
		if matched != "" {
			m.currentFilter = matched
			m.clearPreview()
			m.applyFilters()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.vaultPickerInput, cmd = m.vaultPickerInput.Update(msg)
	return m, cmd
}

func (m Model) handleCommandPaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Cancel) {
		m.mode = modeBrowse
		m.commandInput.Blur()
		return m, nil
	}
	if key.Matches(msg, m.keys.Confirm) {
		return m.executeCommandPalette()
	}
	var cmd tea.Cmd
	m.commandInput, cmd = m.commandInput.Update(msg)
	return m, cmd
}

func (m Model) executeCommandPalette() (tea.Model, tea.Cmd) {
	raw := strings.TrimSpace(m.commandInput.Value())
	raw = strings.TrimPrefix(raw, ":")
	m.commandInput.Blur()
	m.mode = modeBrowse
	if raw == "" {
		return m, nil
	}

	name, arg, _ := strings.Cut(raw, " ")
	arg = strings.TrimSpace(arg)

	switch name {
	case "vault":
		if arg == "" {
			m.mode = modeVaultPicker
			m.vaultPickerInput.SetValue("")
			m.vaultPickerInput.Focus()
			return m, textinput.Blink
		}
		matched := m.fuzzyMatchVault(arg)
		if matched != "" {
			m.currentFilter = matched
			m.clearPreview()
			m.applyFilters()
		}
		return m, nil
	case "search":
		m.currentFilter = allVaultsLabel
		m.search.SetValue(arg)
		if arg == "" {
			m.mode = modeSearch
			m.search.Focus()
			m.applyFilters()
			return m, textinput.Blink
		}
		m.search.Blur()
		m.applyFilters()
		return m, nil
	case "export":
		path := arg
		if path == "" {
			path = fmt.Sprintf("%s.env", m.currentVaultContext())
		}
		return m, exportVaultCmd(m.deps, m.currentVaultContext(), path)
	case "import":
		if arg == "" {
			m.flashMessage = "Import requires a file path"
			return m, nil
		}
		return m, importVaultCmd(m.deps, m.currentVaultContext(), arg)
	default:
		m.flashMessage = fmt.Sprintf("Unknown command: %s", name)
		return m, nil
	}
}

func (m Model) submitForm() (Model, tea.Cmd) {
	vault := strings.TrimSpace(m.form.vault.Value())
	keyName := strings.TrimSpace(m.form.key.Value())
	value := m.form.value.Value()
	if vault == "" {
		vault = m.activeVault
	}
	if keyName == "" {
		m.form.confirming = false
		m.formError = "key name cannot be empty"
		return m, nil
	}
	protection := protectionUnprotected
	if m.form.isProtected {
		protection = protectionProtected
	}
	target := entry{Vault: vault, Key: keyName, Protection: protection}
	origin := m.form.origin

	// Adding a key: nothing to preserve, nothing to move.
	if origin == nil {
		m.formError = ""
		return m, saveCmd(m.deps, target, value)
	}

	// Editing: an untouched value field means "leave the secret as it is".
	// The form opens blank whenever the value was never revealed, so treating
	// blank as an instruction would silently destroy the secret on a plain
	// Enter. The current value is read back at write time instead.
	keepValue := value == "" && !m.form.valueSeeded
	if keepValue && sameEntry(*origin, target) && origin.Protection == target.Protection {
		// Nothing changed at all — do not touch the Keychain (and do not burn
		// a history slot) just to rewrite identical bytes.
		m.mode = modeBrowse
		m.form.confirming = false
		m.formError = ""
		m.flashToken++
		m.flashMessage = fmt.Sprintf("No changes to %s", origin.Key)
		token := m.flashToken
		return m, tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
			return clearFlashMsg{token: token}
		})
	}

	m.formError = ""
	return m, saveEntryCmd(m.deps, target, value, origin, keepValue)
}

func (m Model) handleHistoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.history.confirming {
		if key.Matches(msg, m.keys.Confirm) {
			version, ok := m.history.selected()
			if !ok {
				m.history.confirming = false
				return m, nil
			}
			m.history.confirming = false
			return m, rollbackCmd(m.deps, m.history.vault, m.history.key, version.Seq)
		}
		// Anything that is not a confirmation cancels: a restore overwrites the
		// live secret, so an ambiguous keystroke must never be taken as a yes.
		m.history.confirming = false
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.mode = modeBrowse
		m.history = historyState{}
		return m, nil
	case key.Matches(msg, m.keys.Up):
		if m.history.cursor > 0 {
			m.history.cursor--
		}
		return m, nil
	case key.Matches(msg, m.keys.Down):
		if m.history.cursor < len(m.history.versions)-1 {
			m.history.cursor++
		}
		return m, nil
	case key.Matches(msg, m.keys.Top):
		m.history.cursor = 0
		return m, nil
	case key.Matches(msg, m.keys.Bottom):
		if n := len(m.history.versions); n > 0 {
			m.history.cursor = n - 1
		}
		return m, nil
	case key.Matches(msg, m.keys.Confirm):
		version, ok := m.history.selected()
		if !ok {
			return m, nil
		}
		return m, revealVersionCmd(m.deps, m.history.vault, m.history.key, version.Seq)
	case msg.String() == "r":
		if _, ok := m.history.selected(); !ok {
			return m, nil
		}
		m.history.confirming = true
		return m, nil
	}
	return m, nil
}
