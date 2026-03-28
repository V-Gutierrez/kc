package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type mockStore struct {
	keys     map[string][]string
	values   map[string]map[string]string
	metadata map[string]map[string]string
	listErr  error
	getCalls []storeCall
	setCalls []setCall
	delCalls []storeCall
}

type storeCall struct {
	vault string
	key   string
}

type setCall struct {
	vault     string
	key       string
	value     string
	protected bool
}

func newMockStore() *mockStore {
	return &mockStore{
		keys:     make(map[string][]string),
		values:   make(map[string]map[string]string),
		metadata: make(map[string]map[string]string),
	}
}

func (m *mockStore) Get(vault, key string) (string, error) {
	m.getCalls = append(m.getCalls, storeCall{vault: vault, key: key})
	return m.values[vault][key], nil
}

func (m *mockStore) Set(vault, key, value string) error {
	return m.SetWithProtection(vault, key, value, true)
}

func (m *mockStore) SetWithProtection(vault, key, value string, protected bool) error {
	m.setCalls = append(m.setCalls, setCall{vault: vault, key: key, value: value, protected: protected})
	if m.values[vault] == nil {
		m.values[vault] = make(map[string]string)
	}
	if m.metadata[vault] == nil {
		m.metadata[vault] = make(map[string]string)
	}
	m.values[vault][key] = value
	if protected {
		m.metadata[vault][key] = protectionProtected
	} else {
		m.metadata[vault][key] = protectionUnprotected
	}
	m.ensureKey(vault, key)
	return nil
}

func (m *mockStore) Delete(vault, key string) error {
	m.delCalls = append(m.delCalls, storeCall{vault: vault, key: key})
	if m.values[vault] != nil {
		delete(m.values[vault], key)
	}
	if m.metadata[vault] != nil {
		delete(m.metadata[vault], key)
	}
	keys := m.keys[vault][:0]
	for _, existing := range m.keys[vault] {
		if existing != key {
			keys = append(keys, existing)
		}
	}
	m.keys[vault] = keys
	return nil
}

func (m *mockStore) List(vault string) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	keys := append([]string(nil), m.keys[vault]...)
	return keys, nil
}

func (m *mockStore) ListMetadata(vault string) ([]SecretMetadata, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	keys := m.keys[vault]
	metas := make([]SecretMetadata, len(keys))
	for i, key := range keys {
		protection := m.metadata[vault][key]
		if protection == "" {
			protection = protectionProtected
		}
		metas[i] = SecretMetadata{Key: key, Vault: vault, Protection: protection}
	}
	return metas, nil
}

func (m *mockStore) ensureKey(vault, key string) {
	for _, existing := range m.keys[vault] {
		if existing == key {
			return
		}
	}
	m.keys[vault] = append(m.keys[vault], key)
}

type mockVaults struct {
	list        []string
	active      string
	switchCalls []string
	createCalls []string
	deleteCalls []vaultDeleteCall
}

type vaultDeleteCall struct {
	name  string
	force bool
}

func (m *mockVaults) List() ([]string, error) { return append([]string(nil), m.list...), nil }
func (m *mockVaults) Active() (string, error) { return m.active, nil }
func (m *mockVaults) Switch(name string) error {
	m.active = name
	m.switchCalls = append(m.switchCalls, name)
	return nil
}
func (m *mockVaults) Create(name string) error {
	m.createCalls = append(m.createCalls, name)
	m.list = append(m.list, name)
	return nil
}
func (m *mockVaults) Delete(name string, force bool) error {
	m.deleteCalls = append(m.deleteCalls, vaultDeleteCall{name: name, force: force})
	filtered := m.list[:0]
	for _, vault := range m.list {
		if vault != name {
			filtered = append(filtered, vault)
		}
	}
	m.list = filtered
	if m.active == name {
		m.active = "default"
	}
	return nil
}

type mockClipboard struct {
	values []string
}

func (m *mockClipboard) Copy(value string) error {
	m.values = append(m.values, value)
	return nil
}

