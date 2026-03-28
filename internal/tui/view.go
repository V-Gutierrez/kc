package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

func (m Model) View() string {
	if m.loading {
		banner := m.styles.banner.Render(kcBanner)
		content := lipgloss.JoinVertical(
			lipgloss.Center,
			banner,
			"",
			m.styles.loading.Render("Loading vaults and keys..."),
		)
		width := max(m.width, 80)
		height := max(m.height, 24)
		return m.styles.app.Render(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content))
	}

	if len(m.entries) == 0 && m.err == nil {
		return m.welcomeView()
	}

	left := lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(),
		m.tabBarView(),
		m.searchView(),
		m.list.View(),
		m.helpView(),
	)

	right := m.previewView()
	if m.mode == modeAdd || m.mode == modeEdit || m.mode == modeConfirmDelete {
		right = m.overlayView()
	}
	if m.mode == modeHelp {
		right = m.helpOverlayView()
	}
	if m.mode == modeCreateVault {
		right = m.createVaultView()
	}
	if m.mode == modeVaultPicker {
		right = m.vaultPickerView()
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(max(40, m.width/2)).Render(left),
		lipgloss.NewStyle().PaddingLeft(2).Width(max(30, m.width/2-4)).Render(right),
	)

	statusBar := m.statusView()
	return m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, body, "\n", statusBar))
}

func (m Model) statusView() string {
	if m.flashMessage != "" {
		return m.styles.flash.Render(m.flashMessage)
	}

	breadcrumb := m.breadcrumb()
	hints := m.contextualHints()

	barWidth := max(m.width-4, 40)
	left := m.styles.breadcrumb.Render(breadcrumb)
	right := m.styles.statusHint.Render(hints)

	gap := barWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	padding := strings.Repeat(" ", gap)

	return m.styles.statusBar.Render(left + padding + right)
}

func (m Model) breadcrumb() string {
	vault := m.currentFilter
	if vault == allVaultsLabel {
		vault = m.activeVault
	}

	selected, ok := m.selectedEntry()
	if !ok {
		return vault
	}

	category := strings.ToUpper(prefixOf(selected.Key))
	if category == "OTHER" {
		return vault + " > " + selected.Key
	}
	return vault + " > " + category + " > " + selected.Key
}

func (m Model) contextualHints() string {
	switch m.mode {
	case modeSearch:
		return "Enter confirm • Esc cancel"
	case modeAdd, modeEdit:
		return "Tab next • Esc cancel • Enter confirm"
	case modeConfirmDelete:
		return "y confirm • n cancel"
	case modeHelp:
		return "? or Esc close help"
	case modeCreateVault:
		return "Enter create • Esc cancel"
	case modeVaultPicker:
		return "Enter select • Esc cancel"
	}
	return "/ search  : cmd  ? help"
}

func (m Model) headerView() string {
	vault := m.currentFilter
	if vault == allVaultsLabel {
		vault = m.activeVault
	}
	count := len(m.list.Items())
	label := "keys"
	if count == 1 {
		label = "key"
	}
	return m.styles.header.Render(fmt.Sprintf("🔒 kc • vault: %s • %d %s", vault, count, label))
}

