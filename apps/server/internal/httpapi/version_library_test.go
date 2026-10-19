package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
)

// TestVersionLibraryRoutes verifies metadata paging and named clone contracts at
// the HTTP boundary, including authorization and strict query decoding.
func TestVersionLibraryRoutes(t *testing.T) {
	now := time.Now().UTC()
	stub := &contentServiceStub{}
	stub.listFn = func(_ context.Context, actor domain.Principal, options content.ListOptions) (content.ListPage, error) {
		if actor.Subject != "editor" || options.Limit != 2 || options.Cursor != "cursor" {
			t.Fatal("query or actor not forwarded")
		}
		return content.ListPage{Items: []content.VersionSummary{{ID: "ver_8fd7f519-2fa0-438e-bea2-9c5f67f9c4fe", Name: "版本一", Status: domain.StatusDraft, Revision: 1, CreatedAt: now, UpdatedAt: now}}, NextCursor: "next"}, nil
	}
	stub.cloneFn = func(_ context.Context, actor domain.Principal, source, name string) (domain.ContentVersion, error) {
		if source != "ver_8fd7f519-2fa0-438e-bea2-9c5f67f9c4fe" || name != "副本" || actor.Subject != "editor" {
			t.Fatal("clone contract not forwarded")
		}
		return domain.ContentVersion{ID: "ver_0713cc94-f651-4802-9650-20fbfac3c781", Name: name, SourceVersionID: source, Snapshot: json.RawMessage(`{}`), Revision: 1, Status: domain.StatusDraft}, nil
	}
	stub.namedCreateFn = func(_ context.Context, _ domain.Principal, name string, snapshot json.RawMessage) (domain.ContentVersion, error) {
		if name != "新版本" || string(snapshot) != "{}" {
			t.Fatal("named create contract not forwarded")
		}
		return domain.ContentVersion{ID: "ver_85ffbe4c-6a0a-40c9-93a7-4acb67a8c526", Name: name, Snapshot: snapshot, Revision: 1}, nil
	}
	middleware, _ := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	router := NewRouter(RouterOptions{Content: stub, Middleware: middleware})
	for _, test := range []struct {
		method, path, body string
		code               int
		expected           string
	}{
		{"GET", "/api/v1/versions?limit=2&cursor=cursor", "", 200, `"name":"版本一"`},
		{"POST", "/api/v1/versions/ver_8fd7f519-2fa0-438e-bea2-9c5f67f9c4fe/clone", `{"name":"副本"}`, 201, `"sourceVersionId":"ver_8fd7f519-2fa0-438e-bea2-9c5f67f9c4fe"`},
		{"POST", "/api/v1/versions", `{"name":"新版本","snapshot":{}}`, 201, `"name":"新版本"`},
		{"GET", "/api/v1/versions?limit=bad", "", 400, "invalid_request"},
		{"GET", "/api/v1/versions?limit=1&limit=2", "", 400, "invalid_request"},
		{"GET", "/api/v1/versions?cursor=a&cursor=b", "", 400, "invalid_request"},
		{"POST", "/api/v1/versions/ver_8fd7f519-2fa0-438e-bea2-9c5f67f9c4fe/clone", `{"name":"副本","snapshot":{}}`, 400, "invalid_request"},
	} {
		r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		r.Header.Set("X-Dev-Subject", "editor")
		r.Header.Set("X-Dev-Roles", "editor")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != test.code || !strings.Contains(w.Body.String(), test.expected) {
			t.Fatalf("%s %s: %d %s", test.method, test.path, w.Code, w.Body.String())
		}
		if test.method == "GET" && w.Code == 200 && strings.Contains(w.Body.String(), "snapshot") {
			t.Fatal("list includes snapshot")
		}
	}
	forbidden := httptest.NewRequest("POST", "/api/v1/versions/ver_8fd7f519-2fa0-438e-bea2-9c5f67f9c4fe/clone", strings.NewReader(`{"name":"副本"}`))
	forbidden.Header.Set("X-Dev-Subject", "reviewer")
	forbidden.Header.Set("X-Dev-Roles", "reviewer")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, forbidden)
	if w.Code != 403 {
		t.Fatal("reviewer cloned version")
	}
}

// TestUpdateVersionRejectsDisplayName prevents a successful update response from
// silently discarding metadata that only create and clone operations support.
func TestUpdateVersionRejectsDisplayName(t *testing.T) {
	called := false
	service := &contentServiceStub{updateFn: func(context.Context, domain.Principal, string, int64, json.RawMessage) (domain.ContentVersion, error) {
		called = true
		return testVersion(), nil
	}}
	router := testRouter(t, service, &publishServiceStub{})
	request := authenticatedRequest("PUT", "/api/v1/versions/ver_8fd7f519-2fa0-438e-bea2-9c5f67f9c4fe", `{"name":"新名称","snapshot":{}}`, "editor")
	request.Header.Set("If-Match", `"1"`)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "invalid_request") || called {
		t.Fatalf("unsupported rename accepted: %d called=%t %s", response.Code, called, response.Body.String())
	}
}
