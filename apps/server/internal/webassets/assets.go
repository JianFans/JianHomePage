// Package webassets exposes immutable browser assets compiled into the server.
package webassets

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:bundle
var bundle embed.FS

// Admin returns the built management application, refusing missing build output.
func Admin() (fs.FS, error) {
	return assetTree(bundle, "admin", "index.html")
}

// Public returns the browser renderer assets required by generated homepages.
func Public() (fs.FS, error) {
	return publicAssets(bundle)
}

// Renderer returns the build-time compiled renderer without a development fallback.
func Renderer() ([]byte, error) {
	return rendererScript(bundle)
}

// assetTree requires built entries and validates management browser dependencies.
func assetTree(root fs.FS, name, entry string) (fs.FS, error) {
	tree, err := fs.Sub(root, "bundle/"+name)
	if err != nil {
		return nil, fmt.Errorf("open embedded %s assets: %w", name, err)
	}
	data, err := fs.ReadFile(tree, entry)
	if err != nil {
		return nil, fmt.Errorf("embedded %s assets are incomplete: %w", name, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("embedded %s entry is empty", name)
	}
	if name == "admin" {
		if err := validateAdmin(tree); err != nil {
			return nil, err
		}
	}
	return tree, nil
}

// rendererScript validates that the embedded renderer contains actual code.
func rendererScript(root fs.FS) ([]byte, error) {
	data, err := fs.ReadFile(root, "bundle/renderer.js")
	if err != nil {
		return nil, fmt.Errorf("embedded renderer is missing: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("embedded renderer is empty")
	}
	return data, nil
}
