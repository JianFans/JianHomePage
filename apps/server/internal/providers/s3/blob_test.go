package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

func TestBlobStoreSupportsSignedAndServerSideObjectOperations(t *testing.T) {
	var putBody string
	var putChecksum string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, "/media/") {
			t.Fatalf("unexpected object path %q", request.URL.Path)
		}
		switch request.Method {
		case http.MethodPut:
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read put body: %v", err)
			}
			putBody = string(body)
			putChecksum = request.Header.Get("X-Amz-Meta-Checksum")
			writer.WriteHeader(http.StatusOK)
		case http.MethodHead:
			if strings.Contains(request.URL.Path, "missing") {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.Header().Set("Content-Length", "7")
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("X-Amz-Meta-Checksum", "sha256:test")
			writer.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", request.Method)
		}
	}))
	defer server.Close()

	store, err := NewBlobStore(Config{
		Endpoint:        server.URL,
		Region:          "ap-singapore",
		Bucket:          "media",
		AccessKeyID:     "test-access",
		SecretAccessKey: "test-secret",
		UsePathStyle:    true,
		HTTPClient:      server.Client(),
	})
	if err != nil {
		t.Fatalf("new blob store: %v", err)
	}

	upload, err := store.CreateUpload(context.Background(), ports.UploadRequest{
		BlobKey: "assets/cover.webp", ContentType: "image/webp", Size: 7,
		Checksum: "sha256:test", ExpiresIn: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	parsedUpload, err := url.Parse(upload.URL)
	if err != nil || parsedUpload.Query().Get("X-Amz-Signature") == "" || !strings.Contains(parsedUpload.Path, "/media/assets/cover.webp") {
		t.Fatalf("unexpected signed upload URL %q err=%v", upload.URL, err)
	}
	if upload.Headers["Content-Type"] != "image/webp" || upload.Headers["X-Amz-Meta-Checksum"] != "sha256:test" {
		t.Fatalf("unexpected signed upload headers %#v", upload.Headers)
	}

	metadata := ports.BlobMetadata{ContentType: "application/json", Size: 7, Checksum: "sha256:test"}
	if err := store.Put(context.Background(), "snapshots/release.json", bytes.NewReader([]byte("payload")), metadata); err != nil {
		t.Fatalf("put object: %v", err)
	}
	if putBody != "payload" || putChecksum != "sha256:test" {
		t.Fatalf("unexpected put body=%q checksum=%q", putBody, putChecksum)
	}

	actual, err := store.Stat(context.Background(), "snapshots/release.json")
	if err != nil {
		t.Fatalf("stat object: %v", err)
	}
	if actual.Size != 7 || actual.ContentType != "application/json" || actual.Checksum != "sha256:test" {
		t.Fatalf("unexpected metadata %#v", actual)
	}
	if err := store.Delete(context.Background(), "snapshots/release.json"); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	readURL, err := store.SignedReadURL(context.Background(), "snapshots/release.json", 5*time.Minute)
	if err != nil || !strings.Contains(readURL, "X-Amz-Signature=") {
		t.Fatalf("unexpected read URL %q err=%v", readURL, err)
	}
}

func TestBlobStoreMapsMissingObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	store, err := NewBlobStore(Config{
		Endpoint: server.URL, Region: "test", Bucket: "media", AccessKeyID: "access",
		SecretAccessKey: "secret", UsePathStyle: true, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("new blob store: %v", err)
	}
	if _, err := store.Stat(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestNewBlobStoreRejectsIncompleteConfiguration(t *testing.T) {
	if _, err := NewBlobStore(Config{}); err == nil {
		t.Fatal("expected incomplete configuration error")
	}
	if _, err := NewBlobStore(Config{
		Endpoint: "http://storage.example.test", Region: "test", Bucket: "media",
		AccessKeyID: "access", SecretAccessKey: "secret", RequireHTTPS: true,
	}); err == nil {
		t.Fatal("expected HTTPS validation error")
	}
}
