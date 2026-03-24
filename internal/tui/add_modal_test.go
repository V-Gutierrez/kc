package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func setupAddModalTest() (Model, *mockStore) {
	return setupAddModalTestWithVaults([]string{"default"}, "default")
}

func setupAddModalTestWithVaults(vaults []string, active string) (Model, *mockStore) {
	store := newMockStore()
	store.keys["default"] = []string{"EXISTING_KEY"}
	store.values["default"] = map[string]string{"EXISTING_KEY": "val"}

	m := NewModel(Deps{
		Store:         store,
		Vaults:        &mockVaults{list: vaults, active: active},
		Clipboard:     &mockClipboard{},
		InitialFilter: "",
	})

	updated, _ := m.Update(loadedMsg{
		vaults:      vaults,
		activeVault: active,
		items:       []entry{{Vault: "default", Key: "EXISTING_KEY"}},
	})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	return m, store
}

func TestAddModalInitialState(t *testing.T) {
	m, _ := setupAddModalTest()

	if m.mode != modeAdd {
		t.Fatalf("Expected modeAdd, got %v", m.mode)
	}
	if !m.form.isProtected {
		t.Error("Expected protection to be enabled by default")
	}
	if m.form.value.EchoMode != textinput.EchoPassword {
		t.Error("Expected value field to be masked by default")
	}
	if m.form.vault.Value() != "" {
		t.Fatalf("expected empty vault value so active vault can be shown as placeholder, got %q", m.form.vault.Value())
	}
	if m.form.vault.Placeholder != "default" {
		t.Fatalf("expected active vault placeholder, got %q", m.form.vault.Placeholder)
	}

	output := m.overlayView()
	if !strings.Contains(output, "existing vault") {
		t.Fatalf("expected existing vault hint, got %q", output)
	}
	if !strings.Contains(output, "F2 to reveal") {
		t.Fatalf("expected F2 reveal hint, got %q", output)
	}
	if !strings.Contains(output, "Tab: next field | Esc: cancel | Enter: confirm") {
		t.Fatalf("expected bottom navigation hint, got %q", output)
	}
	if !strings.Contains(output, "→ Key") {
		t.Fatalf("expected focused key label, got %q", output)
	}
	if strings.Contains(output, "1. default") {
		t.Fatalf("did not expect numbered vault list when vault field is not focused, got %q", output)
	}
}

func TestAddModalNavigationAndToggles(t *testing.T) {
	m, _ := setupAddModalTest()

	m.form.focus = 2
	m.focusForm()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(Model)
	if m.form.value.EchoMode == textinput.EchoPassword {
		t.Error("Expected value to be revealed after F2")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(Model)
	if m.form.value.EchoMode != textinput.EchoPassword {
		t.Error("Expected value to be masked after second F2")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.form.focus != 3 {
		t.Errorf("Expected focus 3 (Protection), got %d", m.form.focus)
	}

	initialProtection := m.form.isProtected
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if m.form.isProtected == initialProtection {
		t.Error("Expected protection to toggle on Space")
	}
}

func TestAddModalVaultQuickSelectAndFocusedList(t *testing.T) {
	m, _ := setupAddModalTestWithVaults([]string{"default", "prod", "staging"}, "default")

	m.form.focus = 0
	m.focusForm()

	output := m.overlayView()
	for _, want := range []string{"→ Vault", "1. default (default)", "2. prod", "3. staging"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected vault-focused overlay to contain %q, got %q", want, output)
		}
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)
	if got := m.form.vault.Value(); got != "prod" {
		t.Fatalf("vault after quick-select = %q, want prod", got)
	}

	m.form.vault.SetValue("")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)
	if got := m.form.vault.Value(); got != "staging" {
		t.Fatalf("vault after quick-select = %q, want staging", got)
	}
}

func TestAddModalVaultQuickSelectDoesNotOverrideDigitInputAfterTyping(t *testing.T) {
	m, _ := setupAddModalTestWithVaults([]string{"default", "prod", "staging"}, "default")

	m.form.focus = 0
	m.focusForm()
	m.form.vault.SetValue("custom")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if got := m.form.vault.Value(); got != "custom2" {
		t.Fatalf("vault value after typing digit = %q, want custom2", got)
	}
}

func TestAddModalVaultListLimitsQuickSelectHintsToNine(t *testing.T) {
	vaults := []string{"default", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}
	m, _ := setupAddModalTestWithVaults(vaults, "default")

	m.form.focus = 0
	m.focusForm()

	output := m.overlayView()
	if strings.Contains(output, "10.") {
		t.Fatalf("expected vault quick-select list to stop at 9, got %q", output)
	}
	if !strings.Contains(output, "9. eight") {
		t.Fatalf("expected ninth vault shortcut to be visible, got %q", output)
	}
}

func TestAddModalFocusIndicatorsFollowActiveField(t *testing.T) {
	m, _ := setupAddModalTestWithVaults([]string{"default", "prod"}, "default")

	output := m.overlayView()
	if !strings.Contains(output, "→ Key") {
		t.Fatalf("expected initial focus indicator on key field, got %q", output)
	}

	m.form.focus = 2
	m.focusForm()
	output = m.overlayView()
	if !strings.Contains(output, "→ Value") {
		t.Fatalf("expected value field focus indicator, got %q", output)
	}
	if strings.Contains(output, "→ Key") {
		t.Fatalf("did not expect key field to remain highlighted, got %q", output)
	}
}

func TestAddModalConfirmationAndSave(t *testing.T) {
	m, store := setupAddModalTest()

	m.form.vault.SetValue("default")
	m.form.key.SetValue("NEW_KEY")
	m.form.value.SetValue("secret")
	m.form.isProtected = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if !m.form.confirming {
		t.Error("Expected to be in confirmation state after Enter")
	}
	confirmView := m.overlayView()
	if !strings.Contains(confirmView, "Save NEW_KEY to vault:default (🔐 protected)?") {
		t.Fatalf("expected confirmation summary, got %q", confirmView)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if cmd == nil {
		t.Fatal("Expected save command, got nil")
	}
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)

	if len(store.setCalls) != 1 {
		t.Fatalf("Expected 1 setCall, got %d", len(store.setCalls))
	}
	call := store.setCalls[0]
	if !call.protected {
		t.Error("Expected saved entry to be protected")
	}

	if !strings.Contains(m.flashMessage, "Saved") {
		t.Errorf("Expected flash message to contain 'Saved', got %q", m.flashMessage)
	}
}

func TestAddModalShowsOverwriteWarningAndNamingHint(t *testing.T) {
	m, _ := setupAddModalTest()

	m.form.key.SetValue("EXISTING_KEY")
	output := m.overlayView()
	if !strings.Contains(output, "⚠ Key exists, will overwrite") {
		t.Fatalf("expected overwrite warning, got %q", output)
	}

	m.form.key.SetValue("mixedCase")
	output = m.overlayView()
	if !strings.Contains(output, "Use UPPER_SNAKE_CASE") {
		t.Fatalf("expected naming hint, got %q", output)
	}
}