func (m Model) tabBarView() string {
	realVaults := m.vaultHints()
	if len(realVaults) == 0 {
		return ""
	}

	tabs := make([]string, 0, len(realVaults)+1)
	for i, vault := range realVaults {
		label := fmt.Sprintf("%d:%s", i+1, vault)
		if vault == m.currentFilter {
			tabs = append(tabs, m.styles.tabActive.Render(" "+label+" "))
		} else {
			tabs = append(tabs, m.styles.tabInactive.Render(" "+label+" "))
		}
	}

	if m.currentFilter == allVaultsLabel {
		tabs = append([]string{m.styles.tabActive.Render(" All ")}, tabs...)
	} else {
		tabs = append([]string{m.styles.tabInactive.Render(" All ")}, tabs...)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m Model) searchView() string {
	if m.mode != modeSearch && m.search.Value() == "" {
		return m.styles.subtle.Render("Press / to search across visible keys")
	}
	count := len(m.list.Items())
	suffix := " match"
	if count != 1 {
		suffix += "es"
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, m.search.View(), m.styles.subtle.Render(fmt.Sprintf(" (%d%s)", count, suffix)))
}

func (m Model) previewView() string {
	lines := []string{chiefsBorder(max(18, m.width/2-10), m.styles), m.styles.header.Render("Preview")}
	if item, ok := m.selectedEntry(); ok {
		protection := protectionLabel(item.Protection)
		category := strings.ToUpper(prefixOf(item.Key))
		valueDisplay := maskedValue(item, m.preview)
		isRevealed := m.preview.revealed && m.preview.vault == item.Vault && m.preview.key == item.Key

		lines = append(lines,
			m.styles.subtle.Render("Key"),
			m.styles.previewTitle.Render(item.Key),
			"",
			m.styles.subtle.Render("Value"),
			m.styles.revealed.Render(valueDisplay),
		)
		if !isRevealed {
			lines = append(lines, m.styles.subtle.Render("[Enter to reveal]"))
		}
		if isRevealed {
			lines = append(lines, m.styles.subtle.Render(fmt.Sprintf("Size: %d chars", len(m.preview.value))))
		}

		lines = append(lines,
			"",
			m.styles.subtle.Render("─── Details ───"),
			"",
			m.styles.subtle.Render("Vault"),
			m.styles.normal.Render(item.Vault),
			"",
			m.styles.subtle.Render("Category"),
			m.styles.normal.Render(category),
			"",
			m.styles.subtle.Render("Protection"),
			m.styles.normal.Render(protection),
			"",
			m.styles.subtle.Render("Modified"),
			m.styles.normal.Render(renderModified(item.Modified)),
			"",
			m.styles.subtle.Render("─── Actions ───"),
			m.styles.help.Render("[Enter] Reveal  [c] Copy  [e] Edit  [d] Delete"),
		)
	} else {
		lines = append(lines, m.styles.subtle.Render("No key selected"))
	}
	if m.err != nil {
		lines = append(lines, "", m.styles.error.Render(m.err.Error()))
	}
	return m.styles.preview.Render(strings.Join(lines, "\n"))
}

func (m Model) overlayView() string {
	if m.mode == modeConfirmDelete {
		item, _ := m.selectedEntry()
		return m.styles.overlay.Render(
			m.styles.header.Render("Delete key") + "\n\n" +
				fmt.Sprintf("Delete %s from %s? (y/n)", item.Key, item.Vault),
		)
	}

	if m.form.confirming {
		vault := strings.TrimSpace(m.form.vault.Value())
		if vault == "" {
			vault = m.activeVault
		}
		key := strings.TrimSpace(m.form.key.Value())
		prot := "🔐 protected"
		if !m.form.isProtected {
			prot = "🔓 unprotected"
		}
		return m.styles.overlay.Render(
			m.styles.header.Render("Confirm Save") + "\n\n" +
				fmt.Sprintf("Save %s to vault:%s (%s)?", key, vault, prot) + "\n\n" +
				m.styles.help.Render("[Enter] confirm / [Esc] cancel"),
		)
	}

	title := "Add key"
	if m.mode == modeEdit {
		title = "Edit key"
	}

	vaultVal := strings.TrimSpace(m.form.vault.Value())
	if vaultVal == "" {
		vaultVal = m.activeVault
	}

	vaultHint := m.styles.subtle.Render("(new vault)")
	if m.vaultExists(vaultVal) {
		vaultHint = m.styles.success.Render("existing vault")
	}
	vaultNames := m.vaultHints()
	vaultList := m.vaultListView(vaultNames)

	keyVal := strings.TrimSpace(m.form.key.Value())
	keyNamingHint := m.styles.subtle.Render("Use UPPER_SNAKE_CASE")
	keyWarning := ""
	if keyVal != "" && m.keyExists(vaultVal, keyVal) && m.mode == modeAdd {
		keyWarning = m.styles.warning.Render("⚠ Key exists, will overwrite")
	}

	protChecked := "[ ] "
	if m.form.isProtected {
		protChecked = "[x] "
	}

	protStyle := m.styles.normal
	if m.form.focus == 3 {
		protStyle = m.styles.selected
	}

	content := []string{
		m.styles.header.Render(title),
		"",
		m.formLabel("Vault", 0),
		m.form.vault.View(),
		vaultList,
		vaultHint,
		"",
		m.formLabel("Key", 1),
		m.form.key.View(),
		keyNamingHint,
		keyWarning,
		"",
		m.formLabel("Value", 2) + m.styles.subtle.Render(" (F2 to reveal)"),
		m.form.value.View(),
		"",
		protStyle.Render(protChecked+"Touch ID protected") + m.styles.subtle.Render(" (Space toggle)"),
		"",
		m.styles.activeHelp.Render("Tab: next field | Esc: cancel | Enter: confirm"),
	}
	return m.styles.overlay.Render(strings.Join(content, "\n"))
}

func (m Model) formLabel(label string, focus int) string {
	if m.form.focus == focus {
		return m.styles.focusedLabel.Render("→ " + label)
	}
	return m.styles.inactiveLabel.Render(label)
}

func (m Model) vaultListView(vaultNames []string) string {
	if len(vaultNames) == 0 || m.form.focus != 0 {
		return ""
	}

	lines := make([]string, 0, len(vaultNames)+1)
	lines = append(lines, m.styles.activeHelp.Render("Available vaults:"))
	for i, vault := range vaultNames {
		if i >= 9 {
			break
		}
		option := m.styles.vaultOption.Render(fmt.Sprintf("%d. %s", i+1, vault))
		if vault == m.activeVault {
			option = m.styles.vaultDefault.Render(fmt.Sprintf("%d. %s (default)", i+1, vault))
		}
		lines = append(lines, option)
	}
	return strings.Join(lines, "\n")
}

func (m Model) helpView() string {
	parts := make([]string, 0, len(m.keys.ShortHelp()))
	for _, binding := range m.keys.ShortHelp() {
		help := binding.Help()
		parts = append(parts, help.Key+" "+help.Desc)
	}
	return m.styles.help.Render(strings.Join(parts, " • "))
}

func (m Model) welcomeView() string {
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.welcomeTitle.Render("No secrets yet! Get started:"),
		"",
		"  "+m.styles.welcomeKey.Render("kc set API_KEY")+"        "+m.styles.welcomeDesc.Render("Store a secret (Touch ID protected)"),
		"  "+m.styles.welcomeKey.Render("kc import .env")+"        "+m.styles.welcomeDesc.Render("Import from .env file"),
		"  "+m.styles.welcomeKey.Render("kc setup")+"              "+m.styles.welcomeDesc.Render("Migrate from your shell config"),
		"",
		m.styles.subtle.Render("Or press `a` to add a secret right here."),
	)
	return m.styles.app.Render(lipgloss.Place(max(m.width, 80), max(m.height, 24), lipgloss.Center, lipgloss.Center, m.styles.welcome.Render(content)))
}

