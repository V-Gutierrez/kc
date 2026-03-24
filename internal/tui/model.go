package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

const allVaultsLabel = "All vaults"

type Store interface {
	Get(vault, key string) (string, error)
	Set(vault, key, value string) error
	SetWithProtection(vault, key, value string, protected bool) error
	Delete(vault, key string) error
	List(vault string) ([]string, error)
}

type Vaults interface {
	List() ([]string, error)
	Active() (string, error)
	Switch(name string) error
}

type Clipboard interface {
	Copy(value string) error
}

type Deps struct {
	Store         Store
	Vaults        Vaults
	Clipboard     Clipboard
	InitialFilter string
}

type entry struct {
	Vault string
	Key   string
}

func (e entry) FilterValue() string {
	return strings.ToLower(e.Key)
}

type mode int

const (
	modeBrowse mode = iota
	modeSearch
	modeAdd
	modeEdit
	modeConfirmDelete
)

type previewState struct {
	vault    string
	key      string
	value    string
	revealed bool
}

type formState struct {
	vault textinput.Model
	key   textinput.Model
	value textinput.Model
	focus int
}

type loadedMsg struct {
	vaults      []string
	activeVault string
	items       []entry
	err         error
}

type revealedMsg struct {
	entry entry
	value string
}

type copiedMsg struct {
	entry entry
	value string
}

type savedMsg struct {
	entry entry
	value string
}

type deletedMsg struct {
	entry entry
}

type hideMsg struct {
	entry entry
	token int
}

type errMsg struct{ err error }

type Model struct {
	deps          Deps
	list          list.Model
	search        textinput.Model
	keys          keyMap
	styles        styles
	entries       []entry
	vaults        []string
	currentFilter string
	activeVault   string
	mode          mode
	preview       previewState
	form          formState
	loading       bool
	status        string
	err           error
	width         int
	height        int
	revealToken   int
	delegate      itemDelegate
}

func NewModel(deps Deps) Model {
	styles := newStyles()
	search := textinput.New()
	search.Placeholder = "Search keys"
	search.CharLimit = 128
	search.Width = 32
	search.Prompt = "search> "

	m := Model{
		deps:          deps,
		keys:          defaultKeyMap(),
		styles:        styles,
		search:        search,
		currentFilter: allVaultsLabel,
		mode:          modeBrowse,
		loading:       true,
	}
	delegate := itemDelegate{styles: &m.styles, model: &m}
	m.delegate = delegate
	m.list = list.New([]list.Item{}, delegate, 0, 0)
	m.list.Title = "kc"
	m.list.SetShowHelp(false)
	m.list.SetShowStatusBar(false)
	m.list.SetFilteringEnabled(false)
	return m
}

func Run(deps Deps) error {
	_, err := tea.NewProgram(NewModel(deps), tea.WithAltScreen()).Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return loadEntriesCmd(m.deps)
}

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
		return m, tea.Tick(10*time.Second, func(_ time.Time) tea.Msg {
			return hideMsg{entry: msg.entry, token: m.revealToken}
		})
	case hideMsg:
		if m.preview.revealed && msg.token == m.revealToken && m.preview.vault == msg.entry.Vault && m.preview.key == msg.entry.Key {
			m.clearPreview()
			m.status = "Value hidden"
		}
		return m, nil
	case copiedMsg:
		m.status = fmt.Sprintf("Copied %s from %s", msg.entry.Key, msg.entry.Vault)
		return m, nil
	case savedMsg:
		m.upsertEntry(msg.entry)
		m.clearPreview()
		m.mode = modeBrowse
		m.status = fmt.Sprintf("Saved %s in %s", msg.entry.Key, msg.entry.Vault)
		m.applyFilters()
		return m, nil
	case deletedMsg:
		m.removeEntry(msg.entry)
		m.clearPreview()
		m.mode = modeBrowse
		m.status = fmt.Sprintf("Deleted %s from %s", msg.entry.Key, msg.entry.Vault)
		m.applyFilters()
		return m, nil
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

