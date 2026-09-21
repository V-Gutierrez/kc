package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func vimFixture(t *testing.T) (Model, *mockStore, *mockClipboard) {
	t.Helper()
	store := newMockStore()
	store.keys["prod"] = []string{"TOKEN"}
	store.values["prod"] = map[string]string{"TOKEN": "s3cret"}
	clip := &mockClipboard{}

	m := NewModel(Deps{
		Store:     store,
		Vaults:    &mockVaults{list: []string{"prod"}, active: "prod"},
		Clipboard: clip,
	})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"prod"},
		activeVault: "prod",
		items:       []entry{{Vault: "prod", Key: "TOKEN", Protection: protectionProtected}},
	})
	return updated.(Model), store, clip
}

// `cc` is the documented way to edit. Getting there must not push the secret
// through the clipboard on the way — on a protected key the first `c` also
// raised a Touch ID prompt the user never asked for.
func TestSingleCDoesNotCopyBeforeTheSecondKey(t *testing.T) {
	model, store, clip := vimFixture(t)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)

	if cmd == nil {
		t.Fatal("c must arm the pending-vim timer")
	}
	// Running the command is the only way to tell a timer from a copy: a copy
	// only touches the store when its command executes.
	if _, isTimeout := cmd().(vimTimeoutMsg); !isTimeout {
		t.Fatal("c returned something other than the pending-vim timer")
	}
	if len(store.getCalls) != 0 {
		t.Fatalf("pressing c read the secret before the second key: %#v", store.getCalls)
	}
	if len(clip.values) != 0 {
		t.Fatalf("pressing c copied to the clipboard: %#v", clip.values)
	}
	if model.pendingVimKey != "c" {
		t.Fatalf("pendingVimKey = %q, want c", model.pendingVimKey)
	}
}

// A second c within the window is the edit, and still no clipboard write.
func TestDoubleCEditsWithoutCopying(t *testing.T) {
	model, _, clip := vimFixture(t)

	updated, first := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if first != nil {
		if _, isTimeout := first().(vimTimeoutMsg); !isTimeout {
			t.Fatal("the first c did something other than arm the timer")
		}
	}
	updated, second := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if second != nil {
		if _, isCopy := second().(copiedMsg); isCopy {
			t.Fatal("cc copied the secret")
		}
	}

	if model.mode != modeEdit {
		t.Fatalf("mode = %v, want modeEdit", model.mode)
	}
	if len(clip.values) != 0 {
		t.Fatalf("cc copied to the clipboard: %#v", clip.values)
	}
}

// c on its own still copies once the window closes — the status bar says so.
func TestSingleCCopiesAfterTheWindowCloses(t *testing.T) {
	model, _, clip := vimFixture(t)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)

	updated, cmd := model.Update(vimTimeoutMsg{token: model.pendingVimToken, key: "c"})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("the timeout must run the single-key action")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	if len(clip.values) != 1 || clip.values[0] != "s3cret" {
		t.Fatalf("clipboard = %#v, want the secret", clip.values)
	}
}

// Same contract for d: the confirmation belongs to the resolved keystroke, not
// to the first half of a pending pair.
func TestSingleDDoesNotOpenDeleteBeforeTheWindowCloses(t *testing.T) {
	model, _, _ := vimFixture(t)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)

	if model.mode == modeConfirmDelete {
		t.Fatal("a single d opened the delete confirmation immediately")
	}
	if cmd == nil {
		t.Fatal("d must arm the pending-vim timer")
	}

	updated, _ = model.Update(vimTimeoutMsg{token: model.pendingVimToken, key: "d"})
	model = updated.(Model)
	if model.mode != modeConfirmDelete {
		t.Fatalf("mode = %v, want modeConfirmDelete once the window closed", model.mode)
	}
}

// A failed save leaves the form open. Without the reason on screen the user
// just sees Enter do nothing, and presses it again.
func TestFormShowsWhyASaveFailed(t *testing.T) {
	model, _, _ := vimFixture(t)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)
	model.width = 120
	model.height = 40

	updated, _ = model.Update(errMsg{err: errors.New("keychain write denied")})
	model = updated.(Model)

	if view := model.overlayView(); !strings.Contains(view, "keychain write denied") {
		t.Fatalf("the form hides the failure:\n%s", view)
	}
}

// Same for the vault creation pane and the command palette.
func TestCreateVaultPaneShowsErrors(t *testing.T) {
	model, _, _ := vimFixture(t)
	model.mode = modeCreateVault
	model.width = 120
	model.height = 40
	model.err = errors.New("vault already exists")

	if view := model.createVaultView(); !strings.Contains(view, "vault already exists") {
		t.Fatalf("the vault pane hides the failure:\n%s", view)
	}
}

func TestCommandPaletteShowsErrors(t *testing.T) {
	model, _, _ := vimFixture(t)
	model.mode = modeCommandPalette
	model.width = 120
	model.height = 40
	model.err = errors.New("unknown command")

	if view := model.commandPaletteView(); !strings.Contains(view, "unknown command") {
		t.Fatalf("the palette hides the failure:\n%s", view)
	}
}

// Starting a new action clears the previous failure, or a stale error follows
// the user around every pane.
func TestOpeningTheFormClearsAPreviousError(t *testing.T) {
	model, _, _ := vimFixture(t)
	model.err = errors.New("something failed earlier")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)

	if model.err != nil {
		t.Fatalf("stale error survived into a fresh form: %v", model.err)
	}
}

// The pending-vim window is a real timer; the token guards against a stale one.
func TestStaleVimTimeoutIsIgnored(t *testing.T) {
	model, _, clip := vimFixture(t)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)

	updated, cmd := model.Update(vimTimeoutMsg{token: model.pendingVimToken - 1, key: "c"})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("a stale timeout must not act")
	}
	if len(clip.values) != 0 {
		t.Fatalf("stale timeout copied: %#v", clip.values)
	}
	_ = time.Second
}
