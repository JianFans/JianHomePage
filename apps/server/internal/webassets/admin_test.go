package webassets

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// TestAdminEmbeddedBuildInventory verifies the build script and startup contract together.
func TestAdminEmbeddedBuildInventory(t *testing.T) {
	if _, err := Admin(); err != nil {
		t.Fatalf("generate embedded management assets before Go tests: %v", err)
	}
}

// TestAdminAssetsRejectIncompleteBuilds covers bundles damaged after frontend generation.
func TestAdminAssetsRejectIncompleteBuilds(t *testing.T) {
	valid := func() fstest.MapFS {
		return fstest.MapFS{
			"bundle/admin/index.html":         {Data: []byte(`<link rel="stylesheet" href="/_nuxt/app.css"><script type="module" src="/_nuxt/app.js"></script>`)},
			"bundle/admin/preview/index.html": {Data: []byte(`<link rel="modulepreload" href="_nuxt/preview.js"><script src="./_nuxt/app.js"></script>`)},
			"bundle/admin/_nuxt/app.js":       {Data: []byte("app()")},
			"bundle/admin/_nuxt/preview.js":   {Data: []byte("preview()")},
			"bundle/admin/_nuxt/app.css":      {Data: []byte("body{}")},
		}
	}
	for _, test := range []struct {
		name   string
		change func(fstest.MapFS)
	}{
		{"missing index", func(files fstest.MapFS) { delete(files, "bundle/admin/index.html") }},
		{"missing preview", func(files fstest.MapFS) { delete(files, "bundle/admin/preview/index.html") }},
		{"empty preview", func(files fstest.MapFS) { files["bundle/admin/preview/index.html"].Data = nil }},
		{"missing script", func(files fstest.MapFS) { delete(files, "bundle/admin/_nuxt/app.js") }},
		{"empty script", func(files fstest.MapFS) { files["bundle/admin/_nuxt/app.js"].Data = nil }},
		{"missing style", func(files fstest.MapFS) { delete(files, "bundle/admin/_nuxt/app.css") }},
		{"empty style", func(files fstest.MapFS) { files["bundle/admin/_nuxt/app.css"].Data = nil }},
		{"missing preload", func(files fstest.MapFS) { delete(files, "bundle/admin/_nuxt/preview.js") }},
		{"no entry script", func(files fstest.MapFS) {
			files["bundle/admin/index.html"].Data = []byte(`<html><script>{}</script></html>`)
		}},
		{"preview without entry script", func(files fstest.MapFS) { files["bundle/admin/preview/index.html"].Data = []byte(`<html></html>`) }},
		{"traversal", func(files fstest.MapFS) {
			files["bundle/admin/index.html"].Data = []byte(`<script src="/_nuxt/../app.js"></script>`)
		}},
		{"encoded traversal", func(files fstest.MapFS) {
			files["bundle/admin/index.html"].Data = []byte(`<script src="/_nuxt/%2e%2e/app.js"></script>`)
		}},
		{"remote script", func(files fstest.MapFS) {
			files["bundle/admin/index.html"].Data = []byte(`<script src="https://remote.example/_nuxt/app.js"></script>`)
		}},
		{"protocol relative script", func(files fstest.MapFS) {
			files["bundle/admin/index.html"].Data = []byte(`<script src="//remote.example/_nuxt/app.js"></script>`)
		}},
		{"private script", func(files fstest.MapFS) {
			files["bundle/admin/index.html"].Data = []byte(`<script src="/_nuxt/app.js.map"></script>`)
		}},
		{"script query", func(files fstest.MapFS) {
			files["bundle/admin/index.html"].Data = []byte(`<script src="/_nuxt/app.js?x=1"></script>`)
		}},
		{"prefetch query", func(files fstest.MapFS) {
			files["bundle/admin/index.html"].Data = []byte(`<link rel="prefetch" href="/_nuxt/preview.js?x=1"><script src="/_nuxt/app.js"></script>`)
		}},
		{"base override", func(files fstest.MapFS) {
			files["bundle/admin/index.html"].Data = []byte(`<base href="https://remote.example/"><script src="/_nuxt/app.js"></script>`)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := valid()
			test.change(files)
			addAdminManifest(t, files)
			if _, err := assetTree(files, "admin", "index.html"); err == nil {
				t.Fatal("accepted incomplete or unsafe management build")
			}
		})
	}
	t.Run("absolute and relative assets", func(t *testing.T) {
		files := valid()
		addAdminManifest(t, files)
		if _, err := assetTree(files, "admin", "index.html"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("flat preview", func(t *testing.T) {
		files := valid()
		files["bundle/admin/preview.html"] = files["bundle/admin/preview/index.html"]
		delete(files, "bundle/admin/preview/index.html")
		addAdminManifest(t, files)
		if _, err := assetTree(files, "admin", "index.html"); err != nil {
			t.Fatal(err)
		}
	})
}

// addAdminManifest models the full inventory generated after frontend compilation.
func addAdminManifest(t *testing.T, files fstest.MapFS) {
	t.Helper()
	var entries []map[string]any
	var names []string
	for name := range files {
		if name != "bundle/admin/asset-manifest.json" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		data := files[name].Data
		entries = append(entries, map[string]any{"path": strings.TrimPrefix(name, "bundle/admin/"), "size": len(data), "sha256": fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	data, err := json.Marshal(map[string]any{"version": 1, "files": entries})
	if err != nil {
		t.Fatal(err)
	}
	files["bundle/admin/asset-manifest.json"] = &fstest.MapFile{Data: data}
}

// TestAdminManifestProtectsDynamicDependencies covers chunks absent from entry HTML.
func TestAdminManifestProtectsDynamicDependencies(t *testing.T) {
	valid := func() fstest.MapFS {
		files := fstest.MapFS{
			"bundle/admin/index.html":                   {Data: []byte(`<script src="/_nuxt/app.js"></script>`)},
			"bundle/admin/preview.html":                 {Data: []byte(`<script src="/_nuxt/app.js"></script>`)},
			"bundle/admin/_nuxt/app.js":                 {Data: []byte(`import('./dynamic.js')`)},
			"bundle/admin/_nuxt/dynamic.js":             {Data: []byte("route()")},
			"bundle/admin/_nuxt/dynamic.css":            {Data: []byte("body{}")},
			"bundle/admin/_nuxt/builds/meta/build.json": {Data: []byte(`{"prerendered":[]}`)},
		}
		addAdminManifest(t, files)
		return files
	}
	for _, test := range []struct {
		name   string
		change func(fstest.MapFS)
	}{
		{"missing dynamic script", func(files fstest.MapFS) { delete(files, "bundle/admin/_nuxt/dynamic.js") }},
		{"empty dynamic script", func(files fstest.MapFS) { files["bundle/admin/_nuxt/dynamic.js"].Data = nil }},
		{"corrupt same size script", func(files fstest.MapFS) { files["bundle/admin/_nuxt/dynamic.js"].Data = []byte("other()") }},
		{"missing dynamic style", func(files fstest.MapFS) { delete(files, "bundle/admin/_nuxt/dynamic.css") }},
		{"unlisted chunk", func(files fstest.MapFS) {
			files["bundle/admin/_nuxt/stale.js"] = &fstest.MapFile{Data: []byte("stale()")}
		}},
		{"missing manifest", func(files fstest.MapFS) { delete(files, "bundle/admin/asset-manifest.json") }},
		{"empty manifest", func(files fstest.MapFS) { files["bundle/admin/asset-manifest.json"].Data = nil }},
		{"invalid manifest", func(files fstest.MapFS) { files["bundle/admin/asset-manifest.json"].Data = []byte("{") }},
		{"unsafe path", func(files fstest.MapFS) {
			files["bundle/admin/asset-manifest.json"].Data = []byte(`{"version":1,"files":[{"path":"../private.js","size":1,"sha256":"a"}]}`)
		}},
		{"unknown manifest version", func(files fstest.MapFS) {
			files["bundle/admin/asset-manifest.json"].Data = []byte(`{"version":2,"files":[]}`)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := valid()
			test.change(files)
			if _, err := assetTree(files, "admin", "index.html"); err == nil {
				t.Fatal("accepted incomplete dynamic management dependencies")
			}
		})
	}
	if _, err := assetTree(valid(), "admin", "index.html"); err != nil {
		t.Fatal(err)
	}
}