func (m Model) View() string {
	if m.loading {
		return m.styles.app.Render(m.styles.header.Render("kc") + "\n\nLoading vaults and keys...")
	}

	left := lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(),
		m.searchView(),
		m.list.View(),
		m.helpView(),
	)

	right := m.previewView()
	if m.mode == modeAdd || m.mode == modeEdit || m.mode == modeConfirmDelete {
		right = m.overlayView()
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(max(40, m.width/2)).Render(left),
		lipgloss.NewStyle().PaddingLeft(2).Width(max(30, m.width/2-4)).Render(right),
	)

	statusBar := m.statusView()
	return m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, body, "\n", statusBar))
}

func (m Model) statusView() string {
	status := m.status
	if status == "" {
		status = "Ready"
	}
	return m.styles.status.Render(status)
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
	}

	switch {
	case key.Matches(msg, m.keys.Search):
		m.mode = modeSearch
		m.search.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.VaultNext):
		m.cycleVaultFilter()
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
	if key.Matches(msg, m.keys.Cancel) {
		m.mode = modeBrowse
		m.clearPreview()
		return m, nil
	}
	if key.Matches(msg, m.keys.Confirm) {
		next, cmd := m.submitForm()
		return next, cmd
	}
	if msg.String() == "tab" {
		m.form.focus = (m.form.focus + 1) % 3
		m.focusForm()
		return m, nil
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

func (m Model) submitForm() (Model, tea.Cmd) {
	vault := strings.TrimSpace(m.form.vault.Value())
	keyName := strings.TrimSpace(m.form.key.Value())
	value := m.form.value.Value()
	if vault == "" {
		vault = m.activeVault
	}
	entry := entry{Vault: vault, Key: keyName}
	return m, saveCmd(m.deps, entry, value)
}

func (m *Model) applyFilters() {
	selected, hadSelection := m.selectedEntry()
	items := make([]entry, 0, len(m.entries))
	for _, item := range m.entries {
		if m.currentFilter != allVaultsLabel && item.Vault != m.currentFilter {
			continue
		}
		items = append(items, item)
	}

	query := strings.TrimSpace(m.search.Value())
	if query != "" {
		searchTargets := make([]string, len(items))
		for i, item := range items {
			searchTargets[i] = item.FilterValue()
		}
		matches := fuzzy.Find(strings.ToLower(query), searchTargets)
		filtered := make([]list.Item, 0, len(matches))
		for _, match := range matches {
			filtered = append(filtered, items[match.Index])
		}
		m.list.SetItems(filtered)
		m.restoreSelection(filtered, selected, hadSelection)
		return
	}

	visible := make([]list.Item, 0, len(items))
	for _, item := range items {
		visible = append(visible, item)
	}
	m.list.SetItems(visible)
	m.restoreSelection(visible, selected, hadSelection)
}

func (m *Model) restoreSelection(items []list.Item, selected entry, hadSelection bool) {
	if len(items) == 0 {
		return
	}
	if hadSelection {
		for i, item := range items {
			candidate, ok := item.(entry)
			if ok && candidate == selected {
				m.list.Select(i)
				return
			}
		}
	}
	m.list.Select(0)
}

func (m *Model) cycleVaultFilter() {
	if len(m.vaults) == 0 {
		return
	}
	idx := 0
	for i, vault := range m.vaults {
		if vault == m.currentFilter {
			idx = i
			break
		}
	}
	m.currentFilter = m.vaults[(idx+1)%len(m.vaults)]
	m.clearPreview()
	m.applyFilters()
}

func (m *Model) clearPreview() {
	m.preview = previewState{}
	m.revealToken++
}

func (m *Model) upsertEntry(item entry) {
	for i, existing := range m.entries {
		if existing == item {
			m.entries[i] = item
			return
		}
	}
	m.entries = append(m.entries, item)
	sort.Slice(m.entries, func(i, j int) bool {
		if m.entries[i].Vault == m.entries[j].Vault {
			return m.entries[i].Key < m.entries[j].Key
		}
		return m.entries[i].Vault < m.entries[j].Vault
	})
}

func (m *Model) removeEntry(item entry) {
	filtered := m.entries[:0]
	for _, existing := range m.entries {
		if existing != item {
			filtered = append(filtered, existing)
		}
	}
	m.entries = filtered
}

func (m Model) selectedEntry() (entry, bool) {
	selected := m.list.SelectedItem()
	item, ok := selected.(entry)
	return item, ok
}

func (m *Model) focusForm() {
	m.form.vault.Blur()
	m.form.key.Blur()
	m.form.value.Blur()
	switch m.form.focus {
	case 0:
		m.form.vault.Focus()
	case 1:
		m.form.key.Focus()
	case 2:
		m.form.value.Focus()
	}
}

func newFormState(vault, keyName, value string) formState {
	vaultInput := textinput.New()
	vaultInput.SetValue(vault)
	vaultInput.Prompt = "vault> "
	keyInput := textinput.New()
	keyInput.SetValue(keyName)
	keyInput.Prompt = "key> "
	valueInput := textinput.New()
	valueInput.SetValue(value)
	valueInput.Prompt = "value> "
	form := formState{vault: vaultInput, key: keyInput, value: valueInput, focus: 2}
	if keyName == "" {
		form.focus = 1
	}
	form.vault.CharLimit = 64
	form.key.CharLimit = 128
	form.value.CharLimit = 4096
	form.vault.Width = 24
	form.key.Width = 24
	form.value.Width = 30
	form.vault.Focus()
	form.vault.Blur()
	form.key.Blur()
	form.value.Blur()
	switch form.focus {
	case 1:
		form.key.Focus()
	case 2:
		form.value.Focus()
	default:
		form.vault.Focus()
	}
	return form
}

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
			keys, err := deps.Store.List(vault)
			if err != nil {
				return loadedMsg{err: err}
			}
			sort.Strings(keys)
			for _, key := range keys {
				items = append(items, entry{Vault: vault, Key: key})
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
		if err := deps.Store.SetWithProtection(item.Vault, item.Key, value, true); err != nil {
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

func (m Model) headerView() string {
	filter := m.currentFilter
	if filter == "" {
		filter = allVaultsLabel
	}
	count := len(m.list.Items())
	return lipgloss.JoinVertical(lipgloss.Left,
		m.styles.header.Render("kc interactive"),
		m.styles.subtle.Render(fmt.Sprintf("Vault filter: %s • Active vault: %s • (%d items)", filter, m.activeVault, count)),
	)
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
	lines := []string{m.styles.header.Render("Preview")}
	if item, ok := m.selectedEntry(); ok {
		lines = append(lines,
			m.styles.subtle.Render("Vault: "+item.Vault),
			m.styles.subtle.Render("Key: "+item.Key),
			"",
			m.styles.revealed.Render(maskedValue(item, m.preview)),
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
	title := "Add key"
	if m.mode == modeEdit {
		title = "Edit key"
	}
	content := []string{
		m.styles.header.Render(title),
		"",
		m.styles.inputLabel.Render("Vault"),
		m.form.vault.View(),
		"",
		m.styles.inputLabel.Render("Key"),
		m.form.key.View(),
		"",
		m.styles.inputLabel.Render("Value"),
		m.form.value.View(),
		"",
		m.styles.help.Render("Enter save • Esc cancel • Tab next field"),
	}
	return m.styles.overlay.Render(strings.Join(content, "\n"))
}

func (m Model) helpView() string {
	parts := make([]string, 0, len(m.keys.ShortHelp()))
	for _, binding := range m.keys.ShortHelp() {
		help := binding.Help()
		parts = append(parts, help.Key+" "+help.Desc)
	}
	return m.styles.help.Render(strings.Join(parts, " • "))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
