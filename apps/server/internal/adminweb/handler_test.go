package adminweb

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// adminFiles supplies real file content and intentionally private build artifacts.
func adminFiles() fstest.MapFS {
	return fstest.MapFS{
		"index.html":            {Data: []byte("<html>management</html>")},
		"preview/index.html":    {Data: []byte("<html>preview</html>")},
		"_nuxt/B8a6Vsf2.js":     {Data: []byte("export const app = true;")},
		"_nuxt/B8a6Vsf2.css":    {Data: []byte("body{color:white}")},
		"_nuxt/B8a6Vsf2.js.map": {Data: []byte("private source")},
		"_nuxt/.env":            {Data: []byte("private environment")},
		"config.yaml":           {Data: []byte("private config")},
		"server.ts":             {Data: []byte("private source")},
		"logo.svg":              {Data: []byte("<svg></svg>")},
	}
}

// TestHandlerServesOnlyPublishedFiles verifies routing without SPA fallback.
func TestHandlerServesOnlyPublishedFiles(t *testing.T) {
	handler, err := NewHandler(adminFiles(), http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, body, mime, cache string
		status                  int
	}{
		{"/", "management", "text/html", "no-store", 200},
		{"/index.html", "management", "text/html", "no-store", 200},
		{"/preview", "preview", "text/html", "no-store", 200},
		{"/preview/", "preview", "text/html", "no-store", 200},
		{"/_nuxt/B8a6Vsf2.js", "export", "javascript", "public, max-age=31536000, immutable", 200},
		{"/_nuxt/B8a6Vsf2.css", "color:white", "text/css", "public, max-age=31536000, immutable", 200},
		{"/logo.svg", "<svg", "image/svg+xml", "no-cache", 200},
		{"/missing", "", "", "", 404},
		{"/_nuxt", "", "", "", 404},
		{"/_nuxt/", "", "", "", 404},
		{"/_nuxt/missing.js", "", "", "", 404},
		{"/_nuxt/B8a6Vsf2.js.map", "", "", "", 404},
		{"/_nuxt/.env", "", "", "", 404},
		{"/config.yaml", "", "", "", 404},
		{"/server.ts", "", "", "", 404},
		{"/../index.html", "", "", "", 404},
		{"/%2e%2e/index.html", "", "", "", 404},
		{"/_nuxt/../index.html", "", "", "", 404},
		{"//index.html", "", "", "", 404},
		{"/preview%5c..%5cindex.html", "", "", "", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
			}
			if tc.status != 200 {
				if strings.Contains(response.Body.String(), "management") {
					t.Fatal("unexpected SPA fallback")
				}
				return
			}
			if !strings.Contains(response.Body.String(), tc.body) {
				t.Errorf("body = %s", response.Body.String())
			}
			if !strings.Contains(response.Header().Get("Content-Type"), tc.mime) {
				t.Errorf("MIME = %s", response.Header().Get("Content-Type"))
			}
			if response.Header().Get("Cache-Control") != tc.cache {
				t.Errorf("cache = %s", response.Header().Get("Cache-Control"))
			}
			if response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Error("missing nosniff")
			}
		})
	}
}

// TestHandlerPreservesAPIRoutes verifies API errors and upload methods remain intact.
func TestHandlerPreservesAPIRoutes(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"path":"` + r.URL.Path + `"}`))
	})
	handler, err := NewHandler(adminFiles(), api)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api", "/api/v1/missing", "/healthz", "/local-upload/file", "/media/image.webp"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != 404 || response.Header().Get("Content-Type") != "application/json" || !strings.Contains(response.Body.String(), path) {
			t.Fatalf("route %s changed: %d %s", path, response.Code, response.Body.String())
		}
	}
}

// TestHandlerMethods verifies HEAD content metadata and rejects static writes.
func TestHandlerMethods(t *testing.T) {
	handler, err := NewHandler(adminFiles(), http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/preview", "/_nuxt/B8a6Vsf2.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodHead, path, nil))
		if response.Code != 200 || response.Body.Len() != 0 || response.Header().Get("Content-Length") == "" {
			t.Fatalf("HEAD %s: %d %s", path, response.Code, response.Body.String())
		}
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != 405 || response.Header().Get("Allow") != "GET, HEAD" {
			t.Fatalf("POST %s: %d", path, response.Code)
		}
	}
}

// TestHandlerRequiresBuiltPages prevents silently serving an incomplete release.
func TestHandlerRequiresBuiltPages(t *testing.T) {
	for _, files := range []fs.FS{nil, fstest.MapFS{}, fstest.MapFS{"index.html": {Data: []byte("management")}}} {
		if _, err := NewHandler(files, http.NotFoundHandler()); err == nil {
			t.Fatal("accepted incomplete management pages")
		}
	}
	if _, err := NewHandler(adminFiles(), nil); err == nil {
		t.Fatal("accepted missing API")
	}
	files := adminFiles()
	delete(files, "preview/index.html")
	files["preview.html"] = &fstest.MapFile{Data: []byte("flat preview")}
	handler, err := NewHandler(files, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/preview", nil))
	if response.Code != 200 || response.Body.String() != "flat preview" {
		t.Fatal("flat preview missing")
	}
}
