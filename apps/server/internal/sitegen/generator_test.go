package sitegen

import (
	"context"
	"encoding/json"
	"github.com/dop251/goja"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"yujian.me/server/internal/webassets"
)

// TestEmbeddedVueRendersSnapshotWithoutNode exercises the real bundled Vue
// renderer and verifies a later snapshot cannot reuse stale rendered content.
func TestEmbeddedVueRendersSnapshotWithoutNode(t *testing.T) {
	raw, err := os.ReadFile("../../../../content/fixtures/homepage.json")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := webassets.Renderer()
	if err != nil {
		t.Fatal(err)
	}
	assets, err := webassets.Public()
	if err != nil {
		t.Fatal(err)
	}
	generator, err := New(renderer, assets)
	if err != nil {
		t.Fatal(err)
	}
	files, err := generator.Generate(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	html := string(files["index.html"])
	for _, fragment := range []string{"<!doctype html>", "遇健我", "王子健", `id="music"`, `id="video"`, `application/ld+json`, `rel="canonical"`, `id="yujian-snapshot"`} {
		if !strings.Contains(html, fragment) {
			t.Errorf("rendered page missing %q", fragment)
		}
	}
	if !strings.Contains(string(files["robots.txt"]), "https://yujian.me/sitemap.xml") {
		t.Fatal("missing robots sitemap")
	}
	updated := strings.Replace(string(raw), "遇健我", "新的站名", -1)
	updated = strings.ReplaceAll(updated, "/media/hero-studio.webp", "https://media.example.test/new-cover.webp")
	changed, err := generator.Generate(context.Background(), []byte(updated))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changed["index.html"]), "新的站名") {
		t.Fatal("render used stale content")
	}
	if !strings.Contains(string(changed["index.html"]), `property="og:image" content="https://media.example.test/new-cover.webp"`) ||
		!strings.Contains(string(changed["index.html"]), `src="https://media.example.test/new-cover.webp"`) ||
		strings.Contains(string(changed["index.html"]), "/media/hero-studio.webp") {
		t.Fatal("updated media was not reflected in both page content and SEO")
	}
}

// embeddedGenerator uses the real Vue bundle so missing component markup cannot
// be hidden by a mock renderer in escaping or section-visibility tests.
func embeddedGenerator(t *testing.T) (*Generator, map[string]any) {
	t.Helper()
	renderer, err := webassets.Renderer()
	if err != nil {
		t.Fatal(err)
	}
	assets, err := webassets.Public()
	if err != nil {
		t.Fatal(err)
	}
	generator, err := New(renderer, assets)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../../content/fixtures/homepage.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	return generator, snapshot
}

