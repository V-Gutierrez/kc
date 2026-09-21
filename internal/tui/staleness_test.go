package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func daysAgo(n int) string {
	return time.Now().AddDate(0, 0, -n).Format("2006-01-02 15:04")
}

// A credential nobody has rotated in over a year is the single most useful
// thing to see while browsing. It is derivable from the Modified timestamp the
// list already holds, so it costs no secret reads and no Touch ID prompt.
func TestStaleCredentialIsFlaggedFromMetadataAlone(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		modified string
		want     bool
	}{
		{"old credential", "STRIPE_API_KEY", daysAgo(400), true},
		{"fresh credential", "STRIPE_API_KEY", daysAgo(10), false},
		{"old but not a credential", "FEATURE_FLAG", daysAgo(400), false},
		{"token suffix counts", "GITHUB_TOKEN", daysAgo(200), true},
		{"secret suffix counts", "APP_SECRET", daysAgo(200), true},
		{"password suffix counts", "DB_PASSWORD", daysAgo(200), true},
		{"unparseable timestamp is not a claim", "API_KEY", "who knows", false},
		{"missing timestamp is not a claim", "API_KEY", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isStaleCredential(entry{Key: tc.key, Modified: tc.modified}, 180)
			if got != tc.want {
				t.Fatalf("isStaleCredential(%s, %q) = %v, want %v", tc.key, tc.modified, got, tc.want)
			}
		})
	}
}

// The badge has to reach the screen, not just the predicate.
func TestStaleBadgeIsRenderedInTheList(t *testing.T) {
	store := newMockStore()
	m := NewModel(Deps{Store: store, Vaults: &mockVaults{list: []string{"prod"}, active: "prod"}})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"prod"},
		activeVault: "prod",
		items: []entry{
			{Vault: "prod", Key: "OLD_API_KEY", Protection: protectionProtected, Modified: daysAgo(400)},
			{Vault: "prod", Key: "NEW_API_KEY", Protection: protectionProtected, Modified: daysAgo(3)},
		},
	})
	model := updated.(Model)
	model.width = 140
	model.height = 40
	model.list.SetSize(80, 20)

	view := model.View()
	if !strings.Contains(view, staleBadge) {
		t.Fatalf("no staleness badge rendered:\n%s", view)
	}
	if strings.Count(view, staleBadge) != 1 {
		t.Fatalf("badge count = %d, want 1 (only the old key)", strings.Count(view, staleBadge))
	}
}

// The preview pane should say how long it has been, not just show a symbol.
func TestPreviewExplainsStaleness(t *testing.T) {
	store := newMockStore()
	m := NewModel(Deps{Store: store, Vaults: &mockVaults{list: []string{"prod"}, active: "prod"}})
	updated, _ := m.Update(loadedMsg{
		vaults:      []string{"prod"},
		activeVault: "prod",
		items:       []entry{{Vault: "prod", Key: "OLD_API_KEY", Protection: protectionProtected, Modified: daysAgo(400)}},
	})
	model := updated.(Model)
	model.width = 140
	model.height = 40

	view := model.previewView()
	if !strings.Contains(view, "not rotated") {
		t.Fatalf("preview does not explain the staleness:\n%s", view)
	}
	// The timestamp is minute-resolution, so 400 days ago reads as 399 or 400
	// depending on the clock. Pin the magnitude, not an exact day.
	days := regexp.MustCompile(`not rotated in (\d+) days`).FindStringSubmatch(view)
	if days == nil {
		t.Fatalf("preview does not say how long:\n%s", view)
	}
	n, err := strconv.Atoi(days[1])
	if err != nil || n < 395 || n > 401 {
		t.Fatalf("reported age = %q, want about 400 days", days[1])
	}
}

// The header tells you which vault is in force and, when a .kc-vault marker
// decided it, that the directory is the reason.
func TestHeaderNamesTheDirectoryPin(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore(), PinnedVault: "acme"})
	updated, _ := m.Update(loadedMsg{vaults: []string{"acme"}, activeVault: "acme"})
	model := updated.(Model)

	if header := model.headerView(); !strings.Contains(header, "pinned by .kc-vault") {
		t.Fatalf("header does not mention the directory pin:\n%s", header)
	}
	// The header scrolls off the top of a normal terminal, so the always-visible
	// status bar has to carry the marker too.
	if crumb := model.breadcrumb(); !strings.Contains(crumb, ".kc-vault") {
		t.Fatalf("breadcrumb does not mention the directory pin: %q", crumb)
	}
}

func TestHeaderSaysNothingWithoutAPin(t *testing.T) {
	m := NewModel(Deps{Store: newMockStore()})
	updated, _ := m.Update(loadedMsg{vaults: []string{"default"}, activeVault: "default"})
	model := updated.(Model)

	if header := model.headerView(); strings.Contains(header, "pinned") {
		t.Fatalf("header invented a pin:\n%s", header)
	}
	if crumb := model.breadcrumb(); strings.Contains(crumb, ".kc-vault") {
		t.Fatalf("breadcrumb invented a pin: %q", crumb)
	}
	_ = fmt.Sprint()
}
