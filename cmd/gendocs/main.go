package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra/doc"
	"github.com/v-gutierrez/kc/internal/cli"
)

func main() {
	outDir := "man"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gendocs: mkdir %s: %v\n", outDir, err)
		os.Exit(1)
	}

	app := &cli.App{}
	root := cli.NewRootCmd(app)
	root.DisableAutoGenTag = true

	header := &doc.GenManHeader{
		Title:   "KC",
		Section: "1",
		Date:    &time.Time{},
		Source:  "kc " + cli.Version,
		Manual:  "kc Manual",
	}

	if err := doc.GenManTree(root, header, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "gendocs: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Man pages generated in %s/\n", outDir)
}
