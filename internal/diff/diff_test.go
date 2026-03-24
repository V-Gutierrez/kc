package diff

import "testing"

func TestCompareCategorizesEntries(t *testing.T) {
	entries := Compare(
		map[string]string{"A": "1", "B": "2", "C": "same"},
		map[string]string{"B": "3", "C": "same", "D": "4"},
	)

	if len(entries) != 4 {
		t.Fatalf("len(entries) = %d, want 4", len(entries))
	}

	wants := []struct {
		key    string
		status Status
	}{
		{"A", Added},
		{"B", Changed},
		{"C", Equal},
		{"D", Removed},
	}

	for i, want := range wants {
		if entries[i].Key != want.key || entries[i].Status != want.status {
			t.Fatalf("entries[%d] = %#v, want key=%q status=%q", i, entries[i], want.key, want.status)
		}
	}
}

func TestCompareHandlesEmptyInputs(t *testing.T) {
	entries := Compare(map[string]string{}, map[string]string{})
	if len(entries) != 0 {
		t.Fatalf("len(entries) = %d, want 0", len(entries))
	}
}
