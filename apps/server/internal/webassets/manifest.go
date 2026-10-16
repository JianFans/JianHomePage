package webassets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"unicode"
)

// manifestChunk is the browser dependency metadata emitted by the pinned Vite build.
type manifestChunk struct {
	File           string   `json:"file"`
	Name           string   `json:"name,omitempty"`
	Names          []string `json:"names,omitempty"`
	Src            string   `json:"src,omitempty"`
	IsEntry        bool     `json:"isEntry,omitempty"`
	IsDynamicEntry bool     `json:"isDynamicEntry,omitempty"`
	Imports        []string `json:"imports,omitempty"`
	DynamicImports []string `json:"dynamicImports,omitempty"`
	CSS            []string `json:"css,omitempty"`
	Assets         []string `json:"assets,omitempty"`
}

// publicAssets verifies the complete manifest graph before exposing browser output.
func publicAssets(root fs.FS) (fs.FS, error) {
	tree, err := assetTree(root, "public", "site/.vite/manifest.json")
	if err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(tree, "site/.vite/manifest.json")
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("embedded browser manifest is oversized")
	}
	var chunks map[string]*manifestChunk
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&chunks); err != nil {
		return nil, fmt.Errorf("embedded browser manifest is invalid: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || len(chunks) == 0 || len(chunks) > 10000 {
		return nil, errors.New("embedded browser manifest is invalid")
	}
	entries := 0
	for _, chunk := range chunks {
		if chunk == nil {
			return nil, errors.New("embedded browser manifest contains a missing chunk")
		}
		if err := requireBrowserFile(tree, chunk.File, ""); err != nil {
			return nil, err
		}
		if chunk.IsEntry {
			entries++
			if !scriptFile(chunk.File) {
				return nil, errors.New("embedded browser entry must be JavaScript")
			}
		}
		for _, stylesheet := range chunk.CSS {
			if err := requireBrowserFile(tree, stylesheet, ".css"); err != nil {
				return nil, err
			}
		}
		for _, asset := range chunk.Assets {
			if err := requireBrowserFile(tree, asset, ""); err != nil {
				return nil, err
			}
		}
		for _, imported := range append(append([]string{}, chunk.Imports...), chunk.DynamicImports...) {
			dependency, ok := chunks[imported]
			if !ok || dependency == nil || !scriptFile(dependency.File) {
				return nil, errors.New("embedded browser manifest references an unknown script chunk")
			}
		}
	}
	if entries != 1 {
		return nil, errors.New("embedded browser manifest must contain one entry")
	}
	return tree, nil
}

// requireBrowserFile rejects unsafe paths, private file types, and missing dependencies.
func requireBrowserFile(tree fs.FS, name, extension string) error {
	if !safeBrowserPath(name) || (extension != "" && path.Ext(name) != extension) {
		return errors.New("embedded browser manifest contains an unsafe file reference")
	}
	info, err := fs.Stat(tree, "site/"+name)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("embedded browser dependency %q is missing or empty", name)
	}
	return nil
}

// safeBrowserPath allows only canonical relative browser asset names within site/.
func safeBrowserPath(name string) bool {
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:?#") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if strings.HasPrefix(segment, ".") {
			return false
		}
	}
	switch path.Ext(name) {
	case ".js", ".mjs", ".css", ".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".ico", ".woff", ".woff2", ".ttf", ".otf", ".mp3", ".mp4", ".webm", ".ogg", ".wav":
		return true
	default:
		return false
	}
}

// scriptFile distinguishes executable manifest entries from styles and media.
func scriptFile(name string) bool { return path.Ext(name) == ".js" || path.Ext(name) == ".mjs" }
