package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type mockHistory struct {
	versions     map[string][]Version
	values       map[string]string
	versionsErr  error
	valueErr     error
	rollbackErr  error
	rollbackCall []rollbackCall
}

type rollbackCall struct {
	vault string
	key   string
	seq   int
}

func newMockHistory() *mockHistory {
	return &mockHistory{
		versions: make(map[string][]Version),
		values:   make(map[string]string),
	}
}

func (m *mockHistory) Versions(vault, key string) ([]Version, error) {
	if m.versionsErr != nil {
		return nil, m.versionsErr
	}
	return m.versions[vault+"/"+key], nil
}

func (m *mockHistory) Value(vault, key string, seq int) (string, error) {
	if m.valueErr != nil {
		return "", m.valueErr
	}
	return m.values[vault+"/"+key], nil
}

func (m *mockHistory) Rollback(vault, key string, seq int) error {
	m.rollbackCall = append(m.rollbackCall, rollbackCall{vault: vault, key: key, seq: seq})
	return m.rollbackErr
}

// historyFixture builds a model with one secret that has two recorded versions.
func historyFixture(t *testing.T) (Model, *mockStore, *mockHistory) {
	t.Helper()
	store := newMockStore()
	store.keys["prod"] = []string{"TOKEN"}
	store.values["prod"] = map[string]string{"TOKEN": "current"}
	store.metadata["prod"] = map[string]string{"TOKEN": protectionProtected}

	history := newMockHistory()
	history.versions["prod/TOKEN"] = []Version{
		{Seq: 2, Recorded: "2026-09-20 11:00", Protected: true, Digest: "bbbbbbbbbbbb"},
		{Seq: 1, Recorded: "2026-09-19 09:30", Protected: true, Digest: "aaaaaaaaaaaa"},
	}
	history.values["prod/TOKEN"] = "older-secret"

	m := NewModel(Deps{
		Store:     store,
		Vaults:    &mockVaults{list: []string{"default", "prod"}, active: "prod"},
		Clipboard: &mockClipboard{},
		History:   history,
	})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"default", "prod"},
		activeVault: "prod",
		items:       []entry{{Vault: "prod", Key: "TOKEN", Protection: protectionProtected}},
	})
	return updated.(Model), store, history
}

// openHistory presses "h" on the selected entry and drains the load command.
func openHistory(t *testing.T, model Model) Model {
	t.Helper()
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	next := updated.(Model)
	if next.mode != modeHistory {
		t.Fatalf("mode after h = %v, want modeHistory", next.mode)
	}
	if cmd == nil {
		t.Fatal("opening history must issue a load command")
	}
	updated, _ = next.Update(cmd())
	return updated.(Model)
}

// Pressing h on a secret opens its recorded versions.
func TestHistoryOpensForSelectedEntry(t *testing.T) {
	model, _, _ := historyFixture(t)
	model = openHistory(t, model)

	if len(model.history.versions) != 2 {
		t.Fatalf("versions = %#v, want 2", model.history.versions)
	}
	if model.history.vault != "prod" || model.history.key != "TOKEN" {
		t.Fatalf("history target = %s/%s, want prod/TOKEN", model.history.vault, model.history.key)
	}
}

