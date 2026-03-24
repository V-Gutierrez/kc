package output

import (
	"encoding/json"
	"io"
	"sort"
)

type ListItem struct {
	Key   string `json:"key"`
	Vault string `json:"vault"`
	Value string `json:"value,omitempty"`
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
