package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteJSONListOmitsValuesByDefault(t *testing.T) {
	items := ListItems([]string{"B", "A"}, "default")

	var buf bytes.Buffer
	if err := WriteJSON(&buf, items); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded length = %d, want 2", len(decoded))
	}
	if decoded[0]["key"] != "A" || decoded[1]["key"] != "B" {
		t.Fatalf("decoded keys = %#v, want sorted A,B", decoded)
	}
	if _, ok := decoded[0]["value"]; ok {
		t.Fatalf("value field should be omitted: %#v", decoded[0])
	}
}

func TestWriteJSONListIncludesValuesWhenRequested(t *testing.T) {
	items := ListItemsWithValues(map[string]string{"B": "2", "A": "1"}, "prod")

	var buf bytes.Buffer
	if err := WriteJSON(&buf, items); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded[0]["vault"] != "prod" {
		t.Fatalf("vault = %#v, want prod", decoded[0]["vault"])
	}
	if decoded[0]["value"] != "1" || decoded[1]["value"] != "2" {
		t.Fatalf("decoded values = %#v, want sorted values 1,2", decoded)
	}
}

func TestWriteJSONGetResult(t *testing.T) {
	item := GetResult("API_KEY", "secret123", "default")

	var buf bytes.Buffer
	if err := WriteJSON(&buf, item); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded["key"] != "API_KEY" || decoded["value"] != "secret123" || decoded["vault"] != "default" {
		t.Fatalf("decoded = %#v", decoded)
	}
}
