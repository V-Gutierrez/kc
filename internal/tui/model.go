package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
)

const allVaultsLabel = "All vaults"

const (
	protectionProtected   = "protected"
	protectionUnprotected = "unprotected"
	kcBanner              = `    ██╗  ██╗ ██████╗
    ██║ ██╔╝██╔════╝
    █████╔╝ ██║     
    ██╔═██╗ ██║     
    ██║  ██╗╚██████╗
    ╚═╝  ╚═╝ ╚═════╝`
)

type SecretMetadata struct {
	Key        string
	Vault      string
	Protection string
	Modified   string
}

type Store interface {
	Get(vault, key string) (string, error)
	Set(vault, key, value string) error
	SetWithProtection(vault, key, value string, protected bool) error
	Delete(vault, key string) error
	List(vault string) ([]string, error)
	ListMetadata(vault string) ([]SecretMetadata, error)
}

type Vaults interface {
	List() ([]string, error)
	Active() (string, error)
	Switch(name string) error
	Create(name string) error
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
	Vault      string
	Key        string
	Protection string
	Modified   string
}

type groupHeader struct {
	Vault string
	Count int
}

func (g groupHeader) FilterValue() string {
	return strings.ToLower(g.Vault)
}

func (e entry) FilterValue() string {
	return strings.ToLower(e.Key)
}

func (e entry) prefix() string {
	return prefixOf(e.Key)
}

type mode int

const (
	modeBrowse mode = iota
	modeSearch
	modeAdd
	modeEdit
	modeConfirmDelete
	modeHelp
	modeCreateVault
	modeVaultPicker
	modeCommandPalette
)

type previewState struct {
	vault    string
	key      string
	value    string
	revealed bool
}

type copyRecord struct {
	Vault string
	Key   string
}

