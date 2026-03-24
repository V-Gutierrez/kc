package audit

import (
	"reflect"
	"testing"
)

func TestScanFindsDuplicatesWeakAndStale(t *testing.T) {
	findings := Scan([]ScanInput{
		{
			Vault:         "default",
			Entries:       map[string]string{"API_KEY": "shared-secret-value!", "TEMP": "password", "ONLY_IN_VAULT": "long-secret-value!"},
			ReferenceKeys: map[string]struct{}{"API_KEY": {}},
			MinLength:     16,
		},
		{
			Vault:         "prod",
			Entries:       map[string]string{"DUPLICATE": "shared-secret-value!", "old_token": "abc123"},
			ReferenceKeys: map[string]struct{}{"API_KEY": {}},
			MinLength:     16,
		},
	})

	if len(findings) == 0 {
		t.Fatal("expected findings")
	}

	var hasDuplicate, hasWeak, hasStale, hasSuspicious bool
	for _, finding := range findings {
		switch finding.Rule {
		case "duplicate":
			hasDuplicate = true
		case "weak-secret":
			hasWeak = true
		case "stale":
			hasStale = true
		case "suspicious-name":
			hasSuspicious = true
		}
	}

	if !hasDuplicate || !hasWeak || !hasStale || !hasSuspicious {
		t.Fatalf("findings missing expected rules: %#v", findings)
	}
}

func TestScanCleanVault(t *testing.T) {
	findings := Scan([]ScanInput{{
		Vault:         "default",
		Entries:       map[string]string{"API_KEY": "long-secret-value!@#"},
		ReferenceKeys: map[string]struct{}{"API_KEY": {}},
		MinLength:     16,
	}})

	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestScanSkipsStaleWhenReferenceKeysMissing(t *testing.T) {
	findings := Scan([]ScanInput{{
		Vault:     "default",
		Entries:   map[string]string{"API_KEY": "long-secret-value!@#"},
		MinLength: 16,
	}})

	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestScanSkipsEmptyValuesInDuplicateDetection(t *testing.T) {
	findings := Scan([]ScanInput{
		{Vault: "default", Entries: map[string]string{"EMPTY": "   "}},
		{Vault: "prod", Entries: map[string]string{"EMPTY": ""}},
	})

	for _, finding := range findings {
		if finding.Rule == "duplicate" {
			t.Fatalf("findings = %#v, want no duplicate findings", findings)
		}
	}
}

func TestWeakReasonsEmptyValue(t *testing.T) {
	got := weakReasons("   ", 16)
	want := []string{"empty value"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("weakReasons() = %#v, want %#v", got, want)
	}
}

func TestWeakReasonsSkipsSpecialCharacterWarningForLongAPIKeys(t *testing.T) {
	got := weakReasons("abcdefghijklmnopqrstuvwxyz1234567890", 16)
	for _, reason := range got {
		if reason == "missing special characters" {
			t.Fatalf("unexpected missing special characters reason for long API key style value: %#v", got)
		}
	}
}
