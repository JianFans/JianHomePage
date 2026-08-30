package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

func TestBlobStoreSupportsSignedAndServerSideObjectOperations(t *testing.T) {
	digest := sha256.Sum256([]byte("payload"))
	checksum := fmt.Sprintf("sha256:%x", digest)
	providerChecksum := base64.StdEncoding.EncodeToString(digest[:])
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
			putChecksum = request.Header.Get("X-Amz-Checksum-Sha256")
			writer.WriteHeader(http.StatusOK)
		case http.MethodHead:
			if strings.Contains(request.URL.Path, "missing") {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.Header().Set("Content-Length", "7")
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("X-Amz-Meta-Checksum", "sha256:untrusted")
			writer.Header().Set("X-Amz-Checksum-Sha256", providerChecksum)
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
		PublicBaseURL:   "https://media.example.test/base/",
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
	publicURL, err := store.PublicURL(context.Background(), "assets/cover.webp")
	if err != nil || publicURL != "https://media.example.test/base/assets/cover.webp" {
		t.Fatalf("unexpected public URL %q err=%v", publicURL, err)
	}

	upload, err := store.CreateUpload(context.Background(), ports.UploadRequest{
		BlobKey: "assets/cover.webp", ContentType: "image/webp", Size: 7,
		Checksum: checksum, ExpiresIn: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	parsedUpload, err := url.Parse(upload.URL)
	if err != nil || parsedUpload.Query().Get("X-Amz-Signature") == "" || !strings.Contains(parsedUpload.Path, "/media/assets/cover.webp") {
		t.Fatalf("unexpected signed upload URL %q err=%v", upload.URL, err)
	}
	if upload.Headers["Content-Type"] != "image/webp" || upload.Headers["X-Amz-Checksum-Sha256"] != providerChecksum {
		t.Fatalf("unexpected signed upload headers %#v", upload.Headers)
	}

	metadata := ports.BlobMetadata{ContentType: "application/json", Size: 7, Checksum: checksum}
	if err := store.Put(context.Background(), "snapshots/release.json", bytes.NewReader([]byte("payload")), metadata); err != nil {
		t.Fatalf("put object: %v", err)
	}
	if putBody != "payload" || putChecksum != providerChecksum {
		t.Fatalf("unexpected put body=%q checksum=%q", putBody, putChecksum)
	}

	actual, err := store.Stat(context.Background(), "snapshots/release.json")
	if err != nil {
		t.Fatalf("stat object: %v", err)
	}
	if actual.Size != 7 || actual.ContentType != "application/json" || actual.Checksum != checksum {
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

func TestS3HTTPClientUsesBoundedTimeoutInProduction(t *testing.T) {
	client := s3HTTPClient(nil, true)
	if client == nil || client.Timeout <= 0 {
		t.Fatalf("production S3 client has no total timeout: %#v", client)
	}
}

func TestBlobStoreMapsMissingObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	store, err := NewBlobStore(Config{
		Endpoint: server.URL, Region: "test", Bucket: "media", AccessKeyID: "access",
		SecretAccessKey: "secret", PublicBaseURL: "https://media.example.test",
		UsePathStyle: true, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("new blob store: %v", err)
	}
	if _, err := store.Stat(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestBlobStoreRejectsHTTPSDowngradeRedirectBeforeSendingSignedRequest(t *testing.T) {
	var insecureCalls atomic.Int32
	insecure := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		insecureCalls.Add(1)
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer insecure.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, insecure.URL, http.StatusTemporaryRedirect)
	}))
	defer secure.Close()
	store, err := NewBlobStore(Config{
		Endpoint: secure.URL, Region: "test", Bucket: "media", AccessKeyID: "access",
		SecretAccessKey: "secret", PublicBaseURL: "https://media.example.test",
		UsePathStyle: true, HTTPClient: secure.Client(), RequireHTTPS: true,
	})
	if err != nil {
		t.Fatalf("new blob store: %v", err)
	}

	if _, err := store.Stat(context.Background(), "assets/cover.webp"); err == nil {
		t.Fatal("expected HTTPS downgrade rejection")
	}
	if insecureCalls.Load() != 0 {
		t.Fatalf("insecure redirect target received %d requests", insecureCalls.Load())
	}
}

func TestNewBlobStoreRejectsIncompleteConfiguration(t *testing.T) {
	if _, err := NewBlobStore(Config{}); err == nil {
		t.Fatal("expected incomplete configuration error")
	}
	if _, err := NewBlobStore(Config{
		Endpoint: "http://storage.example.test", Region: "test", Bucket: "media",
		AccessKeyID: "access", SecretAccessKey: "secret",
		PublicBaseURL: "https://media.example.test", RequireHTTPS: true,
	}); err == nil {
		t.Fatal("expected HTTPS validation error")
	}
}

func TestNewBlobStoreRejectsEndpointWithoutHostnameOrValidPort(t *testing.T) {
	for _, endpoint := range []string{"https://:443", "https://storage.example.test:65536"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := NewBlobStore(Config{
				Endpoint: endpoint, Region: "test", Bucket: "media", AccessKeyID: "access",
				SecretAccessKey: "secret", PublicBaseURL: "https://media.example.test", RequireHTTPS: true,
			})
			if err == nil {
				t.Fatalf("expected invalid endpoint rejection for %q", endpoint)
			}
		})
	}
}

func TestNewBlobStoreRejectsInvalidPublicBaseURL(t *testing.T) {
	for _, publicBaseURL := range []string{
		"",
		"/media",
		"https://:443",
		"https://media.example.test:65536",
		"https://user:pass@media.example.test",
		"https://media.example.test/base?token=secret",
		"https://media.example.test/base#fragment",
		"http://media.example.test",
	} {
		t.Run(publicBaseURL, func(t *testing.T) {
			_, err := NewBlobStore(Config{
				Endpoint: "https://storage.example.test", Region: "test", Bucket: "media",
				AccessKeyID: "access", SecretAccessKey: "secret",
				PublicBaseURL: publicBaseURL, RequireHTTPS: true,
			})
			if err == nil {
				t.Fatalf("expected invalid public base URL rejection for %q", publicBaseURL)
			}
		})
	}
}

func TestBlobStoreRejectsParentDirectoryObjectKeys(t *testing.T) {
	for _, key := range []string{"..", "../escape", "assets/../escape"} {
		if err := validateKey(key); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected invalid key %q, got %v", key, err)
		}
	}
}
