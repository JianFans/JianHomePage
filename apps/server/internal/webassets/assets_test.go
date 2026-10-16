package webassets

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

// TestAssetTreeRequiresTheBuiltEntrypoint verifies incomplete builds fail closed.
func TestAssetTreeRequiresTheBuiltEntrypoint(t *testing.T) {
	for _, name := range []string{"public"} {
		entry := "index.html"
		if name == "public" {
			entry = "site/.vite/manifest.json"
		}
		if _, err := assetTree(fstest.MapFS{}, name, entry); err == nil {
			t.Fatal("accepted missing assets")
		}
		files := fstest.MapFS{"bundle/" + name + "/" + entry: {Data: []byte("built")}}
		tree, err := assetTree(files, name, entry)
		if err != nil {
			t.Fatal(err)
		}
		data, err := fs.ReadFile(tree, entry)
		if err != nil || string(data) != "built" {
			t.Fatalf("entry = %q, %v", data, err)
		}
		files["bundle/"+name+"/"+entry] = &fstest.MapFile{}
		if _, err := assetTree(files, name, entry); err == nil {
			t.Fatal("accepted empty entry")
		}
	}
}

// TestRendererScriptRequiresRealCode verifies no renderer fallback is supplied.
func TestRendererScriptRequiresRealCode(t *testing.T) {
	for _, files := range []fs.FS{fstest.MapFS{}, fstest.MapFS{"bundle/renderer.js": {}}} {
		if _, err := rendererScript(files); err == nil {
			t.Fatal("accepted missing renderer")
		}
	}
	code, err := rendererScript(fstest.MapFS{"bundle/renderer.js": {Data: []byte("function render(){}")}})
	if err != nil || string(code) != "function render(){}" {
		t.Fatalf("renderer = %q, %v", code, err)
	}
}