func (m Model) helpOverlayView() string {
	sections := []struct {
		title string
		binds []string
	}{
		{"Navigation", []string{
			"j/k        Up / Down",
			"g/G        Top / Bottom",
			"Tab        Next vault filter",
			"/          Search",
		}},
		{"Actions", []string{
			"Enter      Reveal / Copy revealed",
			"c          Copy (reveal + clipboard)",
			"a          Add new key",
			"e          Edit selected key",
			"d          Delete selected key",
		}},
		{"Vaults", []string{
			"Tab        Next vault filter",
			"Shift+Tab  Previous vault filter",
			"1-9        Quick-switch vault",
		}},
		{"General", []string{
			"? or Esc   Close this help",
			"q          Quit",
		}},
	}

	var lines []string
	lines = append(lines, m.styles.header.Render("Keyboard Shortcuts"))
	lines = append(lines, "")
	for _, section := range sections {
		lines = append(lines, m.styles.previewTitle.Render(section.title))
		for _, bind := range section.binds {
			lines = append(lines, m.styles.normal.Render("  "+bind))
		}
		lines = append(lines, "")
	}
	lines = append(lines, m.styles.subtle.Render("Press ? or Esc to close"))
	return m.styles.helpOverlay.Render(strings.Join(lines, "\n"))
}

func (m Model) createVaultView() string {
	content := []string{
		m.styles.header.Render("Create Vault"),
		"",
		m.styles.previewTitle.Render("Name"),
		m.vaultNameInput.View(),
		"",
		m.styles.subtle.Render("Use lowercase alphanumeric, dash, or underscore"),
		"",
		m.styles.activeHelp.Render("Enter: create | Esc: cancel"),
	}
	return m.styles.overlay.Render(strings.Join(content, "\n"))
}

func (m Model) vaultPickerView() string {
	query := strings.TrimSpace(m.vaultPickerInput.Value())
	realVaults := m.vaultHints()

	type vaultRow struct {
		name  string
		count int
	}

	var filtered []vaultRow
	if query == "" {
		for _, v := range realVaults {
			filtered = append(filtered, vaultRow{name: v, count: m.vaultKeyCount(v)})
		}
	} else {
		matches := fuzzy.Find(strings.ToLower(query), realVaults)
		for _, match := range matches {
			v := realVaults[match.Index]
			filtered = append(filtered, vaultRow{name: v, count: m.vaultKeyCount(v)})
		}
	}

	content := []string{
		m.styles.header.Render("Switch Vault"),
		"",
		m.vaultPickerInput.View(),
		"",
	}

	if len(filtered) == 0 {
		content = append(content, m.styles.subtle.Render("No matching vaults"))
	} else {
		for _, row := range filtered {
			label := "keys"
			if row.count == 1 {
				label = "key"
			}
			line := fmt.Sprintf("  %s  (%d %s)", row.name, row.count, label)
			if row.name == m.currentFilter {
				content = append(content, m.styles.selected.Render(line))
			} else {
				content = append(content, m.styles.normal.Render(line))
			}
		}
	}

	content = append(content, "", m.styles.activeHelp.Render("Enter: select | Esc: cancel"))
	return m.styles.overlay.Render(strings.Join(content, "\n"))
}

func maskedValue(item entry, preview previewState) string {
	if preview.revealed && preview.vault == item.Vault && preview.key == item.Key {
		trimmed := strings.TrimSpace(preview.value)
		if trimmed == "" {
			return "[empty]"
		}
		return preview.value
	}
	return "••••••"
}

func protectionLabel(protection string) string {
	switch strings.ToLower(strings.TrimSpace(protection)) {
	case protectionUnprotected:
		return "🔓 Unprotected"
	case protectionProtected, "":
		return "🔐 Protected"
	default:
		return "🔐 Protected"
	}
}

func renderModified(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Unknown"
	}
	return value
}

func chiefsBorder(width int, styles styles) string {
	if width < 6 {
		width = 6
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		segment := "━"
		if i%2 == 0 {
			b.WriteString(styles.borderRed.Render(segment))
			continue
		}
		b.WriteString(styles.borderGold.Render(segment))
	}
	return b.String()
}
