package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportSiteRunsFromEmbeddedResources verifies offline HTML generation and
// refuses to overwrite an existing export directory on a second invocation.
func TestExportSiteRunsFromEmbeddedResources(t *testing.T) {
	output := filepath.Join(t.TempDir(), "site")
	var log bytes.Buffer
	handled, err := runCommand([]string{"--export-site", "../../../../content/fixtures/homepage.json", "--output", output}, &log)
	if err != nil || !handled {
		t.Fatalf("export failed: %v", err)
	}
	html, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil || !strings.Contains(string(html), "遇健我") {
		t.Fatalf("invalid homepage: %v", err)
	}
	if _, err := runCommand([]string{"--export-site", "../../../../content/fixtures/homepage.json", "--output", output}, &log); err == nil {
		t.Fatal("overwrote nonempty output directory")
	}
}

// TestExportCommandRejectsUnknownOrIncompleteOptions keeps export errors from
// silently falling through to normal server startup.
func TestExportCommandRejectsUnknownOrIncompleteOptions(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"--export-site"}, {"--export-site", "x"}, {"--export-site", "x", "--output", "."}} {
		if handled, err := runCommand(args, &bytes.Buffer{}); !handled || err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if handled, err := runCommand(nil, &bytes.Buffer{}); handled || err != nil {
		t.Fatal("default server invocation intercepted")
	}
}
