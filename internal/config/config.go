// Package config reads and writes kc's user settings file (~/.kc/config).
//
// The format is deliberately boring: one `key = value` pair per line, `#`
// comments, no nesting. Unknown keys are rejected on write so a typo fails
// loudly instead of silently doing nothing.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Known configuration keys.
const (
	HistoryEnabled       = "history.enabled"
	HistoryRetention     = "history.retention"
	ArchiveRetentionDays = "archive.retention_days"
	AuditRotationDays    = "audit.rotation_days"
)

// FileName is the config file name inside the kc data dir.
const FileName = "config"

type kind int

const (
	kindBool kind = iota
	kindPositiveInt
)

type spec struct {
	kind     kind
	fallback string
	desc     string
}

var specs = map[string]spec{
	HistoryEnabled:       {kind: kindBool, fallback: "true", desc: "capture the previous value on every write"},
	HistoryRetention:     {kind: kindPositiveInt, fallback: "5", desc: "how many previous versions to keep per key"},
	ArchiveRetentionDays: {kind: kindPositiveInt, fallback: "30", desc: "days a soft-deleted vault stays restorable"},
	AuditRotationDays:    {kind: kindPositiveInt, fallback: "180", desc: "age in days after which audit flags a secret for rotation"},
}

// Config holds the values read from disk plus the defaults for everything else.
type Config struct {
	dir      string
	values   map[string]string
	comments []string
}

// Defaults returns the built-in value for every known key.
func Defaults() map[string]string {
	out := make(map[string]string, len(specs))
	for key, s := range specs {
		out[key] = s.fallback
	}
	return out
}

// Keys returns every known configuration key, sorted.
func Keys() []string {
	keys := make([]string, 0, len(specs))
	for key := range specs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Describe returns the human-readable purpose of a key.
func Describe(key string) string {
	return specs[key].desc
}

// Known reports whether key is a recognized setting.
func Known(key string) bool {
	_, ok := specs[key]
	return ok
}

// Load reads dir/config. A missing file is not an error — defaults apply.
func Load(dir string) (*Config, error) {
	cfg := &Config{dir: dir, values: make(map[string]string)}

	data, err := os.ReadFile(cfg.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("config: read %q: %w", cfg.Path(), err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			cfg.comments = append(cfg.comments, trimmed)
			continue
		}
		key, value, found := strings.Cut(trimmed, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !Known(key) {
			// Forward compatibility: keep unknown keys in the file, ignore them here.
			continue
		}
		if err := validate(key, value); err != nil {
			return nil, err
		}
		cfg.values[key] = value
	}

	return cfg, nil
}

// Path is the absolute location of the config file.
func (c *Config) Path() string {
	return filepath.Join(c.dir, FileName)
}

// Get returns the configured value for key, falling back to the default.
func (c *Config) Get(key string) string {
	if value, ok := c.values[key]; ok {
		return value
	}
	return specs[key].fallback
}

// Int returns key as an integer, falling back to the default on any problem.
func (c *Config) Int(key string) int {
	n, err := strconv.Atoi(c.Get(key))
	if err != nil {
		n, _ = strconv.Atoi(specs[key].fallback)
	}
	return n
}

// Bool returns key as a boolean, falling back to the default on any problem.
func (c *Config) Bool(key string) bool {
	b, err := strconv.ParseBool(c.Get(key))
	if err != nil {
		b, _ = strconv.ParseBool(specs[key].fallback)
	}
	return b
}

// All returns every known key with its effective value.
func (c *Config) All() map[string]string {
	out := Defaults()
	for key, value := range c.values {
		out[key] = value
	}
	return out
}

// Set validates and persists a single setting.
func (c *Config) Set(key, value string) error {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if !Known(key) {
		return fmt.Errorf("config: unknown key %q (known: %s)", key, strings.Join(Keys(), ", "))
	}
	if err := validate(key, value); err != nil {
		return err
	}
	c.values[key] = value
	return c.save()
}

// Unset removes an override so the default applies again.
func (c *Config) Unset(key string) error {
	if !Known(key) {
		return fmt.Errorf("config: unknown key %q (known: %s)", key, strings.Join(Keys(), ", "))
	}
	delete(c.values, key)
	return c.save()
}

func (c *Config) save() error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return fmt.Errorf("config: create %q: %w", c.dir, err)
	}

	var buf strings.Builder
	buf.WriteString("# kc configuration — `kc config set <key> <value>`\n")
	for _, key := range Keys() {
		value, ok := c.values[key]
		if !ok {
			continue
		}
		fmt.Fprintf(&buf, "%s = %s\n", key, value)
	}

	if err := os.WriteFile(c.Path(), []byte(buf.String()), 0o600); err != nil {
		return fmt.Errorf("config: write %q: %w", c.Path(), err)
	}
	return nil
}

func validate(key, value string) error {
	switch specs[key].kind {
	case kindBool:
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("config: %s must be true or false, got %q", key, value)
		}
	case kindPositiveInt:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("config: %s must be a number, got %q", key, value)
		}
		if n < 0 {
			return fmt.Errorf("config: %s must be zero or greater, got %d", key, n)
		}
	}
	return nil
}
