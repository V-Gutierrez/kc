package envutil

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ParseEnvReader(r io.Reader) map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			continue
		}
		val = stripInlineComment(val)
		result[key] = unquoteValue(val)
	}
	return result
}

func ShellQuote(s string) string {
	if !NeedsQuoting(s) {
		return s
	}
	var b strings.Builder
	b.WriteByte('\'')
	for _, c := range s {
		if c == '\'' {
			b.WriteString(`'\''`)
		} else {
			b.WriteRune(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func NeedsQuoting(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.' || c == '/' || c == ':') {
			return true
		}
	}
	return false
}

func DotenvQuote(s string) string {
	if !NeedsQuoting(s) {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func JoinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func UpsertEnvFile(path string, entries map[string]string) (updated, appended int, err error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return 0, 0, err
	}

	content := string(data)
	lines := splitEnvFileLines(content)
	seen := make(map[string]bool, len(entries))
	for i, line := range lines {
		key, ok := upsertableKey(line)
		if !ok {
			continue
		}
		value, exists := entries[key]
		if !exists {
			continue
		}
		lines[i] = key + "=" + DotenvQuote(value)
		seen[key] = true
		updated++
	}

	for _, key := range SortedKeys(entries) {
		if seen[key] {
			continue
		}
		lines = append(lines, key+"="+DotenvQuote(entries[key]))
		appended++
	}

	output := JoinLines(lines)
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return updated, appended, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(output); err != nil {
		_ = tmp.Close()
		return updated, appended, err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return updated, appended, err
	}
	if err := tmp.Close(); err != nil {
		return updated, appended, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return updated, appended, err
	}
	return updated, appended, nil
}

func splitEnvFileLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
}

func upsertableKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	if uncommented, ok := strings.CutPrefix(trimmed, "#"); ok {
		trimmed = strings.TrimSpace(uncommented)
	}
	key, _, found := strings.Cut(trimmed, "=")
	if !found {
		return "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	return key, true
}

func stripInlineComment(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] == '\'' || s[0] == '"' {
		return s
	}
	if before, _, found := strings.Cut(s, " #"); found {
		return strings.TrimSpace(before)
	}
	return s
}

func unquoteValue(s string) string {
	if len(s) >= 2 {
		if s[0] == '\'' && s[len(s)-1] == '\'' {
			return s[1 : len(s)-1]
		}
		if s[0] == '"' && s[len(s)-1] == '"' {
			inner := s[1 : len(s)-1]
			inner = strings.ReplaceAll(inner, `\"`, `"`)
			inner = strings.ReplaceAll(inner, `\\`, `\`)
			return inner
		}
	}
	return s
}
