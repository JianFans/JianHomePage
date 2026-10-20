package s3

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
)

// TestSignedReadURLIsRestrictedToMediaKeys prevents snapshot reads from using COS.
func TestSignedReadURLIsRestrictedToMediaKeys(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()
	store, err := NewBlobStore(Config{
		Endpoint: server.URL, PublicBaseURL: "https://media.example.test", Region: "test",
		Bucket: "media", AccessKeyID: "test", SecretAccessKey: "test", UsePathStyle: true,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SignedReadURL(context.Background(), "snapshots/rel_b73feb78-c751-4ccb-be54-8a8039247ca0/sha256.json", time.Minute); err == nil {
		t.Fatal("signed a publish snapshot key")
	} else if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("unexpected non-media-key error: %v", err)
	}
	if requests != 0 {
		t.Fatalf("invalid media key reached the provider: %d requests", requests)
	}
}