func TestModelInitLoadsMetadataWithoutSecretReads(t *testing.T) {
	store := newMockStore()
	store.keys["default"] = []string{"API_KEY"}
	store.values["default"] = map[string]string{"API_KEY": "secret-default"}
	store.metadata["default"] = map[string]string{"API_KEY": protectionProtected}
	store.keys["prod"] = []string{"API_KEY"}
	store.values["prod"] = map[string]string{"API_KEY": "secret-prod"}
	store.metadata["prod"] = map[string]string{"API_KEY": protectionUnprotected}

	m := NewModel(Deps{
		Store:         store,
		Vaults:        &mockVaults{list: []string{"default", "prod"}, active: "default"},
		Clipboard:     &mockClipboard{},
		InitialFilter: "",
	})

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil command")
	}

	msg := cmd()
	loaded, ok := msg.(loadedMsg)
	if !ok {
		t.Fatalf("Init() command returned %T, want loadedMsg", msg)
	}

	if len(loaded.items) != 2 {
		t.Fatalf("loaded items = %d, want 2", len(loaded.items))
	}
	if len(store.getCalls) != 0 {
		t.Fatalf("Get() calls = %d, want 0 during metadata load", len(store.getCalls))
	}
	if loaded.activeVault != "default" {
		t.Fatalf("active vault = %q, want default", loaded.activeVault)
	}
	if loaded.items[1].Protection != protectionUnprotected {
		t.Fatalf("protection = %q, want %q", loaded.items[1].Protection, protectionUnprotected)
	}
}

func TestUpdateLoadedMsgPopulatesAllVaultView(t *testing.T) {
	m := NewModel(Deps{})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"default", "prod"},
		activeVault: "default",
		items:       []entry{{Vault: "default", Key: "A"}, {Vault: "prod", Key: "B"}},
	})
	model := updated.(Model)

	if got := len(model.list.Items()); got != 2 {
		t.Fatalf("visible items = %d, want 2", got)
	}
	if model.currentFilter != allVaultsLabel {
		t.Fatalf("current filter = %q, want %q", model.currentFilter, allVaultsLabel)
	}
	if selected, ok := model.list.SelectedItem().(entry); !ok || selected.Vault != "default" || selected.Key != "A" {
		t.Fatalf("selected item = %#v, want default/A", model.list.SelectedItem())
	}
}

func TestRevealUsesVaultAndKeyIdentity(t *testing.T) {
	store := newMockStore()
	store.values["default"] = map[string]string{"API_KEY": "default-secret"}
	store.values["prod"] = map[string]string{"API_KEY": "prod-secret"}
	m := NewModel(Deps{Store: store, Clipboard: &mockClipboard{}})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"default", "prod"},
		activeVault: "default",
		items:       []entry{{Vault: "default", Key: "API_KEY"}, {Vault: "prod", Key: "API_KEY"}},
	})
	model := updated.(Model)
	model.list.Select(1)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("Enter did not return reveal command")
	}
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)

	if len(store.getCalls) != 1 {
		t.Fatalf("Get() calls = %d, want 1", len(store.getCalls))
	}
	if got := store.getCalls[0]; got.vault != "prod" || got.key != "API_KEY" {
		t.Fatalf("Get() called with %#v, want prod/API_KEY", got)
	}
	if !model.preview.revealed || model.preview.value != "prod-secret" {
		t.Fatalf("preview = %#v, want revealed prod-secret", model.preview)
	}
}

func TestSearchChangesVisibleItemsWithoutSecretReads(t *testing.T) {
	store := newMockStore()
	m := NewModel(Deps{Store: store})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"default", "prod"},
		activeVault: "default",
		items:       []entry{{Vault: "default", Key: "API_KEY"}, {Vault: "prod", Key: "DB_PASS"}},
	})
	model := updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)

	if got := len(model.list.Items()); got != 1 {
		t.Fatalf("visible items = %d, want 1 after search", got)
	}
	if len(store.getCalls) != 0 {
		t.Fatalf("Get() calls = %d, want 0 during search", len(store.getCalls))
	}
	if got := model.search.Value(); got != "d" {
		t.Fatalf("search value = %q, want d", got)
	}
}

