package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// editFixture builds a model holding one secret, already loaded and selected.
func editFixture(t *testing.T) (Model, *mockStore) {
	t.Helper()
	store := newMockStore()
	store.keys["prod"] = []string{"TOKEN"}
	store.values["prod"] = map[string]string{"TOKEN": "s3cret"}
	store.metadata["prod"] = map[string]string{"TOKEN": protectionProtected}

	m := NewModel(Deps{
		Store:     store,
		Vaults:    &mockVaults{list: []string{"default", "prod"}, active: "prod"},
		Clipboard: &mockClipboard{},
	})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"default", "prod"},
		activeVault: "prod",
		items:       []entry{{Vault: "prod", Key: "TOKEN", Protection: protectionProtected}},
	})
	return updated.(Model), store
}

// enterEdit presses "e" on the selected entry, the path a user takes without
// revealing the value first — so the form's value field starts empty.
func enterEdit(t *testing.T, model Model) Model {
	t.Helper()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	next := updated.(Model)
	if next.mode != modeEdit {
		t.Fatalf("mode after e = %v, want modeEdit", next.mode)
	}
	return next
}

// run drains one command and feeds the resulting message back into the model.
func run(t *testing.T, model Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return model
	}
	msg := cmd()
	if err, ok := msg.(errMsg); ok {
		t.Fatalf("command returned error: %v", err.err)
	}
	updated, _ := model.Update(msg)
	return updated.(Model)
}

// An edit submitted without typing a value must never blank the secret.
// This is the destructive path: the form opens empty because the value was
// never revealed, so a plain Enter used to overwrite the live secret with "".
func TestEditSubmitWithUntouchedValueKeepsSecret(t *testing.T) {
	model, store := editFixture(t)
	model = enterEdit(t, model)

	model, cmd := model.submitForm()
	model = run(t, model, cmd)

	if got := store.values["prod"]["TOKEN"]; got != "s3cret" {
		t.Fatalf("stored value = %q, want %q (edit must not blank an untouched value)", got, "s3cret")
	}
	for _, call := range store.setCalls {
		if call.value == "" {
			t.Fatalf("edit wrote an empty value: %#v", call)
		}
	}
	if len(store.delCalls) != 0 {
		t.Fatalf("delete calls = %d, want 0 for an in-place edit", len(store.delCalls))
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse after a successful save", model.mode)
	}
}

// Renaming the key in the edit form is a move, not a copy: the new name carries
// the value over and the original stops existing.
func TestEditRenameMovesSecretAndRemovesOrigin(t *testing.T) {
	model, store := editFixture(t)
	model = enterEdit(t, model)
	model.form.key.SetValue("API_TOKEN")

	model, cmd := model.submitForm()
	model = run(t, model, cmd)

	if got := store.values["prod"]["API_TOKEN"]; got != "s3cret" {
		t.Fatalf("renamed value = %q, want %q", got, "s3cret")
	}
	if _, still := store.values["prod"]["TOKEN"]; still {
		t.Fatal("original key survived the rename — edit left a duplicate secret behind")
	}
	if len(store.delCalls) != 1 || store.delCalls[0].key != "TOKEN" {
		t.Fatalf("delete calls = %#v, want one delete of TOKEN", store.delCalls)
	}
	if len(model.entries) != 1 || model.entries[0].Key != "API_TOKEN" {
		t.Fatalf("entries = %#v, want only API_TOKEN", model.entries)
	}
}

// Same contract when the vault field changes: the secret moves across vaults.
func TestEditChangingVaultMovesSecret(t *testing.T) {
	model, store := editFixture(t)
	model = enterEdit(t, model)
	model.form.vault.SetValue("default")

	model, cmd := model.submitForm()
	model = run(t, model, cmd)

	if got := store.values["default"]["TOKEN"]; got != "s3cret" {
		t.Fatalf("moved value = %q, want %q", got, "s3cret")
	}
	if _, still := store.values["prod"]["TOKEN"]; still {
		t.Fatal("original vault kept the secret — move degraded into a copy")
	}
	if len(model.entries) != 1 || model.entries[0].Vault != "default" {
		t.Fatalf("entries = %#v, want only the default-vault row", model.entries)
	}
}

// A blank key name is a typo, not an instruction to write a nameless secret.
func TestEditRejectsEmptyKeyName(t *testing.T) {
	model, store := editFixture(t)
	model = enterEdit(t, model)
	model.form.key.SetValue("")

	model, cmd := model.submitForm()
	if cmd != nil {
		t.Fatal("submitting an empty key must not issue a store command")
	}
	if len(store.setCalls) != 0 {
		t.Fatalf("set calls = %#v, want none", store.setCalls)
	}
	if model.mode != modeEdit {
		t.Fatalf("mode = %v, want modeEdit (form stays open on a rejected submit)", model.mode)
	}
	if model.formError == "" {
		t.Fatal("expected the form to explain why the submit was rejected")
	}
}

// Toggling protection edits one row; it must not spawn a second row for the
// same vault+key just because the struct is no longer byte-identical.
func TestEditProtectionToggleDoesNotDuplicateRow(t *testing.T) {
	model, _ := editFixture(t)
	model = enterEdit(t, model)
	model.form.isProtected = false

	model, cmd := model.submitForm()
	model = run(t, model, cmd)

	if len(model.entries) != 1 {
		t.Fatalf("entries = %#v, want a single row", model.entries)
	}
	if model.entries[0].Protection != protectionUnprotected {
		t.Fatalf("protection = %q, want %q", model.entries[0].Protection, protectionUnprotected)
	}
}

