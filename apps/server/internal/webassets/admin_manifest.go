package webassets

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"unicode"
)

const adminManifestName = "asset-manifest.json"

type adminManifest struct {
	Version int                 `json:"version"`
	Files   []adminManifestFile `json:"files"`
}

type adminManifestFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// validateAdminManifest checks all generated files, including lazy route chunks.
func validateAdminManifest(tree fs.FS) error {
	data, err := fs.ReadFile(tree, adminManifestName)
	if err != nil || len(data) == 0 || len(data) > 2<<20 {
		return errors.New("embedded management asset manifest is missing, empty or oversized")
	}
	var manifest adminManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("invalid management asset manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || manifest.Version != 1 || len(manifest.Files) == 0 || len(manifest.Files) > 10000 {
		return errors.New("invalid management asset manifest")
	}
	listed := make(map[string]bool, len(manifest.Files))
	for _, file := range manifest.Files {
		if !safeAdminManifestPath(file.Path) || listed[file.Path] || file.Size <= 0 || len(file.SHA256) != 64 {
			return errors.New("management asset manifest contains an invalid file")
		}
		listed[file.Path] = true
		info, err := fs.Stat(tree, file.Path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != file.Size {
			return fmt.Errorf("management asset %q is missing, empty or has changed size", file.Path)
		}
		resource, err := tree.Open(file.Path)
		if err != nil {
			return fmt.Errorf("open management asset %q: %w", file.Path, err)
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, resource)
		closeErr := resource.Close()
		if copyErr != nil || closeErr != nil || fmt.Sprintf("%x", digest.Sum(nil)) != file.SHA256 {
			return fmt.Errorf("management asset %q checksum differs from the build manifest", file.Path)
		}
	}
	return fs.WalkDir(tree, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || name == adminManifestName {
			return nil
		}
		if !listed[name] {
			return fmt.Errorf("management asset %q is absent from the build manifest", name)
		}
		return nil
	})
}

// safeAdminManifestPath permits HTML and payloads as well as browser chunks,
// while excluding ambiguous paths and the inventory's own recursive entry.
func safeAdminManifestPath(name string) bool {
	if name == adminManifestName || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:?#%") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if strings.HasPrefix(segment, ".") {
			return false
		}
	}
	return true
}
