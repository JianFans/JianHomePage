package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncFileCopiesContractAndCreatesDestinationDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source", "contract.json")
	destination := filepath.Join(root, "nested", "destination", "contract.json")
	writeTestFile(t, source, []byte(`{"title":"homepage"}`))

	if err := syncFile(source, destination); err != nil {
		t.Fatalf("sync contract: %v", err)
	}
	actual, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read synchronized contract: %v", err)
	}
	if !bytes.Equal(actual, []byte(`{"title":"homepage"}`)) {
		t.Fatalf("unexpected synchronized contract %q", actual)
	}
}

func TestSyncFileDoesNotRewriteMatchingContract(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.json")
	destination := filepath.Join(root, "destination.json")
	contract := []byte(`{"title":"homepage"}`)
	writeTestFile(t, source, contract)
	writeTestFile(t, destination, contract)
	wantModTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(destination, wantModTime, wantModTime); err != nil {
		t.Fatalf("set destination timestamp: %v", err)
	}

	if err := syncFile(source, destination); err != nil {
		t.Fatalf("sync matching contract: %v", err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("stat destination: %v", err)
	}
	if !info.ModTime().Equal(wantModTime) {
		t.Fatalf("matching contract was rewritten: modtime=%s", info.ModTime())
	}
}

func TestSyncFileRejectsMissingSource(t *testing.T) {
	root := t.TempDir()
	err := syncFile(filepath.Join(root, "missing.json"), filepath.Join(root, "destination.json"))
	if err == nil || !strings.Contains(err.Error(), "read ") {
		t.Fatalf("expected source read error, got %v", err)
	}
}

func TestSyncFileRejectsEmptyContract(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "empty.json")
	writeTestFile(t, source, nil)

	err := syncFile(source, filepath.Join(root, "destination.json"))
	if err == nil || !strings.Contains(err.Error(), "empty contract") {
		t.Fatalf("expected empty contract error, got %v", err)
	}
}

func TestSyncFileReturnsDestinationDirectoryError(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.json")
	blockedParent := filepath.Join(root, "blocked")
	writeTestFile(t, source, []byte(`{"title":"homepage"}`))
	writeTestFile(t, blockedParent, []byte("not a directory"))

	err := syncFile(source, filepath.Join(blockedParent, "contract.json"))
	if err == nil || !strings.Contains(err.Error(), "create destination directory") {
		t.Fatalf("expected destination directory error, got %v", err)
	}
}

func TestSyncFileReturnsDestinationWriteError(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.json")
	destination := filepath.Join(root, "destination")
	writeTestFile(t, source, []byte(`{"title":"homepage"}`))
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatalf("create destination directory: %v", err)
	}

	err := syncFile(source, destination)
	if err == nil || !strings.Contains(err.Error(), "write ") {
		t.Fatalf("expected destination write error, got %v", err)
	}
}

func TestMainSynchronizesSchemaAndOpenAPIContracts(t *testing.T) {
	workspace, serverDir := prepareMainWorkspace(t, true)
	t.Chdir(serverDir)

	main()

	assertFileMatches(t,
		filepath.Join(workspace, "packages", "schema", "schema", "content-snapshot.schema.json"),
		filepath.Join(serverDir, "internal", "contract", "schema.json"),
	)
	assertFileMatches(t,
		filepath.Join(workspace, "packages", "schema", "openapi", "admin.yaml"),
		filepath.Join(serverDir, "internal", "httpapi", "openapi.yaml"),
	)
}

func TestMainPanicsWhenAContractCannotBeRead(t *testing.T) {
	_, serverDir := prepareMainWorkspace(t, false)
	t.Chdir(serverDir)

	defer func() {
		value := recover()
		if value == nil {
			t.Fatal("expected missing OpenAPI contract to panic")
		}
		if !strings.Contains(value.(error).Error(), "admin.yaml") {
			t.Fatalf("unexpected panic: %v", value)
		}
	}()
	main()
}

func prepareMainWorkspace(t *testing.T, includeOpenAPI bool) (string, string) {
	t.Helper()
	workspace := t.TempDir()
	serverDir := filepath.Join(workspace, "apps", "server")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatalf("create server directory: %v", err)
	}
	writeTestFile(t,
		filepath.Join(workspace, "packages", "schema", "schema", "content-snapshot.schema.json"),
		[]byte(`{"title":"homepage"}`),
	)
	if includeOpenAPI {
		writeTestFile(t,
			filepath.Join(workspace, "packages", "schema", "openapi", "admin.yaml"),
			[]byte("openapi: 3.1.0\n"),
		)
	}
	return workspace, serverDir
}

func writeTestFile(t *testing.T, path string, value []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create test file parent: %v", err)
	}
	if err := os.WriteFile(path, value, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

func assertFileMatches(t *testing.T, source, destination string) {
	t.Helper()
	want, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source %s: %v", source, err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination %s: %v", destination, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("destination %s does not match source", destination)
	}
}