// A typed value still wins: keep-current only applies to an untouched field.
func TestEditWithTypedValueOverwrites(t *testing.T) {
	model, store := editFixture(t)
	model = enterEdit(t, model)
	model.form.value.SetValue("rotated")

	model, cmd := model.submitForm()
	model = run(t, model, cmd)

	if got := store.values["prod"]["TOKEN"]; got != "rotated" {
		t.Fatalf("stored value = %q, want %q", got, "rotated")
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse after a successful save", model.mode)
	}
}

// Add mode is unchanged: a brand-new key has no previous value to preserve.
func TestAddModeStillWritesTypedValue(t *testing.T) {
	model, store := editFixture(t)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)
	model.form.vault.SetValue("prod")
	model.form.key.SetValue("NEW_KEY")
	model.form.value.SetValue("fresh")

	model, cmd := model.submitForm()
	model = run(t, model, cmd)

	if got := store.values["prod"]["NEW_KEY"]; got != "fresh" {
		t.Fatalf("stored value = %q, want %q", got, "fresh")
	}
	if len(store.delCalls) != 0 {
		t.Fatalf("delete calls = %#v, want none in add mode", store.delCalls)
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse after a successful save", model.mode)
	}
}

// The form has to say out loud that an empty value field is not destructive,
// otherwise the safe behaviour is invisible and users still fear the Enter.
func TestEditOverlayAnnouncesValueIsPreserved(t *testing.T) {
	model, store := editFixture(t)
	store.getErr = errors.New("authentication failed: app cancel")
	model = enterEdit(t, model)
	model.width = 120
	model.height = 40

	view := model.overlayView()
	if !strings.Contains(view, "keeps current value") {
		t.Fatalf("edit overlay does not explain the empty value field:\n%s", view)
	}
}

// Add mode has no current value to keep — the hint must not appear there.
func TestAddOverlayOmitsKeepCurrentHint(t *testing.T) {
	model, _ := editFixture(t)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)

	if view := model.overlayView(); strings.Contains(view, "keeps current value") {
		t.Fatalf("add overlay must not promise a current value:\n%s", view)
	}
}

// The confirm step must name the destructive part: a rename moves the secret.
func TestConfirmOverlayDescribesMove(t *testing.T) {
	model, _ := editFixture(t)
	model = enterEdit(t, model)
	model.form.key.SetValue("API_TOKEN")
	model.form.confirming = true

	view := model.overlayView()
	if !strings.Contains(view, "Move") || !strings.Contains(view, "TOKEN") || !strings.Contains(view, "API_TOKEN") {
		t.Fatalf("confirm overlay does not describe the move:\n%s", view)
	}
}

// A rejected submit has to show why, inside the form the user is still in.
func TestFormErrorIsRendered(t *testing.T) {
	model, _ := editFixture(t)
	model = enterEdit(t, model)
	model.form.key.SetValue("")

	model, _ = model.submitForm()
	view := model.overlayView()
	if !strings.Contains(view, "key name cannot be empty") {
		t.Fatalf("form error not rendered:\n%s", view)
	}
}

// The case Victor hit: the form pre-fill is a Keychain read, and a read can
// fail — a declined Touch ID leaves the value field empty. Submitting then must
// not take that emptiness as an instruction to blank the stored secret.
func TestEditKeepsSecretWhenPrefillFails(t *testing.T) {
	model, store := editFixture(t)
	store.getErr = errors.New("authentication failed: app cancel")
	model = enterEdit(t, model)

	if model.form.value.Value() != "" {
		t.Fatalf("value field = %q, want empty when the pre-fill failed", model.form.value.Value())
	}
	if model.form.valueSeeded {
		t.Fatal("a failed pre-fill must not count as a seeded value")
	}

	// The write path reads the current value back; let that read succeed.
	store.getErr = nil
	model, cmd := model.submitForm()
	model = run(t, model, cmd)

	if got := store.values["prod"]["TOKEN"]; got != "s3cret" {
		t.Fatalf("stored value = %q, want %q", got, "s3cret")
	}
	for _, call := range store.setCalls {
		if call.value == "" {
			t.Fatalf("a failed pre-fill still blanked the secret: %#v", call)
		}
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse after a successful save", model.mode)
	}
}

// Renaming still moves the secret when the pre-fill failed: the value is read
// at write time, so the new name carries the real secret, not an empty string.
func TestEditRenameAfterFailedPrefillCarriesTheValue(t *testing.T) {
	model, store := editFixture(t)
	store.getErr = errors.New("authentication failed: app cancel")
	model = enterEdit(t, model)
	store.getErr = nil
	model.form.key.SetValue("API_TOKEN")

	model, cmd := model.submitForm()
	model = run(t, model, cmd)

	if got := store.values["prod"]["API_TOKEN"]; got != "s3cret" {
		t.Fatalf("renamed value = %q, want %q", got, "s3cret")
	}
	if _, still := store.values["prod"]["TOKEN"]; still {
		t.Fatal("original key survived the rename")
	}
}