// TestInitialContentRendersWithoutMedia verifies the first real draft exports without broken images.
func TestInitialContentRendersWithoutMedia(t *testing.T) {
	generator, value := embeddedGenerator(t)
	delete(value["site"].(map[string]any)["seo"].(map[string]any), "ogAssetId")
	delete(value["artist"].(map[string]any), "portraitAssetId")
	for _, field := range []string{"heroSlides", "releases", "tracks", "videos", "events", "moments", "assets"} {
		value[field] = []any{}
	}
	for _, raw := range value["homepage"].(map[string]any)["sections"].([]any) {
		section := raw.(map[string]any)
		if section["type"] != "artist" {
			section["itemIds"] = []any{}
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	files, err := generator.Generate(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	html := string(files["index.html"])
	if !strings.Contains(html, "遇健我") || !strings.Contains(html, `id="artist"`) ||
		!strings.Contains(html, `name="twitter:card" content="summary"`) {
		t.Fatal("initial content did not render site information")
	}
	if strings.Contains(html, `property="og:image"`) || strings.Contains(html, `src="undefined"`) || strings.Contains(html, `id="music"`) {
		t.Fatal("initial content emitted absent media or an empty section")
	}
}

// TestGeneratedHTMLPreservesDataWithoutExecutableInjection distinguishes safe
// display escaping from lossless JSON payload escaping during hydration.
func TestGeneratedHTMLPreservesDataWithoutExecutableInjection(t *testing.T) {
	generator, snapshot := embeddedGenerator(t)
	malicious := `</script><script>alert("x")</script>`
	snapshot["site"].(map[string]any)["brand"].(map[string]any)["zh-CN"] = malicious
	raw, _ := json.Marshal(snapshot)
	files, err := generator.Generate(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	html := string(files["index.html"])
	if strings.Contains(html, malicious) || !strings.Contains(html, `&lt;/script&gt;`) || !strings.Contains(html, `\u003c/script\u003e`) {
		t.Fatal("content escaped incorrectly")
	}
	start := strings.Index(html, `<script type="application/json" id="yujian-snapshot">`) + len(`<script type="application/json" id="yujian-snapshot">`)
	end := strings.Index(html[start:], "</script>") + start
	var restored map[string]any
	if err := json.Unmarshal([]byte(html[start:end]), &restored); err != nil {
		t.Fatal(err)
	}
	if restored["site"].(map[string]any)["brand"].(map[string]any)["zh-CN"] != malicious {
		t.Fatal("escaped payload changed content")
	}
}

// TestRenderingRespectsSectionsAndDefaultLocale checks initial crawler-visible
// markup against content settings before any browser script executes.
func TestRenderingRespectsSectionsAndDefaultLocale(t *testing.T) {
	generator, snapshot := embeddedGenerator(t)
	for _, section := range snapshot["homepage"].(map[string]any)["sections"].([]any) {
		if section.(map[string]any)["type"] == "music" {
			section.(map[string]any)["enabled"] = false
		}
	}
	for _, slide := range snapshot["heroSlides"].([]any) {
		delete(slide.(map[string]any), "target")
	}
	raw, _ := json.Marshal(snapshot)
	files, err := generator.Generate(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	html := string(files["index.html"])
	if strings.Contains(html, `id="music"`) || !strings.Contains(html, `<html lang="zh-CN">`) || !strings.Contains(html, "遇健我") {
		t.Fatal("ignored default locale or disabled section")
	}
	if strings.Contains(html, "/api/") {
		t.Fatal("public document depends on management API")
	}
}

// TestRendererURLMatchesBrowserNormalization guards hydration against Go URL
// parsing differences for case, international domains and IPv6 literals.
func TestRendererURLMatchesBrowserNormalization(t *testing.T) {
	vm := goja.New()
	if err := installBrowserPrimitives(vm); err != nil {
		t.Fatal(err)
	}
	for raw, expected := range map[string]string{"https://YUJiAN.me": "yujian.me", "https://例子.测试": "xn--fsqu00a.xn--0zwm56d", "https://[::1]": "[::1]"} {
		_ = vm.Set("input", raw)
		value, err := vm.RunString(`new URL(input).hostname`)
		if err != nil || value.String() != expected {
			t.Errorf("URL(%q).hostname = %v want %s: %v", raw, value, expected, err)
		}
	}
}

// TestRendererBoundsAndUnavailableCapabilities checks cancellation, output and
// input limits, and absence of network/module APIs in the embedded VM.
func TestRendererBoundsAndUnavailableCapabilities(t *testing.T) {
	_, snapshot := embeddedGenerator(t)
	raw, _ := json.Marshal(snapshot)
	assets := fstest.MapFS{"site/client.js": &fstest.MapFile{Data: []byte("export{}")}}
	for name, script := range map[string]string{
		"pending":   `globalThis.__render = () => new Promise(() => {});`,
		"rejected":  `globalThis.__render = () => Promise.reject(new Error('invalid'));`,
		"empty":     `globalThis.__render = () => Promise.resolve({html:'',robots:'',sitemap:''});`,
		"oversized": `globalThis.__render = () => Promise.resolve({html:'a'.repeat(33554433),robots:'',sitemap:''});`,
		"noentry":   `globalThis.x=1;`,
		"network":   `globalThis.__render = () => Promise.resolve(fetch('https://example.test'));`,
		"process":   `globalThis.__render = () => Promise.resolve(require('child_process'));`,
	} {
		t.Run(name, func(t *testing.T) {
			generator, err := New([]byte(script), assets)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := generator.Generate(context.Background(), raw); err == nil {
				t.Fatal("accepted invalid renderer")
			}
		})
	}
	generator, err := New([]byte(`while(true){}`), assets)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := generator.Generate(ctx, raw); err == nil {
		t.Fatal("infinite renderer completed")
	}
	if time.Since(start) > time.Second {
		t.Fatal("renderer ignored cancellation")
	}
	for _, invalid := range [][]byte{nil, []byte(`{}`), make([]byte, maxSnapshot+1)} {
		if _, err := generator.Generate(context.Background(), invalid); err == nil {
			t.Fatal("accepted invalid snapshot")
		}
	}
}

// TestResourceIdentityAndResultsAreImmutable prevents caller mutations from
// changing future generated artifacts or their frozen resource identity.
func TestResourceIdentityAndResultsAreImmutable(t *testing.T) {
	assets := fstest.MapFS{"site/client.js": &fstest.MapFile{Data: []byte("version-one")}}
	script := []byte(`globalThis.__render = () => Promise.resolve({html:'<html></html>',robots:'robots',sitemap:'sitemap'});`)
	one, err := New(script, assets)
	if err != nil {
		t.Fatal(err)
	}
	two, _ := New(script, assets)
	if one.Identity() != two.Identity() || len(one.Identity()) != 64 {
		t.Fatal("unstable bundle identity")
	}
	assets["site/client.js"].Data = []byte("version-two")
	three, _ := New(script, assets)
	if one.Identity() == three.Identity() {
		t.Fatal("resource change did not change identity")
	}
	_, snapshot := embeddedGenerator(t)
	raw, _ := json.Marshal(snapshot)
	files, _ := one.Generate(context.Background(), raw)
	files["site/client.js"][0] = 'x'
	next, _ := one.Generate(context.Background(), raw)
	if string(next["site/client.js"]) != "version-one" {
		t.Fatal("caller mutated embedded assets")
	}
	for _, entry := range []struct {
		code []byte
		fs   fs.FS
	}{{nil, assets}, {[]byte("invalid !"), assets}, {script, fstest.MapFS{}}, {script, nil}, {script, fstest.MapFS{"link": &fstest.MapFile{Mode: fs.ModeSymlink}}}} {
		if _, err := New(entry.code, entry.fs); err == nil {
			t.Fatal("accepted invalid embedded resources")
		}
	}
}
