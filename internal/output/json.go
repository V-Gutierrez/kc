package output

import (
	"encoding/json"
	"io"
	"sort"
)

type ListItem struct {
	Key        string `json:"key"`
	Vault      string `json:"vault"`
	Value      string `json:"value,omitempty"`
	Protection string `json:"protection,omitempty"`
}

type GetItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Vault string `json:"vault"`
}

func ListItems(keys []string, vault string) []ListItem {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)

	items := make([]ListItem, 0, len(sorted))
	for _, key := range sorted {
		items = append(items, ListItem{Key: key, Vault: vault})
	}
	return items
}

func ListItemsWithValues(entries map[string]string, vault string) []ListItem {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]ListItem, 0, len(keys))
	for _, key := range keys {
		items = append(items, ListItem{Key: key, Vault: vault, Value: entries[key]})
	}
	return items
}

func GetResult(key, value, vault string) GetItem {
	return GetItem{Key: key, Value: value, Vault: vault}
}

func WriteJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(v)
}

// HistoryItem is one recorded version of a secret. The value is deliberately
// absent: a history listing must never print plaintext.
type HistoryItem struct {
	Version   int    `json:"version"`
	Key       string `json:"key"`
	Vault     string `json:"vault"`
	Recorded  string `json:"recorded,omitempty"`
	Protected bool   `json:"protected"`
	Digest    string `json:"digest,omitempty"`
}

// HistoryItems passes versions through unchanged, newest first.
func HistoryItems(items []HistoryItem) []HistoryItem {
	if items == nil {
		return []HistoryItem{}
	}
	return items
}

// VaultInfoResult is the JSON shape of `kc vault info`.
type VaultInfoResult struct {
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	Created           string   `json:"created,omitempty"`
	RequireProtection bool     `json:"requireProtection"`
	Keys              int      `json:"keys"`
	Service           string   `json:"service"`
}

// GetVersionItem is `kc get --version` output: a historical value, plus which
// version it came from.
type GetVersionItem struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Vault   string `json:"vault"`
	Version int    `json:"version"`
}

func GetVersionResult(key, value, vault string, version int) GetVersionItem {
	return GetVersionItem{Key: key, Value: value, Vault: vault, Version: version}
}
