package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Helper to initialize model for Add Modal tests
func setupAddModalTest() (Model, *mockStore) {
	store := newMockStore()
	store.keys["default"] = []string{"EXISTING_KEY"}
	store.values["default"] = map[string]string{"EXISTING_KEY": "val"}

	m := NewModel(Deps{
		Store:         store,
		Vaults:        &mockVaults{list: []string{"default"}, active: "default"},
		Clipboard:     &mockClipboard{},
		InitialFilter: "",
	})

	// Load initial data
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"default"},
		activeVault: "default",
		items:       []entry{{Vault: "default", Key: "EXISTING_KEY"}},
	})
	m = updated.(Model)

	// Enter Add mode
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

	output := m.overlayView()
	if !strings.Contains(output, "Vaults: default") {
		t.Fatalf("expected vault list hint, got %q", output)
	}
	if !strings.Contains(output, "existing vault") {
		t.Fatalf("expected existing vault hint, got %q", output)
	}
}

func TestAddModalNavigationAndToggles(t *testing.T) {
	m, _ := setupAddModalTest()

	// Force focus to Value (index 2)
	m.form.focus = 2
	m.focusForm()

	// Test Ctrl+R toggle on Value field
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated.(Model)
	if m.form.value.EchoMode == textinput.EchoPassword {
		t.Error("Expected value to be revealed after Ctrl+R")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated.(Model)
	if m.form.value.EchoMode != textinput.EchoPassword {
		t.Error("Expected value to be masked after second Ctrl+R")
	}

	// Tab to Protection field (index 3)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.form.focus != 3 {
		t.Errorf("Expected focus 3 (Protection), got %d", m.form.focus)
	}

	// Test Space toggle on Protection field
	initialProtection := m.form.isProtected
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if m.form.isProtected == initialProtection {
		t.Error("Expected protection to toggle on Space")
	}
}

func TestAddModalConfirmationAndSave(t *testing.T) {
	m, store := setupAddModalTest()

	// Fill form
	m.form.vault.SetValue("default")
	m.form.key.SetValue("NEW_KEY")
	m.form.value.SetValue("secret")
	m.form.isProtected = true

	// Press Enter to trigger Confirmation
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if !m.form.confirming {
		t.Error("Expected to be in confirmation state after Enter")
	}
	confirmView := m.overlayView()
	if !strings.Contains(confirmView, "Save NEW_KEY to vault:default (🔐 protected)?") {
		t.Fatalf("expected confirmation summary, got %q", confirmView)
	}

	// Press Enter again to Save
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	// Handle save command
	if cmd == nil {
		t.Fatal("Expected save command, got nil")
	}
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)

	// Verify Save
	if len(store.setCalls) != 1 {
		t.Fatalf("Expected 1 setCall, got %d", len(store.setCalls))
	}
	call := store.setCalls[0]
	if !call.protected {
		t.Error("Expected saved entry to be protected")
	}

	// Verify Success Flash
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