// The view lists sequence, timestamp and digest — and never a plaintext value.
func TestHistoryViewShowsVersionsWithoutPlaintext(t *testing.T) {
	model, _, _ := historyFixture(t)
	model = openHistory(t, model)
	model.width = 120
	model.height = 40

	view := model.historyView()
	for _, want := range []string{"TOKEN", "2026-09-20 11:00", "bbbbbbbbbbbb", "aaaaaaaaaaaa"} {
		if !strings.Contains(view, want) {
			t.Fatalf("history view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "older-secret") || strings.Contains(view, "current") {
		t.Fatalf("history view leaked a secret value:\n%s", view)
	}
}

// A key that was never overwritten says so instead of showing an empty box.
func TestHistoryViewExplainsWhenThereAreNoVersions(t *testing.T) {
	model, _, history := historyFixture(t)
	history.versions["prod/TOKEN"] = nil
	model = openHistory(t, model)
	model.width = 120
	model.height = 40

	if view := model.historyView(); !strings.Contains(view, "No recorded versions") {
		t.Fatalf("history view does not explain the empty case:\n%s", view)
	}
}

// Rollback asks before it writes: restoring is a write to the live secret.
func TestHistoryRollbackAsksForConfirmation(t *testing.T) {
	model, _, history := historyFixture(t)
	model = openHistory(t, model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("rollback must not write before the confirmation")
	}
	if !model.history.confirming {
		t.Fatal("expected the history view to ask for confirmation")
	}
	if len(history.rollbackCall) != 0 {
		t.Fatalf("rollback ran without confirmation: %#v", history.rollbackCall)
	}
	if view := model.historyView(); !strings.Contains(view, "Restore version") {
		t.Fatalf("confirmation is not visible:\n%s", view)
	}
}

// Confirming restores the selected version and returns to the list.
func TestHistoryRollbackConfirmedRestoresSelectedVersion(t *testing.T) {
	model, _, history := historyFixture(t)
	model = openHistory(t, model)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("confirmed rollback must issue a command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	if len(history.rollbackCall) != 1 {
		t.Fatalf("rollback calls = %#v, want exactly one", history.rollbackCall)
	}
	got := history.rollbackCall[0]
	if got.vault != "prod" || got.key != "TOKEN" || got.seq != 2 {
		t.Fatalf("rollback call = %#v, want prod/TOKEN seq 2", got)
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse after a rollback", model.mode)
	}
	if !strings.Contains(model.flashMessage, "Restored") {
		t.Fatalf("flash = %q, want it to confirm the restore", model.flashMessage)
	}
}

// Cancelling the confirmation leaves the history open and writes nothing.
func TestHistoryRollbackCancelKeepsHistoryOpen(t *testing.T) {
	model, _, history := historyFixture(t)
	model = openHistory(t, model)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if model.history.confirming {
		t.Fatal("cancel should drop the confirmation")
	}
	if model.mode != modeHistory {
		t.Fatalf("mode = %v, want modeHistory", model.mode)
	}
	if len(history.rollbackCall) != 0 {
		t.Fatalf("cancel still rolled back: %#v", history.rollbackCall)
	}
}

// Enter copies a recorded value to the clipboard — same contract as the list.
func TestHistoryEnterCopiesTheRecordedValue(t *testing.T) {
	model, _, _ := historyFixture(t)
	clip := &mockClipboard{}
	model.deps.Clipboard = clip
	model = openHistory(t, model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("enter must issue a copy command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	if len(clip.values) != 1 || clip.values[0] != "older-secret" {
		t.Fatalf("clipboard = %#v, want the recorded value", clip.values)
	}
	if !strings.Contains(model.flashMessage, "Copied") {
		t.Fatalf("flash = %q, want a copy confirmation", model.flashMessage)
	}
}

// Esc closes the history and goes back to browsing.
func TestHistoryEscapeReturnsToBrowse(t *testing.T) {
	model, _, _ := historyFixture(t)
	model = openHistory(t, model)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse", model.mode)
	}
}

// A backend that cannot list versions surfaces the error instead of an empty box.
func TestHistoryLoadErrorIsShown(t *testing.T) {
	model, _, history := historyFixture(t)
	history.versionsErr = errors.New("keychain unavailable")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	if model.err == nil {
		t.Fatal("expected the load failure to surface")
	}
	model.width = 120
	model.height = 40
	if view := model.historyView(); !strings.Contains(view, "keychain unavailable") {
		t.Fatalf("the failure is recorded but invisible — the pane must show it:\n%s", view)
	}
}

// Without a history backend wired, the key is inert rather than a nil panic.
func TestHistoryKeyIsInertWithoutBackend(t *testing.T) {
	store := newMockStore()
	store.keys["prod"] = []string{"TOKEN"}
	m := NewModel(Deps{Store: store, Vaults: &mockVaults{list: []string{"prod"}, active: "prod"}})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"prod"},
		activeVault: "prod",
		items:       []entry{{Vault: "prod", Key: "TOKEN"}},
	})
	model := updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("no history backend means no command to run")
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse when history is unavailable", model.mode)
	}
}

// The shortcut has to be discoverable, or the feature may as well not exist.
func TestHelpOverlayDocumentsHistoryShortcut(t *testing.T) {
	model, _, _ := historyFixture(t)
	model.width = 120
	model.height = 40
	model.mode = modeHelp

	view := model.helpOverlayView()
	if !strings.Contains(view, "History") {
		t.Fatalf("help overlay does not mention history:\n%s", view)
	}
}

// A Keychain digest is a full SHA-256. Rendering all 64 characters wraps the
// pane and buries the timestamp — the CLI shows 12, and so must the TUI.
func TestHistoryViewShortensTheDigest(t *testing.T) {
	model, _, history := historyFixture(t)
	full := "fb04dcb6970e4c3d1873de51fd5a50d7bb46b3383113602665c350ec40b5f990"
	history.versions["prod/TOKEN"] = []Version{
		{Seq: 1, Recorded: "2026-09-21 20:27", Protected: false, Digest: full},
	}
	model = openHistory(t, model)
	model.width = 120
	model.height = 40

	view := model.historyView()
	if strings.Contains(view, full) {
		t.Fatalf("history view rendered the full digest:\n%s", view)
	}
	if !strings.Contains(view, full[:12]) {
		t.Fatalf("history view dropped the digest entirely:\n%s", view)
	}
}

// The preview pane is where a user actually looks before acting on a key, so
// the history shortcut has to be listed there too — not only in the help modal.
func TestPreviewActionsListHistoryShortcut(t *testing.T) {
	model, _, _ := historyFixture(t)
	model.width = 120
	model.height = 40

	if view := model.previewView(); !strings.Contains(view, "[h] History") {
		t.Fatalf("preview actions do not offer history:\n%s", view)
	}
}

// An error left over from an earlier action must not hijack a history pane that
// loaded fine — otherwise the list is there and the user is told it is not.
func TestHistorySuccessfulLoadClearsAStaleError(t *testing.T) {
	model, _, _ := historyFixture(t)
	model.err = errors.New("something failed earlier")
	model = openHistory(t, model)
	model.width = 120
	model.height = 40

	if model.err != nil {
		t.Fatalf("stale error survived a successful load: %v", model.err)
	}
	if view := model.historyView(); !strings.Contains(view, "VER") {
		t.Fatalf("history list not rendered:\n%s", view)
	}
}
