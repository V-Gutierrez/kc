package audit

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/v-gutierrez/kc/internal/keychain"
)

type Severity string

const (
	SeverityHigh   Severity = "HIGH"
	SeverityMedium Severity = "MEDIUM"
	SeverityLow    Severity = "LOW"
)

type Finding struct {
	Severity Severity
	Vault    string
	Key      string
	Rule     string
	Detail   string
}

type ScanInput struct {
	Vault         string
	Entries       map[string]string
	ReferenceKeys map[string]struct{}
	MinLength     int
	// Modified maps a key to the timestamp of its last write
	// ("2006-01-02 15:04", as Keychain reports it).
	Modified map[string]string
	// RotationDays flags sensitive secrets older than this many days.
	// Zero disables the rule.
	RotationDays int
}

func Scan(inputs []ScanInput) []Finding {
	findings := duplicateFindings(inputs)
	for _, input := range inputs {
		findings = append(findings, weakSecretFindings(input)...)
		findings = append(findings, suspiciousNameFindings(input)...)
		findings = append(findings, staleKeyFindings(input)...)
		findings = append(findings, rotationFindings(input)...)
	}

	sort.Slice(findings, func(i, j int) bool {
		if severityRank(findings[i].Severity) != severityRank(findings[j].Severity) {
			return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
		}
		if findings[i].Vault != findings[j].Vault {
			return findings[i].Vault < findings[j].Vault
		}
		if findings[i].Key != findings[j].Key {
			return findings[i].Key < findings[j].Key
		}
		if findings[i].Rule != findings[j].Rule {
			return findings[i].Rule < findings[j].Rule
		}
		return findings[i].Detail < findings[j].Detail
	})

	return findings
}

func duplicateFindings(inputs []ScanInput) []Finding {
	type occurrence struct {
		vault string
		key   string
	}

	byDigest := make(map[string][]occurrence)
	for _, input := range inputs {
		for key, value := range input.Entries {
			if strings.TrimSpace(value) == "" {
				continue
			}
			byDigest[keychain.Digest(value)] = append(byDigest[keychain.Digest(value)], occurrence{vault: input.Vault, key: key})
		}
	}

	var findings []Finding
	for _, occurrences := range byDigest {
		if len(occurrences) < 2 {
			continue
		}

		names := make([]string, 0, len(occurrences))
		for _, occurrence := range occurrences {
			names = append(names, occurrence.vault+":"+occurrence.key)
		}
		sort.Strings(names)
		detail := "duplicate value shared with " + strings.Join(names, ", ")

		for _, occurrence := range occurrences {
			findings = append(findings, Finding{
				Severity: SeverityHigh,
				Vault:    occurrence.vault,
				Key:      occurrence.key,
				Rule:     "duplicate",
				Detail:   detail,
			})
		}
	}

	return findings
}

func weakSecretFindings(input ScanInput) []Finding {
	minLength := input.MinLength
	if minLength == 0 {
		minLength = 16
	}

	keys := sortedKeys(input.Entries)
	findings := make([]Finding, 0)
	for _, key := range keys {
		value := input.Entries[key]
		reasons := weakReasons(value, minLength)
		if len(reasons) == 0 {
			continue
		}
		findings = append(findings, Finding{
			Severity: SeverityMedium,
			Vault:    input.Vault,
			Key:      key,
			Rule:     "weak-secret",
			Detail:   strings.Join(reasons, "; "),
		})
	}
	return findings
}

func suspiciousNameFindings(input ScanInput) []Finding {
	keys := sortedKeys(input.Entries)
	findings := make([]Finding, 0)
	for _, key := range keys {
		if !isSuspiciousName(key) {
			continue
		}
		findings = append(findings, Finding{
			Severity: SeverityLow,
			Vault:    input.Vault,
			Key:      key,
			Rule:     "suspicious-name",
			Detail:   "key name looks temporary or deprecated",
		})
	}
	return findings
}

func staleKeyFindings(input ScanInput) []Finding {
	if len(input.ReferenceKeys) == 0 {
		return nil
	}

	keys := sortedKeys(input.Entries)
	findings := make([]Finding, 0)
	for _, key := range keys {
		if _, ok := input.ReferenceKeys[key]; ok {
			continue
		}
		findings = append(findings, Finding{
			Severity: SeverityLow,
			Vault:    input.Vault,
			Key:      key,
			Rule:     "stale",
			Detail:   "key not present in reference environment",
		})
	}
	return findings
}

// rotationFindings flags secrets whose name marks them as credentials and
// whose last write is older than the configured rotation window. A secret that
// never changes is a secret whose blast radius only grows.
func rotationFindings(input ScanInput) []Finding {
	if input.RotationDays <= 0 || len(input.Modified) == 0 {
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -input.RotationDays)
	keys := sortedKeys(input.Entries)
	findings := make([]Finding, 0)
	for _, key := range keys {
		if !isRotatableName(key) {
			continue
		}
		modified, err := time.Parse("2006-01-02 15:04", strings.TrimSpace(input.Modified[key]))
		if err != nil || !modified.Before(cutoff) {
			continue
		}
		days := int(time.Since(modified).Hours() / 24)
		findings = append(findings, Finding{
			Severity: SeverityMedium,
			Vault:    input.Vault,
			Key:      key,
			Rule:     "stale-rotation",
			Detail:   fmt.Sprintf("unchanged for %d days (rotation window is %d)", days, input.RotationDays),
		})
	}
	return findings
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityHigh:
		return 0
	case SeverityMedium:
		return 1
	default:
		return 2
	}
}

func sortedKeys(entries map[string]string) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
