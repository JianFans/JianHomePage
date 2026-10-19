package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReadSnapshotUsesAuthenticatedObjectRead keeps private snapshot reads on
// signed object requests and rejects keys outside the snapshots namespace.
func TestReadSnapshotUsesAuthenticatedObjectRead(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/media/snapshots/rel_b73feb78-c751-4ccb-be54-8a8039247ca0/digest.json" || r.Header.Get("Authorization") == "" {
			t.Errorf("unexpected object read: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"releaseId":"rel_b73feb78-c751-4ccb-be54-8a8039247ca0"}`)
	}))
	defer server.Close()
	store, err := NewBlobStore(Config{Endpoint: server.URL, PublicBaseURL: "https://media.example.test", Region: "test", Bucket: "media", AccessKeyID: "test", SecretAccessKey: "test", UsePathStyle: true, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	body, err := store.ReadSnapshot(context.Background(), "snapshots/rel_b73feb78-c751-4ccb-be54-8a8039247ca0/digest.json")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || string(data) != `{"releaseId":"rel_b73feb78-c751-4ccb-be54-8a8039247ca0"}` {
		t.Fatalf("wrong snapshot: %s %v", data, err)
	}
	for _, key := range []string{"../snapshots/private", "assets/file", "snapshots/../secret"} {
		if _, err := store.ReadSnapshot(context.Background(), key); err == nil {
			t.Fatalf("accepted unsafe snapshot %q", key)
		}
	}
	if requests != 1 {
		t.Fatalf("unsafe key reached object storage: %d requests", requests)
	}
}