func TestVaultCycleFiltersVisibleItems(t *testing.T) {
	m := NewModel(Deps{})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"default", "prod"},
		activeVault: "default",
		items:       []entry{{Vault: "default", Key: "A"}, {Vault: "prod", Key: "B"}},
	})
	model := updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if model.currentFilter != "default" {
		t.Fatalf("filter after first Tab = %q, want default", model.currentFilter)
	}
	if got := len(model.list.Items()); got != 1 {
		t.Fatalf("visible items after default filter = %d, want 1", got)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if model.currentFilter != "prod" {
		t.Fatalf("filter after second Tab = %q, want prod", model.currentFilter)
	}
}

func TestAddEditAndDeleteFlows(t *testing.T) {
	store := newMockStore()
	store.keys["prod"] = []string{"TOKEN"}
	store.values["prod"] = map[string]string{"TOKEN": "before"}
	m := NewModel(Deps{
		Store:         store,
		Vaults:        &mockVaults{list: []string{"default", "prod"}, active: "default"},
		Clipboard:     &mockClipboard{},
		InitialFilter: "",
	})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"default", "prod"},
		activeVault: "default",
		items:       []entry{{Vault: "prod", Key: "TOKEN"}},
	})
	model := updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)
	if model.mode != modeAdd {
		t.Fatalf("mode after add = %v, want modeAdd", model.mode)
	}

	model.form.vault.SetValue("default")
	model.form.key.SetValue("NEW_KEY")
	model.form.value.SetValue("new-value")
	nextModel, cmd := model.submitForm()
	model = nextModel
	msg := cmd()
	updatedTea, _ := model.Update(msg)
	model = updatedTea.(Model)
	if len(store.setCalls) == 0 {
		t.Fatal("expected set call for add flow")
	}

	updatedTea, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updatedTea.(Model)
	if model.mode != modeEdit {
		t.Fatalf("mode after edit = %v, want modeEdit", model.mode)
	}
	model.form.value.SetValue("after")
	nextModel, cmd = model.submitForm()
	model = nextModel
	msg = cmd()
	updatedTea, _ = model.Update(msg)
	model = updatedTea.(Model)
	if last := store.setCalls[len(store.setCalls)-1]; last.vault != "prod" || last.key != "TOKEN" || last.value != "after" || !last.protected {
		t.Fatalf("edit set call = %#v, want prod/TOKEN/after", last)
	}

	updatedTea, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updatedTea.(Model)
	if model.mode != modeConfirmDelete {
		t.Fatalf("mode after delete = %v, want modeConfirmDelete", model.mode)
	}
	updatedTea, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updatedTea.(Model)
	msg = cmd()
	updatedTea, _ = model.Update(msg)
	model = updatedTea.(Model)
	if len(store.delCalls) != 1 {
		t.Fatalf("delete calls = %d, want 1", len(store.delCalls))
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode after delete confirm = %v, want modeBrowse", model.mode)
	}
}

func TestDeleteConfirmCancelWithN(t *testing.T) {
	store := newMockStore()
	m := NewModel(Deps{Store: store})
	updated, _ := m.Update(loadedMsg{items: []entry{{Vault: "default", Key: "TOKEN"}}, activeVault: "default"})
	model := updated.(Model)
	model.mode = modeConfirmDelete

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("cancel should not return a command")
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse", model.mode)
	}
	if len(store.delCalls) != 0 {
		t.Fatalf("delete calls = %d, want 0", len(store.delCalls))
	}
}

func TestHandleFormKeyEscCancel(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	m.mode = modeAdd
	m.form = newFormState("default", "", "")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := updated.(Model)
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse", model.mode)
	}
}

func TestHandleFormKeyTabCyclesFocus(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	m.mode = modeAdd
	m.form = newFormState("default", "", "")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := updated.(Model)
	if model.form.focus != 2 {
		t.Fatalf("focus = %d, want 2", model.form.focus)
	}
}

func TestHandleFormKeyEnterSubmits(t *testing.T) {
	store := newMockStore()
	m := NewModel(Deps{Store: store})
	m.mode = modeAdd
	m.activeVault = "default"
	m.form = newFormState("default", "TOKEN", "secret")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)
	if cmd != nil {
		t.Fatal("expected nil command on first Enter (confirmation step)")
	}
	if !model.form.confirming {
		t.Fatal("expected confirming state")
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected save command on second Enter")
	}
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse", model.mode)
	}
	if len(store.setCalls) != 1 {
		t.Fatalf("set calls = %d, want 1", len(store.setCalls))
	}
}

