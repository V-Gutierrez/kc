package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

// Version is one recorded previous value of a secret. The value never travels
// with it — only a digest, so a listing can be rendered without holding
// plaintext anywhere near the screen.
type Version struct {
	Seq       int
	Recorded  string
	Protected bool
	Digest    string
}

// History exposes the versions kc recorded before each overwrite. It is
// optional: a Deps without it simply has no history view.
type History interface {
	Versions(vault, key string) ([]Version, error)
	Value(vault, key string, seq int) (string, error)
	Rollback(vault, key string, seq int) error
}

type Deps struct {
	Store         Store
	Vaults        Vaults
	Clipboard     Clipboard
	History       History
	InitialFilter string
	// PinnedVault is the vault a .kc-vault marker fixes for this directory,
	// empty when none does. It cannot change while the TUI runs — the working
	// directory does not move — so it is resolved once at launch.
	PinnedVault string
	// RotationDays is the window past which a credential is shown as stale.
	// Zero falls back to defaultRotationDays.
	RotationDays int
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

// staleBadge marks a credential past its rotation window in the list.
const staleBadge = "⟳"

// defaultRotationDays matches the audit rule's default.
const defaultRotationDays = 180

// credentialSuffixes are the name endings that make a secret a credential worth
// rotating. A feature flag going untouched for a year is not a finding.
var credentialSuffixes = []string{"_KEY", "_TOKEN", "_SECRET", "_PASSWORD"}

// keyAge returns how many days ago the secret was last written, and whether the
// timestamp could be read at all. An unparseable or missing timestamp is not a
// claim about age — it is the absence of one.
func keyAge(modified string) (int, bool) {
	stamp := strings.TrimSpace(modified)
	if stamp == "" {
		return 0, false
	}
	when, err := time.Parse("2006-01-02 15:04", stamp)
	if err != nil {
		return 0, false
	}
	return int(time.Since(when).Hours() / 24), true
}

// isStaleCredential reports whether a key is a credential that has not been
// written inside the rotation window. It reads only the metadata the list
// already holds, so flagging costs no Keychain reads and no Touch ID prompt.
func isStaleCredential(item entry, rotationDays int) bool {
	if rotationDays <= 0 {
		rotationDays = defaultRotationDays
	}
	name := strings.ToUpper(item.Key)
	isCredential := false
	for _, suffix := range credentialSuffixes {
		if strings.HasSuffix(name, suffix) {
			isCredential = true
			break
		}
	}
	if !isCredential {
		return false
	}
	days, ok := keyAge(item.Modified)
	return ok && days > rotationDays
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
	modeHistory
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

// historyState is the version browser for one secret.
type historyState struct {
	vault      string
	key        string
	versions   []Version
	cursor     int
	loading    bool
	confirming bool
}

// selected returns the version under the cursor.
func (h historyState) selected() (Version, bool) {
	if h.cursor < 0 || h.cursor >= len(h.versions) {
		return Version{}, false
	}
	return h.versions[h.cursor], true
}

type formState struct {
	vault       textinput.Model
	key         textinput.Model
	value       textinput.Model
	focus       int
	isProtected bool
	confirming  bool
	// origin is the entry being edited, nil in add mode. It is what makes a
	// rename a move instead of a copy, and what the value is restored from
	// when the user leaves the value field untouched.
	origin *entry
	// valueSeeded records that the form opened with the real current value
	// (only possible when it was revealed first). When it is false, an empty
	// value field means "unchanged", never "blank the secret".
	valueSeeded bool
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
	// removed is set when the save was a move: the origin row no longer exists.
	removed *entry
}

type deletedMsg struct {
	entry entry
}

type historyLoadedMsg struct {
	vault    string
	key      string
	versions []Version
	err      error
}

type historyValueMsg struct {
	seq   int
	value string
}

type rolledBackMsg struct {
	vault string
	key   string
	seq   int
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
	formError        string
	history          historyState
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
		if sameEntry(existing, item) {
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
		if !sameEntry(existing, item) {
			filtered = append(filtered, existing)
		}
	}
	m.entries = filtered
}

// sameEntry identifies a secret by where it lives, not by the mutable
// attributes around it: a protection toggle edits a row, it does not create one.
func sameEntry(a, b entry) bool {
	return a.Vault == b.Vault && a.Key == b.Key
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

// newEditFormState builds the form for an existing secret. It remembers where
// the secret came from so the submit can tell an in-place edit from a move, and
// whether the value field was seeded with the real value or left blank because
// the secret was never revealed.
func newEditFormState(origin entry, value string) formState {
	form := newFormState(origin.Vault, origin.Key, value)
	source := origin
	form.origin = &source
	form.valueSeeded = value != ""
	form.isProtected = origin.Protection != protectionUnprotected
	return form
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