type formState struct {
	vault       textinput.Model
	key         textinput.Model
	value       textinput.Model
	focus       int
	isProtected bool
	confirming  bool
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

type clearFlashMsg struct {
	token int
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

type vaultCreatedMsg struct {
	name string
}

type vimTimeoutMsg struct {
	token int
	key   string
}

type exportCompletedMsg struct {
	vault string
	path  string
	count int
}

type importCompletedMsg struct {
	vault string
	path  string
	count int
}

type errMsg struct{ err error }

type Model struct {
	deps             Deps
	list             list.Model
	search           textinput.Model
	commandInput     textinput.Model
	vaultNameInput   textinput.Model
	vaultPickerInput textinput.Model
	keys             keyMap
	styles           styles
	entries          []entry
	vaults           []string
	currentFilter    string
	activeVault      string
	mode             mode
	preview          previewState
	form             formState
	loading          bool
	status           string
	flashMessage     string
	flashToken       int
	copyHistory      []copyRecord
	bookmarks        map[string]bool
	err              error
	width            int
	height           int
	revealToken      int
	pendingVimKey    string
	pendingVimToken  int
	delegate         itemDelegate
}

func NewModel(deps Deps) Model {
	styles := newStyles()
	search := textinput.New()
	search.Placeholder = "Search keys"
	search.CharLimit = 128
	search.Width = 32
	search.Prompt = "search> "

	commandInput := textinput.New()
	commandInput.Placeholder = "vault | search | export | import"
	commandInput.CharLimit = 256
	commandInput.Width = 36
	commandInput.Prompt = ":"

	vaultInput := textinput.New()
	vaultInput.Placeholder = "vault-name"
	vaultInput.CharLimit = 64
	vaultInput.Width = 24
	vaultInput.Prompt = "new vault> "

	pickerInput := textinput.New()
	pickerInput.Placeholder = "filter vaults..."
	pickerInput.CharLimit = 64
	pickerInput.Width = 24
	pickerInput.Prompt = "> "

	m := Model{
		deps:             deps,
		keys:             defaultKeyMap(),
		styles:           styles,
		search:           search,
		commandInput:     commandInput,
		vaultNameInput:   vaultInput,
		vaultPickerInput: pickerInput,
		currentFilter:    allVaultsLabel,
		mode:             modeBrowse,
		loading:          true,
		bookmarks:        loadBookmarks(),
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

func (m *Model) applyFilters() {
	selected, hadSelection := m.selectedEntry()
	items := make([]entry, 0, len(m.entries))
	favorites := make([]entry, 0)
	regular := make([]entry, 0)
	for _, item := range m.entries {
		if m.currentFilter != allVaultsLabel && item.Vault != m.currentFilter {
			continue
		}
		items = append(items, item)
		if m.isBookmarked(item) {
			favorites = append(favorites, item)
		} else {
			regular = append(regular, item)
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		leftPrefix := items[i].prefix()
		rightPrefix := items[j].prefix()
		if leftPrefix != rightPrefix {
			return leftPrefix < rightPrefix
		}
		if items[i].Key != items[j].Key {
			return items[i].Key < items[j].Key
		}
		return items[i].Vault < items[j].Vault
	})

	query := strings.TrimSpace(m.search.Value())
	if query != "" {
		searchTargets := make([]string, len(items))
		for i, item := range items {
			searchTargets[i] = item.FilterValue()
		}
		matches := fuzzy.Find(strings.ToLower(query), searchTargets)
		matchedEntries := make([]entry, 0, len(matches))
		for _, match := range matches {
			matchedEntries = append(matchedEntries, items[match.Index])
		}

		if m.currentFilter == allVaultsLabel {
			sort.SliceStable(matchedEntries, func(i, j int) bool {
				if matchedEntries[i].Vault != matchedEntries[j].Vault {
					return matchedEntries[i].Vault < matchedEntries[j].Vault
				}
				return matchedEntries[i].Key < matchedEntries[j].Key
			})

			counts := make(map[string]int)
			for _, item := range matchedEntries {
				counts[item.Vault]++
			}
			if len(counts) <= 1 {
				filtered := make([]list.Item, 0, len(matchedEntries))
				for _, item := range matchedEntries {
					filtered = append(filtered, item)
				}
				m.list.SetItems(filtered)
				m.restoreSelection(filtered, selected, hadSelection)
				return
			}

			grouped := make([]list.Item, 0, len(matchedEntries)+len(counts))
			lastVault := ""
			for _, item := range matchedEntries {
				if item.Vault != lastVault {
					lastVault = item.Vault
					grouped = append(grouped, groupHeader{Vault: item.Vault, Count: counts[item.Vault]})
				}
				grouped = append(grouped, item)
			}
			m.list.SetItems(grouped)
			m.restoreSelection(grouped, selected, hadSelection)
			return
		}

		filtered := make([]list.Item, 0, len(matchedEntries))
		for _, item := range matchedEntries {
			filtered = append(filtered, item)
		}
		m.list.SetItems(filtered)
		m.restoreSelection(filtered, selected, hadSelection)
		return
	}

	visible := make([]list.Item, 0, len(items))
	if len(favorites) > 0 {
		sortEntries(favorites)
		visible = append(visible, groupHeader{Vault: "⭐ Favorites", Count: len(favorites)})
		for _, item := range favorites {
			visible = append(visible, item)
		}
	}
	sortEntries(regular)
	for _, item := range regular {
		visible = append(visible, item)
	}
	m.list.SetItems(visible)
	m.restoreSelection(visible, selected, hadSelection)
}

func sortEntries(items []entry) {
	sort.SliceStable(items, func(i, j int) bool {
		leftPrefix := items[i].prefix()
		rightPrefix := items[j].prefix()
		if leftPrefix != rightPrefix {
			return leftPrefix < rightPrefix
		}
		if items[i].Key != items[j].Key {
			return items[i].Key < items[j].Key
		}
		return items[i].Vault < items[j].Vault
	})
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
	for i, item := range items {
		if _, ok := item.(entry); ok {
			m.list.Select(i)
			return
		}
	}
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

func (m *Model) cycleVaultFilterReverse() {
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
	m.currentFilter = m.vaults[(idx-1+len(m.vaults))%len(m.vaults)]
	m.clearPreview()
	m.applyFilters()
}

func (m *Model) selectVaultByIndex(n int) bool {
	realVaults := m.vaultHints()
	if n < 0 || n >= len(realVaults) {
		return false
	}
	m.currentFilter = realVaults[n]
	m.clearPreview()
	m.applyFilters()
	return true
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
	m.applyFormInputStyles()
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
	vaultInput.Placeholder = vault
	if keyName != "" {
		vaultInput.SetValue(vault)
	}
	vaultInput.Prompt = "vault> "
	keyInput := textinput.New()
	keyInput.SetValue(keyName)
	keyInput.Prompt = "key> "
	valueInput := textinput.New()
	valueInput.SetValue(value)
	valueInput.Prompt = "value> "
	valueInput.EchoMode = textinput.EchoPassword
	valueInput.EchoCharacter = '•'
	form := formState{vault: vaultInput, key: keyInput, value: valueInput, focus: 2, isProtected: true}
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

func (m *Model) applyFormInputStyles() {
	focusedPrompt := m.styles.focusedLabel
	blurredPrompt := m.styles.inactiveLabel
	focusedText := m.styles.normal
	blurredText := m.styles.subtle

	m.form.vault.PromptStyle = blurredPrompt
	m.form.vault.TextStyle = blurredText
	m.form.vault.PlaceholderStyle = m.styles.subtle
	m.form.key.PromptStyle = blurredPrompt
	m.form.key.TextStyle = blurredText
	m.form.key.PlaceholderStyle = m.styles.subtle
	m.form.value.PromptStyle = blurredPrompt
	m.form.value.TextStyle = blurredText
	m.form.value.PlaceholderStyle = m.styles.subtle

	switch m.form.focus {
	case 0:
		m.form.vault.PromptStyle = focusedPrompt
		m.form.vault.TextStyle = focusedText
	case 1:
		m.form.key.PromptStyle = focusedPrompt
		m.form.key.TextStyle = focusedText
	case 2:
		m.form.value.PromptStyle = focusedPrompt
		m.form.value.TextStyle = focusedText
	}
}

func (m Model) quickSelectVault(input string) (string, bool) {
	if len(input) != 1 || input[0] < '1' || input[0] > '9' {
		return "", false
	}
	index := int(input[0] - '1')
	vaults := m.vaultHints()
	if index < 0 || index >= len(vaults) {
		return "", false
	}
	return vaults[index], true
}

func (m Model) vaultExists(name string) bool {
	for _, v := range m.vaults {
		if v == name && v != allVaultsLabel {
			return true
		}
	}
	return false
}

func (m Model) keyExists(vault, key string) bool {
	for _, e := range m.entries {
		if e.Vault == vault && e.Key == key {
			return true
		}
	}
	return false
}

func (m Model) vaultHints() []string {
	hints := make([]string, 0, len(m.vaults))
	for _, vault := range m.vaults {
		if vault == allVaultsLabel {
			continue
		}
		hints = append(hints, vault)
	}
	return hints
}

func (m Model) fuzzyMatchVault(query string) string {
	realVaults := m.vaultHints()
	if len(realVaults) == 0 {
		return ""
	}
	matches := fuzzy.Find(strings.ToLower(query), realVaults)
	if len(matches) == 0 {
		return ""
	}
	return realVaults[matches[0].Index]
}

func (m Model) vaultKeyCount(vault string) int {
	count := 0
	for _, e := range m.entries {
		if e.Vault == vault {
			count++
		}
	}
	return count
}

func (m Model) visibleEntryCount() int {
	count := 0
	for _, item := range m.list.Items() {
		if _, ok := item.(entry); ok {
			count++
		}
	}
	return count
}

func (m Model) currentVaultContext() string {
	if m.currentFilter != "" && m.currentFilter != allVaultsLabel {
		return m.currentFilter
	}
	if m.activeVault != "" {
		return m.activeVault
	}
	return "default"
}

func bookmarkKey(item entry) string {
	return item.Vault + "/" + item.Key
}

func (m Model) isBookmarked(item entry) bool {
	if m.bookmarks == nil {
		return false
	}
	return m.bookmarks[bookmarkKey(item)]
}

func (m *Model) toggleBookmark(item entry) error {
	if m.bookmarks == nil {
		m.bookmarks = make(map[string]bool)
	}
	key := bookmarkKey(item)
	if m.bookmarks[key] {
		delete(m.bookmarks, key)
	} else {
		m.bookmarks[key] = true
	}
	return saveBookmarks(m.bookmarks)
}

func (m *Model) recordCopy(item entry) {
	updated := []copyRecord{{Vault: item.Vault, Key: item.Key}}
	for _, existing := range m.copyHistory {
		if existing.Vault == item.Vault && existing.Key == item.Key {
			continue
		}
		updated = append(updated, existing)
		if len(updated) == 3 {
			break
		}
	}
	m.copyHistory = updated
}

func (m Model) copyHistorySummary() string {
	if len(m.copyHistory) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m.copyHistory))
	for _, item := range m.copyHistory {
		parts = append(parts, item.Key)
	}
	return "Copied: " + strings.Join(parts, ", ")
}

var bookmarksPath = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kc", "bookmarks.json")
}

func loadBookmarks() map[string]bool {
	path := bookmarksPath()
	if path == "" {
		return make(map[string]bool)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]bool)
	}
	var bookmarks map[string]bool
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return make(map[string]bool)
	}
	if bookmarks == nil {
		return make(map[string]bool)
	}
	return bookmarks
}