func TestLoadEntriesCmdStoreListError(t *testing.T) {
	store := newMockStore()
	store.listErr = errors.New("list failed")
	msg := loadEntriesCmd(Deps{Store: store, Vaults: &mockVaults{list: []string{"default"}, active: "default"}})()
	loaded := msg.(loadedMsg)
	if loaded.err == nil {
		t.Fatal("expected error")
	}
}

func TestCopyCmd(t *testing.T) {
	store := newMockStore()
	store.keys["default"] = []string{"TOKEN"}
	store.values["default"] = map[string]string{"TOKEN": "secret"}
	clipboard := &mockClipboard{}
	m := NewModel(Deps{Store: store, Clipboard: clipboard})
	updated, _ := m.Update(loadedMsg{items: []entry{{Vault: "default", Key: "TOKEN"}}, activeVault: "default"})
	model := updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected copy command")
	}
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if clipboard.values[0] != "secret" {
		t.Fatalf("clipboard = %#v, want secret", clipboard.values)
	}
	if model.flashMessage == "" {
		t.Fatal("expected flash message")
	}
}

func TestMaskedValueRevealed(t *testing.T) {
	got := maskedValue(entry{Vault: "default", Key: "TOKEN"}, previewState{vault: "default", key: "TOKEN", value: "secret", revealed: true})
	if got != "secret" {
		t.Fatalf("maskedValue = %q, want secret", got)
	}
}

func TestMaskedValueRevealedEmpty(t *testing.T) {
	got := maskedValue(entry{Vault: "default", Key: "TOKEN"}, previewState{vault: "default", key: "TOKEN", value: "   ", revealed: true})
	if got != "[empty]" {
		t.Fatalf("maskedValue = %q, want [empty]", got)
	}
}

func TestCopyFlashMessageBehavior(t *testing.T) {
	store := newMockStore()
	store.keys["v"] = []string{"k"}
	store.values["v"] = map[string]string{"k": "secret"}
	m := NewModel(Deps{Store: store, Clipboard: &mockClipboard{}})
	updated, _ := m.Update(loadedMsg{
		items:       []entry{{Vault: "v", Key: "k"}},
		activeVault: "v",
	})
	model := updated.(Model)
	model.list.Select(0)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)

	if cmd == nil {
		t.Fatal("expected copy command")
	}

	msg := cmd()
	updated, cmd = model.Update(msg)
	model = updated.(Model)

	if !strings.Contains(model.flashMessage, "Copied to clipboard") {
		t.Errorf("expected flash message containing 'Copied to clipboard', got %q", model.flashMessage)
	}
	if cmd == nil {
		t.Fatal("expected tick command")
	}

	updated, _ = model.Update(clearFlashMsg{token: model.flashToken})
	model = updated.(Model)

	if model.flashMessage != "" {
		t.Errorf("expected flash message to clear, got %q", model.flashMessage)
	}
}

func TestPrefixOf(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{key: "AWS_ACCESS_KEY", want: "aws"},
		{key: "NOTION_TOKEN", want: "notion"},
		{key: "TOKEN", want: "other"},
		{key: "_HIDDEN", want: "other"},
	}

	for _, tt := range tests {
		if got := prefixOf(tt.key); got != tt.want {
			t.Fatalf("prefixOf(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestProtectionLabel(t *testing.T) {
	if got := protectionLabel(protectionProtected); got != "🔐 Protected" {
		t.Fatalf("protectionLabel(protected) = %q", got)
	}
	if got := protectionLabel(protectionUnprotected); got != "🔓 Unprotected" {
		t.Fatalf("protectionLabel(unprotected) = %q", got)
	}
}

func TestCrossVaultSearchGroupsResultsByVault(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"default", "prod"},
		activeVault: "default",
		items: []entry{
			{Vault: "default", Key: "STRIPE_LIVE"},
			{Vault: "default", Key: "AWS_KEY"},
			{Vault: "prod", Key: "STRIPE_WEBHOOK"},
		},
	})
	model := updated.(Model)
	model.currentFilter = allVaultsLabel
	model.search.SetValue("stripe")
	model.applyFilters()

	items := model.list.Items()
	if len(items) != 4 {
		t.Fatalf("grouped search items = %d, want 4", len(items))
	}
	if header, ok := items[0].(groupHeader); !ok || header.Vault != "default" || header.Count != 1 {
		t.Fatalf("first item = %#v, want default group header", items[0])
	}
	if _, ok := items[1].(entry); !ok {
		t.Fatalf("second item should be entry, got %#v", items[1])
	}
	if header, ok := items[2].(groupHeader); !ok || header.Vault != "prod" || header.Count != 1 {
		t.Fatalf("third item = %#v, want prod group header", items[2])
	}
	if model.visibleEntryCount() != 2 {
		t.Fatalf("visible entry count = %d, want 2", model.visibleEntryCount())
	}
}

