// Package adminweb combines embedded management pages with the existing API.
package adminweb

import (
	"bytes"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

var hashedAsset = regexp.MustCompile(`(?:^|[.\-])[A-Za-z0-9_-]{8,}\.(?:js|css|woff2?|ttf|png|webp|svg|avif)$`)

// NewHandler serves built pages and delegates API, health, and local storage routes.
func NewHandler(adminFS fs.FS, api http.Handler) (http.Handler, error) {
	if adminFS == nil || api == nil {
		return nil, fmt.Errorf("management files and API handler are required")
	}
	index, err := fs.ReadFile(adminFS, "index.html")
	if err != nil || len(index) == 0 {
		return nil, fmt.Errorf("management index.html is missing or empty")
	}
	preview := "preview.html"
	data, err := fs.ReadFile(adminFS, preview)
	if err != nil || len(data) == 0 {
		preview = "preview/index.html"
		data, err = fs.ReadFile(adminFS, preview)
		if err != nil || len(data) == 0 {
			return nil, fmt.Errorf("management preview page is missing or empty")
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPath := r.URL.Path
		if urlPath == "/api" || strings.HasPrefix(urlPath, "/api/") || urlPath == "/healthz" || strings.HasPrefix(urlPath, "/local-upload/") || strings.HasPrefix(urlPath, "/media/") {
			api.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(urlPath, "/")
		switch urlPath {
		case "/":
			name = "index.html"
		case "/preview", "/preview/":
			name = preview
		}
		if !publishedFile(name) {
			http.NotFound(w, r)
			return
		}
		content, err := fs.ReadFile(adminFS, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", assetMIME(name))
		cache := "no-cache"
		if strings.HasSuffix(name, ".html") {
			cache = "no-store"
		} else if strings.HasPrefix(name, "_nuxt/") && hashedAsset.MatchString(path.Base(name)) {
			cache = "public, max-age=31536000, immutable"
		}
		w.Header().Set("Cache-Control", cache)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
	}), nil
}

// publishedFile restricts requests to browser assets and rejects unsafe path forms.
func publishedFile(name string) bool {
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00") {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if strings.HasPrefix(segment, ".") {
			return false
		}
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".js", ".css", ".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".ico", ".woff", ".woff2", ".ttf", ".otf", ".mp3", ".mp4", ".webm", ".ogg", ".wav":
		return true
	case ".json":
		return strings.HasPrefix(name, "_nuxt/")
	default:
		return false
	}
}

// assetMIME fixes script types across host platforms and identifies other assets.
func assetMIME(name string) string {
	if strings.HasSuffix(name, ".js") {
		return "text/javascript; charset=utf-8"
	}
	if value := mime.TypeByExtension(path.Ext(name)); value != "" {
		return value
	}
	return "application/octet-stream"
}
