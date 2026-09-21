package audit

import (
	"reflect"
	"testing"
	"time"
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

func TestScan_FlagsSensitiveSecretsPastRotationWindow(t *testing.T) {
	stale := time.Now().AddDate(0, 0, -200).Format("2006-01-02 15:04")
	fresh := time.Now().AddDate(0, 0, -10).Format("2006-01-02 15:04")

	findings := Scan([]ScanInput{{
		Vault: "prod",
		Entries: map[string]string{
			"STRIPE_API_KEY": "placeholder-value-long-enough-to-not-be-weak",
			"RECENT_TOKEN":   "tok_abcdefghijklmnopqrstuvwxyz123456",
			"FEATURE_FLAG":   "enabled-for-everyone-and-not-a-secret",
		},
		Modified: map[string]string{
			"STRIPE_API_KEY": stale,
			"RECENT_TOKEN":   fresh,
			"FEATURE_FLAG":   stale,
		},
		RotationDays: 180,
	}})

	var flagged []string
	for _, finding := range findings {
		if finding.Rule == "stale-rotation" {
			flagged = append(flagged, finding.Key)
		}
	}
	if len(flagged) != 1 || flagged[0] != "STRIPE_API_KEY" {
		t.Fatalf("stale-rotation flagged %v, want [STRIPE_API_KEY]", flagged)
	}
}

func TestScan_RotationRuleOffByDefault(t *testing.T) {
	stale := time.Now().AddDate(0, 0, -900).Format("2006-01-02 15:04")
	findings := Scan([]ScanInput{{
		Vault:    "prod",
		Entries:  map[string]string{"STRIPE_API_KEY": "placeholder-value-long-enough-to-not-be-weak"},
		Modified: map[string]string{"STRIPE_API_KEY": stale},
	}})

	for _, finding := range findings {
		if finding.Rule == "stale-rotation" {
			t.Fatal("rotation rule fired without RotationDays set")
		}
	}
}

func TestScan_RotationIgnoresUnparseableTimestamps(t *testing.T) {
	findings := Scan([]ScanInput{{
		Vault:        "prod",
		Entries:      map[string]string{"STRIPE_API_KEY": "placeholder-value-long-enough-to-not-be-weak"},
		Modified:     map[string]string{"STRIPE_API_KEY": "who knows"},
		RotationDays: 30,
	}})

	for _, finding := range findings {
		if finding.Rule == "stale-rotation" {
			t.Fatal("rotation rule fired on an unparseable timestamp")
		}
	}
}