func TestCommandPaletteSearchCommandSetsAllVaultSearch(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	updated, _ := m.Update(loadedMsg{vaults: []string{"default", "prod"}, activeVault: "default", items: []entry{{Vault: "default", Key: "STRIPE_LIVE"}, {Vault: "prod", Key: "STRIPE_WEBHOOK"}}})
	model := updated.(Model)
	model.mode = modeCommandPalette
	model.commandInput.SetValue("search stripe")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("search command should not return async cmd")
	}
	if model.currentFilter != allVaultsLabel {
		t.Fatalf("currentFilter = %q, want all vaults", model.currentFilter)
	}
	if model.search.Value() != "stripe" {
		t.Fatalf("search value = %q, want stripe", model.search.Value())
	}
}

func TestCommandPaletteVaultCommandSetsFilter(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	updated, _ := m.Update(loadedMsg{vaults: []string{"default", "prod"}, activeVault: "default", items: []entry{{Vault: "default", Key: "A"}, {Vault: "prod", Key: "B"}}})
	model := updated.(Model)
	model.mode = modeCommandPalette
	model.commandInput.SetValue("vault prod")

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.currentFilter != "prod" {
		t.Fatalf("currentFilter = %q, want prod", model.currentFilter)
	}
}

func TestCommandPaletteExportCommandWritesFile(t *testing.T) {
	store := newMockStore()
	store.keys["default"] = []string{"API_KEY"}
	store.values["default"] = map[string]string{"API_KEY": "secret"}
	store.metadata["default"] = map[string]string{"API_KEY": protectionProtected}
	path := filepath.Join(t.TempDir(), "export.env")

	m := NewModel(Deps{Store: store})
	m.activeVault = "default"
	m.currentFilter = "default"
	m.mode = modeCommandPalette
	m.commandInput.SetValue("export " + path)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)
	if cmd == nil {
		t.Fatal("expected export command")
	}
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(contents), "API_KEY=secret") {
		t.Fatalf("export file contents = %q", string(contents))
	}
	if !strings.Contains(model.flashMessage, "Exported 1 keys") {
		t.Fatalf("flash = %q", model.flashMessage)
	}
}

func TestCommandPaletteImportCommandReadsFile(t *testing.T) {
	store := newMockStore()
	path := filepath.Join(t.TempDir(), "import.env")
	if err := os.WriteFile(path, []byte("API_KEY=secret\nDB_PASS=hidden\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	m := NewModel(Deps{Store: store})
	m.activeVault = "default"
	m.currentFilter = "default"
	m.mode = modeCommandPalette
	m.commandInput.SetValue("import " + path)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)
	if cmd == nil {
		t.Fatal("expected import command")
	}
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if len(store.setCalls) != 2 {
		t.Fatalf("set calls = %d, want 2", len(store.setCalls))
	}
	if !strings.Contains(model.flashMessage, "Imported 2 keys") {
		t.Fatalf("flash = %q", model.flashMessage)
	}
}

func TestDoubleYYCopiesSelectedEntry(t *testing.T) {
	store := newMockStore()
	store.keys["default"] = []string{"TOKEN"}
	store.values["default"] = map[string]string{"TOKEN": "secret"}
	clipboard := &mockClipboard{}
	m := NewModel(Deps{Store: store, Clipboard: clipboard})
	updated, _ := m.Update(loadedMsg{items: []entry{{Vault: "default", Key: "TOKEN"}}, activeVault: "default"})
	model := updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected copy cmd from yy")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if len(clipboard.values) != 1 || clipboard.values[0] != "secret" {
		t.Fatalf("clipboard values = %v, want [secret]", clipboard.values)
	}
}

