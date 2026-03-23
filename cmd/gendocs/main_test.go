package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra/doc"
	"github.com/v-gutierrez/kc/internal/cli"
)

func TestGenManTreeProducesManPages(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "man")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	app := &cli.App{}
	root := cli.NewRootCmd(app)
	root.DisableAutoGenTag = true

	header := &doc.GenManHeader{
		Title:   "KC",
		Section: "1",
		Date:    &time.Time{},
		Source:  "kc dev",
		Manual:  "kc Manual",
	}

	if err := doc.GenManTree(root, header, outDir); err != nil {
		t.Fatalf("GenManTree failed: %v", err)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no man pages generated")
	}

	foundRoot := false
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".1") {
			t.Errorf("unexpected file extension: %s", e.Name())
		}
		if e.Name() == "kc.1" {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Error("missing root man page kc.1")
	}

	data, err := os.ReadFile(filepath.Join(outDir, "kc.1"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "KC") {
		t.Errorf("root man page missing title 'KC': %s", content[:200])
	}
}

func TestGenManTreeIncludesSubcommands(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "man")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	app := &cli.App{}
	root := cli.NewRootCmd(app)
	root.DisableAutoGenTag = true

	header := &doc.GenManHeader{
		Title:   "KC",
		Section: "1",
		Date:    &time.Time{},
	}

	if err := doc.GenManTree(root, header, outDir); err != nil {
		t.Fatalf("GenManTree failed: %v", err)
	}

	expected := []string{"kc-get.1", "kc-set.1", "kc-del.1", "kc-list.1", "kc-completion.1", "kc-vault.1"}
	for _, name := range expected {
		path := filepath.Join(outDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("missing man page: %s", name)
		}
	}
}
