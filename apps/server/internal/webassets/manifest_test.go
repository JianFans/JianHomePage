package webassets

import (
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"
)

// publicFiles supplies a complete Vite manifest with shared and dynamic chunks.
func publicFiles(t *testing.T, manifest map[string]any) fstest.MapFS {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		"bundle/public/site/.vite/manifest.json":        {Data: data},
		"bundle/public/site/assets/client-abcd1234.js":  {Data: []byte("entry")},
		"bundle/public/site/assets/shared-abcd1234.js":  {Data: []byte("shared")},
		"bundle/public/site/assets/lazy-abcd1234.js":    {Data: []byte("lazy")},
		"bundle/public/site/assets/style-abcd1234.css":  {Data: []byte("style")},
		"bundle/public/site/assets/font-abcd1234.woff2": {Data: []byte("font")},
	}
}

// publicManifest represents the build-generated browser dependency graph.
func publicManifest() map[string]any {
	return map[string]any{
		"standalone/client.ts": map[string]any{"file": "assets/client-abcd1234.js", "name": "client", "src": "standalone/client.ts", "isEntry": true, "imports": []string{"_shared.js"}, "dynamicImports": []string{"lazy.ts"}, "css": []string{"assets/style-abcd1234.css"}, "assets": []string{"assets/font-abcd1234.woff2"}},
		"_shared.js":           map[string]any{"file": "assets/shared-abcd1234.js"},
		"lazy.ts":              map[string]any{"file": "assets/lazy-abcd1234.js", "isDynamicEntry": true},
	}
}

// TestPublicAssetsRequireTheCompleteManifestGraph rejects partial release bundles.
func TestPublicAssetsRequireTheCompleteManifestGraph(t *testing.T) {
	files := publicFiles(t, publicManifest())
	assets, err := publicAssets(files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile(assets, "site/assets/client-abcd1234.js"); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"client-abcd1234.js", "shared-abcd1234.js", "lazy-abcd1234.js", "style-abcd1234.css", "font-abcd1234.woff2"} {
		t.Run(filename, func(t *testing.T) {
			files := publicFiles(t, publicManifest())
			delete(files, "bundle/public/site/assets/"+filename)
			if _, err := publicAssets(files); err == nil {
				t.Fatal("accepted missing manifest dependency")
			}
			files["bundle/public/site/assets/"+filename] = &fstest.MapFile{}
			if _, err := publicAssets(files); err == nil {
				t.Fatal("accepted empty manifest dependency")
			}
		})
	}
}

// TestPublicAssetsRejectInvalidManifestStructure rejects ambiguous build identities.
func TestPublicAssetsRejectInvalidManifestStructure(t *testing.T) {
	for _, raw := range []string{`{}`, `[]`, `null`, `not-json`, `{} {}`, `{"entry":{"file":"assets/client-abcd1234.js","isEntry":"yes"}}`, `{"entry":{"file":"assets/client-abcd1234.js"}}`, `{"one":{"file":"assets/client-abcd1234.js","isEntry":true},"two":{"file":"assets/shared-abcd1234.js","isEntry":true}}`, `{"entry":{"file":"assets/style-abcd1234.css","isEntry":true}}`} {
		t.Run(raw, func(t *testing.T) {
			files := publicFiles(t, publicManifest())
			files["bundle/public/site/.vite/manifest.json"] = &fstest.MapFile{Data: []byte(raw)}
			if _, err := publicAssets(files); err == nil {
				t.Fatal("accepted malformed manifest")
			}
		})
	}
}

// TestPublicAssetsRejectUnsafeReferences prevents publishing private or remote files.
func TestPublicAssetsRejectUnsafeReferences(t *testing.T) {
	for _, ref := range []string{"../client.js", "/assets/client.js", "assets/../client.js", "assets//client.js", "assets\\client.js", "https://example.test/client.js", "assets/client.js?query", "assets/client.js#fragment", "assets/client.js.map", "assets/.env", "assets/server.ts", "assets/config.json", "assets/hidden\x00.js"} {
		t.Run(ref, func(t *testing.T) {
			manifest := publicManifest()
			manifest["standalone/client.ts"].(map[string]any)["file"] = ref
			files := publicFiles(t, manifest)
			files["bundle/public/site/"+ref] = &fstest.MapFile{Data: []byte("private")}
			if _, err := publicAssets(files); err == nil {
				t.Fatal("accepted unsafe manifest reference")
			}
		})
	}
	for _, field := range []string{"imports", "dynamicImports"} {
		manifest := publicManifest()
		manifest["standalone/client.ts"].(map[string]any)[field] = []string{"unknown.js"}
		if _, err := publicAssets(publicFiles(t, manifest)); err == nil {
			t.Fatalf("accepted unknown %s", field)
		}
	}
	manifest := publicManifest()
	manifest["standalone/client.ts"].(map[string]any)["css"] = []string{"assets/client-abcd1234.js"}
	if _, err := publicAssets(publicFiles(t, manifest)); err == nil {
		t.Fatal("accepted non-CSS stylesheet")
	}
}