func TestDoubleCCEntersEditMode(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	updated, _ := m.Update(loadedMsg{items: []entry{{Vault: "default", Key: "TOKEN", Protection: protectionProtected}}, activeVault: "default"})
	model := updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected immediate copy command from first c")
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected blink cmd from cc")
	}
	if model.mode != modeEdit {
		t.Fatalf("mode after cc = %v, want modeEdit", model.mode)
	}
}

func TestDoubleDDEntersConfirmDeleteMode(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	updated, _ := m.Update(loadedMsg{items: []entry{{Vault: "default", Key: "TOKEN", Protection: protectionProtected}}, activeVault: "default"})
	model := updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	if model.mode != modeConfirmDelete {
		t.Fatalf("mode after first d = %v, want modeConfirmDelete", model.mode)
	}
	if model.pendingVimKey != "d" {
		t.Fatalf("pendingVimKey = %q, want d", model.pendingVimKey)
	}

	model.mode = modeBrowse
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("dd should not return async command")
	}
	if model.mode != modeConfirmDelete {
		t.Fatalf("mode after dd = %v, want modeConfirmDelete", model.mode)
	}
	if model.pendingVimKey != "" {
		t.Fatalf("pendingVimKey after dd = %q, want empty", model.pendingVimKey)
	}
}

func TestSingleCTimeoutStillCopies(t *testing.T) {
	store := newMockStore()
	store.keys["default"] = []string{"TOKEN"}
	store.values["default"] = map[string]string{"TOKEN": "secret"}
	clipboard := &mockClipboard{}
	m := NewModel(Deps{Store: store, Clipboard: clipboard})
	updated, _ := m.Update(loadedMsg{items: []entry{{Vault: "default", Key: "TOKEN"}}, activeVault: "default"})
	model := updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected copy command from first c")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if len(clipboard.values) != 1 || clipboard.values[0] != "secret" {
		t.Fatalf("clipboard values = %v, want [secret]", clipboard.values)
	}
}

func TestBookmarkTogglePersistsAndPinsEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.json")
	originalPathFn := bookmarksPath
	bookmarksPath = func() string { return path }
	defer func() { bookmarksPath = originalPathFn }()

	m := NewModel(Deps{Store: newMockStore()})
	updated, _ := m.Update(loadedMsg{items: []entry{{Vault: "default", Key: "TOKEN"}, {Vault: "default", Key: "ALPHA"}}, activeVault: "default"})
	model := updated.(Model)
	model.list.Select(0)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'*'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected flash clear command from bookmark toggle")
	}
	if !model.isBookmarked(entry{Vault: "default", Key: "ALPHA"}) && !model.isBookmarked(entry{Vault: "default", Key: "TOKEN"}) {
		t.Fatal("expected one entry to be bookmarked")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "default/") {
		t.Fatalf("bookmark file contents = %q", string(data))
	}
	if header, ok := model.list.Items()[0].(groupHeader); !ok || header.Vault != "⭐ Favorites" {
		t.Fatalf("first item = %#v, want favorites header", model.list.Items()[0])
	}
}

func TestCopyHistorySummaryTracksLastThreeCopies(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	m.recordCopy(entry{Vault: "default", Key: "ONE"})
	m.recordCopy(entry{Vault: "default", Key: "TWO"})
	m.recordCopy(entry{Vault: "default", Key: "THREE"})
	m.recordCopy(entry{Vault: "default", Key: "FOUR"})

	if got := len(m.copyHistory); got != 3 {
		t.Fatalf("copyHistory len = %d, want 3", got)
	}
	if got := m.copyHistorySummary(); !strings.Contains(got, "FOUR") || !strings.Contains(got, "THREE") || !strings.Contains(got, "TWO") {
		t.Fatalf("copy history summary = %q", got)
	}
	if strings.Contains(m.copyHistorySummary(), "ONE") {
		t.Fatalf("copy history summary should drop oldest entry, got %q", m.copyHistorySummary())
	}
}