func saveBookmarks(bookmarks map[string]bool) error {
	path := bookmarksPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bookmarks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

type paletteCommand struct {
	Name  string
	Usage string
	Desc  string
}

func paletteCommands() []paletteCommand {
	return []paletteCommand{
		{Name: "vault", Usage: "vault [name]", Desc: "switch vault or open picker"},
		{Name: "search", Usage: "search [query]", Desc: "search across all vaults"},
		{Name: "export", Usage: "export [file]", Desc: "export current vault to .env"},
		{Name: "import", Usage: "import <file>", Desc: "import .env into current vault"},
	}
}

func (m Model) matchingPaletteCommands() []paletteCommand {
	raw := strings.TrimSpace(m.commandInput.Value())
	raw = strings.TrimPrefix(raw, ":")
	if raw == "" {
		return paletteCommands()
	}
	name, _, _ := strings.Cut(raw, " ")
	commands := paletteCommands()
	filtered := make([]paletteCommand, 0, len(commands))
	for _, cmd := range commands {
		if strings.HasPrefix(cmd.Name, name) || strings.Contains(cmd.Usage, raw) {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) == 0 {
		return commands
	}
	return filtered
}

func prefixOf(key string) string {
	idx := strings.Index(key, "_")
	if idx > 0 {
		return strings.ToLower(key[:idx])
	}
	return "other"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
