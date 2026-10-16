package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type identityGenerator struct{ calls int }

// Identity intentionally matches only render assets to prove the wrapper overrides it.
func (*identityGenerator) Identity() string { return "asset-only-identity" }

// Generate preserves the injected generator's content behavior through the identity wrapper.
func (g *identityGenerator) Generate(_ context.Context, raw []byte) (map[string][]byte, error) {
	g.calls++
	return map[string][]byte{"index.html": append([]byte(nil), raw...)}, nil
}

// TestFreezeIdentityIncludesRunningBinary covers Go code and embedded assets with one immutable hash.
func TestFreezeIdentityIncludesRunningBinary(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	delegate := &identityGenerator{}
	frozen, err := freezeExecutableIdentity(delegate)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Identity() != fmt.Sprintf("%x", hash.Sum(nil)) || frozen.Identity() == delegate.Identity() {
		t.Fatalf("identity did not cover running program: %s", frozen.Identity())
	}
	files, err := frozen.Generate(t.Context(), []byte("snapshot"))
	if err != nil || string(files["index.html"]) != "snapshot" || delegate.calls != 1 {
		t.Fatalf("render delegation changed: %q %v", files, err)
	}
}

// TestExecutableHashChangesWithGoBinaryBytes freezes code changes even when browser resources match.
func TestExecutableHashChangesWithGoBinaryBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server")
	for _, data := range []string{"go-code-one|same-browser-assets", "go-code-two|same-browser-assets"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		identity, err := hashExecutable(path)
		if err != nil || identity != fmt.Sprintf("%x", sha256.Sum256([]byte(data))) {
			t.Fatalf("binary identity %s %v", identity, err)
		}
	}
}

// TestExecutableHashFailsClosed rejects unavailable executables without a mutable fallback.
func TestExecutableHashFailsClosed(t *testing.T) {
	if _, err := hashExecutable(filepath.Join(t.TempDir(), "missing-server")); err == nil {
		t.Fatal("missing executable accepted")
	}
	if _, err := hashExecutable(t.TempDir()); err == nil {
		t.Fatal("directory accepted as executable")
	}
}
