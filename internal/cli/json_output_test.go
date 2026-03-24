package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestListJSON(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "B", "2")
	store.Set("default", "A", "1")

	stdout, stderr, err := executeCmd(app, "list", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	var decoded []map[string]any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v\nstdout=%q", err, stdout)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded length = %d, want 2", len(decoded))
	}
	if decoded[0]["key"] != "A" || decoded[1]["key"] != "B" {
		t.Fatalf("decoded = %#v, want sorted keys A,B", decoded)
	}
	if _, ok := decoded[0]["value"]; ok {
		t.Fatalf("value field should be omitted by default: %#v", decoded[0])
	}
	if decoded[0]["vault"] != "default" {
		t.Fatalf("vault = %#v, want default", decoded[0]["vault"])
	}
}

func TestListJSONShowValues(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "API_KEY", "secret123")

	stdout, _, err := executeCmd(app, "list", "--json", "--show-values")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v\nstdout=%q", err, stdout)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded length = %d, want 1", len(decoded))
	}
	if decoded[0]["value"] != "secret123" {
		t.Fatalf("value = %#v, want secret123", decoded[0]["value"])
	}
}

func TestListJSONEmpty(t *testing.T) {
	app, _, _, _ := newTestApp()

	stdout, _, err := executeCmd(app, "list", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Fatalf("stdout = %q, want []", strings.TrimSpace(stdout))
	}
}

func TestGetJSON(t *testing.T) {
	app, store, _, clip := newTestApp()
	store.Set("default", "API_KEY", "secret123")

	stdout, stderr, err := executeCmd(app, "get", "API_KEY", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if clip.last != "" {
		t.Fatalf("clipboard = %q, want no clipboard copy in json mode", clip.last)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v\nstdout=%q", err, stdout)
	}
	if decoded["key"] != "API_KEY" || decoded["value"] != "secret123" || decoded["vault"] != "default" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestListShowValuesText(t *testing.T) {
	app, store, _, _ := newTestApp()
	store.Set("default", "API_KEY", "secret123")

	stdout, _, err := executeCmd(app, "list", "--show-values")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "API_KEY=secret123") {
		t.Fatalf("stdout = %q, want API_KEY=secret123", stdout)
	}
}
