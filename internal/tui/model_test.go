package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type mockStore struct {
	keys     map[string][]string
	values   map[string]map[string]string
	getCalls []storeCall
	setCalls []setCall
	delCalls []storeCall
}

type storeCall struct {
	vault string
	key   string
}

type setCall struct {
	vault string
	key   string
	value string
}

func newMockStore() *mockStore {
	return &mockStore{
		keys:   make(map[string][]string),
		values: make(map[string]map[string]string),
	}
}

func (m *mockStore) Get(vault, key string) (string, error) {
	m.getCalls = append(m.getCalls, storeCall{vault: vault, key: key})
	return m.values[vault][key], nil
}

func (m *mockStore) Set(vault, key, value string) error {
	m.setCalls = append(m.setCalls, setCall{vault: vault, key: key, value: value})
	if m.values[vault] == nil {
		m.values[vault] = make(map[string]string)
	}
	m.values[vault][key] = value
	m.ensureKey(vault, key)
	return nil
}

func (m *mockStore) Delete(vault, key string) error {
	m.delCalls = append(m.delCalls, storeCall{vault: vault, key: key})
	if m.values[vault] != nil {
		delete(m.values[vault], key)
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
	keys := append([]string(nil), m.keys[vault]...)
	return keys, nil
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
	list   []string
	active string
}

func (m *mockVaults) List() ([]string, error)  { return append([]string(nil), m.list...), nil }
func (m *mockVaults) Active() (string, error)  { return m.active, nil }
func (m *mockVaults) Switch(name string) error { m.active = name; return nil }

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
	store.keys["prod"] = []string{"API_KEY"}
	store.values["prod"] = map[string]string{"API_KEY": "secret-prod"}

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
	if last := store.setCalls[len(store.setCalls)-1]; last.vault != "prod" || last.key != "TOKEN" || last.value != "after" {
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
