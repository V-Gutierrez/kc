package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
)

func TestHeaderViewChecks(t *testing.T) {
	m := NewModel(Deps{
		Store:  newMockStore(),
		Vaults: &mockVaults{list: []string{"default"}, active: "default"},
	})

	m.entries = []entry{
		{Vault: "default", Key: "k1"},
		{Vault: "default", Key: "k2"},
	}
	m.currentFilter = "default"
	m.applyFilters()

	output := m.headerView()
	if !strings.Contains(output, "default") {
		t.Errorf("Header missing vault name 'default', got: %q", output)
	}
	if !strings.Contains(output, "2 keys") {
		t.Errorf("Header missing item count '2 keys', got: %q", output)
	}
}

func TestLoadingViewShowsBannerAndSubtitle(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	m.width = 80
	m.height = 24
	output := m.View()
	if !strings.Contains(output, "██╗  ██╗") {
		t.Fatalf("loading view missing ASCII banner, got: %q", output)
	}
	if !strings.Contains(output, "Loading vaults and keys...") {
		t.Fatalf("loading view missing subtitle, got: %q", output)
	}
}

func TestAutoHideLogic(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})

	entry := entry{Vault: "v", Key: "k"}
	update1, cmd := m.Update(revealedMsg{entry: entry, value: "secret"})
	m = update1.(Model)

	if !m.preview.revealed {
		t.Fatal("Model should reveal value initially")
	}
	if cmd == nil {
		t.Fatal("revealedMsg must return a command (the auto-hide timer)")
	}

	update2, _ := m.Update(hideMsg{entry: entry, token: m.revealToken})
	m = update2.(Model)

	if m.preview.revealed {
		t.Fatal("hideMsg should clear the revealed state")
	}
	if m.preview.value != "" {
		t.Fatal("hideMsg should clear the preview value")
	}
}

func TestSearchMatchCount(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	m.entries = []entry{
		{Vault: "v", Key: "apple"},
		{Vault: "v", Key: "apricot"},
		{Vault: "v", Key: "banana"},
	}
	m.applyFilters()

	m.mode = modeSearch
	m.search.SetValue("ap")
	m.applyFilters()

	output := m.searchView()
	if !strings.Contains(output, "2 matches") {
		t.Errorf("Search view should show '2 matches', got: %q", output)
	}
}

func TestStaleHideMessageDoesNotHideNewReveal(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})

	first := entry{Vault: "default", Key: "FIRST"}
	second := entry{Vault: "default", Key: "SECOND"}

	updated, cmdFirst := m.Update(revealedMsg{entry: first, value: "one"})
	m = updated.(Model)
	if cmdFirst == nil {
		t.Fatal("first reveal must return hide command")
	}

	updated, cmdSecond := m.Update(revealedMsg{entry: second, value: "two"})
	m = updated.(Model)
	if cmdSecond == nil {
		t.Fatal("second reveal must return hide command")
	}

	updated, _ = m.Update(hideMsg{entry: first, token: 1})
	m = updated.(Model)

	if !m.preview.revealed || m.preview.key != "SECOND" || m.preview.value != "two" {
		t.Fatalf("stale hide should not clear latest reveal, got %#v", m.preview)
	}
}

func TestAlternatingRowsRenderDifferently(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	delegate := itemDelegate{styles: &m.styles, model: &m}
	listModel := list.New([]list.Item{entry{Vault: "default", Key: "KEY"}}, delegate, 0, 0)
	listModel.Select(0)

	var even bytes.Buffer
	var odd bytes.Buffer
	delegate.Render(&even, listModel, 2, entry{Vault: "default", Key: "KEY"})
	delegate.Render(&odd, listModel, 1, entry{Vault: "default", Key: "KEY"})

	if even.String() == odd.String() {
		t.Fatalf("expected alternating row render output to differ, even=%q odd=%q", even.String(), odd.String())
	}
}

func TestStatusViewDefault(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	if got := m.statusView(); !strings.Contains(got, "Ready") {
		t.Fatalf("statusView = %q, want Ready", got)
	}
}

func TestPreviewViewShowsSelectedEntry(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	m.entries = []entry{{Vault: "default", Key: "TOKEN", Protection: protectionProtected}}
	m.applyFilters()
	output := m.previewView()
	if !strings.Contains(output, "TOKEN") {
		t.Fatalf("previewView = %q, want selected key", output)
	}
	if !strings.Contains(output, "🔐 Protected") {
		t.Fatalf("previewView = %q, want protection metadata", output)
	}
}

func TestEmptyVaultWelcomeMatchesRequestedCopy(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	m.loading = false
	m.width = 80
	m.height = 24
	output := m.View()
	for _, want := range []string{
		"No secrets yet! Get started:",
		"kc set API_KEY",
		"Store a secret (Touch ID protected)",
		"kc import .env",
		"Import from .env file",
		"kc setup",
		"Migrate from your shell config",
		"Or press `a` to add a secret right here.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("welcome view missing %q in %q", want, output)
		}
	}
}

func TestChiefsBorderContainsStripeCharacters(t *testing.T) {
	border := chiefsBorder(8, newStyles())
	if strings.Count(border, "━") != 8 {
		t.Fatalf("chiefsBorder() = %q, want 8 stripe characters", border)
	}
}

func TestHelpViewContainsBindings(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	output := m.helpView()
	for _, want := range []string{"/ search", "c copy", "a add", "d delete"} {
		if !strings.Contains(output, want) {
			t.Fatalf("helpView = %q, want %q", output, want)
		}
	}
}

