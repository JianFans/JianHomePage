package local

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

func TestBuildTriggerLifecycle(t *testing.T) {
	trigger := NewBuildTrigger()
	if _, err := trigger.Status(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected missing build, got %v", err)
	}
	first, err := trigger.Trigger(t.Context(), ports.BuildRequest{ReleaseID: "rel_1"})
	if err != nil {
		t.Fatalf("trigger build: %v", err)
	}
	second, err := trigger.Trigger(t.Context(), ports.BuildRequest{ReleaseID: "rel_2"})
	if err != nil {
		t.Fatalf("trigger second build: %v", err)
	}
	if first.ID == second.ID || first.Status != domain.PublishSucceeded {
		t.Fatalf("unexpected builds first=%#v second=%#v", first, second)
	}
	if found, err := trigger.Status(t.Context(), first.ID); err != nil || found != first {
		t.Fatalf("get build: %#v err=%v", found, err)
	}
}

func TestBlobStoreLifecycleAndReadValidation(t *testing.T) {
	store := NewBlobStore()
	metadata := ports.BlobMetadata{ContentType: "text/plain", Size: 4, Checksum: checksumFor([]byte("data"))}
	if err := store.Put(t.Context(), "files/data.txt", bytes.NewBufferString("data"), metadata); err != nil {
		t.Fatalf("put object: %v", err)
	}
	if err := store.Put(t.Context(), "files/data.txt", bytes.NewBufferString("data"), metadata); err != nil {
		t.Fatalf("idempotent put: %v", err)
	}
	conflict := metadata
	conflict.Size++
	if err := store.Put(t.Context(), "files/data.txt", bytes.NewBufferString("other"), conflict); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected put conflict, got %v", err)
	}

	if _, err := store.SignedReadURL(t.Context(), "files/data.txt", 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid expiry, got %v", err)
	}
	if value, err := store.SignedReadURL(t.Context(), "files/data.txt", time.Minute); err != nil || value != "http://127.0.0.1:8080/media/files/data.txt" {
		t.Fatalf("unexpected signed read URL %q err=%v", value, err)
	}
	for _, key := range []string{"", "/absolute", `bad\key`, "../escape", "."} {
		if _, err := store.PublicURL(t.Context(), key); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected invalid key %q, got %v", key, err)
		}
	}

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/media/files/data.txt", nil),
		httptest.NewRequest(http.MethodGet, "/media/missing.txt", nil),
		httptest.NewRequest(http.MethodGet, "/other", nil),
	} {
		recorder := httptest.NewRecorder()
		store.ServeHTTP(recorder, request)
		if recorder.Code < http.StatusBadRequest {
			t.Fatalf("expected rejected request %s %s, got %d", request.Method, request.URL, recorder.Code)
		}
	}
	head := httptest.NewRecorder()
	store.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/media/files/data.txt", nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("unexpected HEAD response code=%d body=%q", head.Code, head.Body.String())
	}

	if err := store.Delete(t.Context(), "files/data.txt"); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	if err := store.Delete(t.Context(), "files/data.txt"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected missing delete, got %v", err)
	}
	if _, err := store.Stat(t.Context(), "files/data.txt"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected missing stat, got %v", err)
	}
}

func TestUploadReservationRejectsInvalidAndExpiredRequests(t *testing.T) {
	store := NewBlobStore()
	for _, request := range []ports.UploadRequest{
		{},
		{BlobKey: "key", ContentType: "text/plain", Size: 1},
	} {
		if _, err := store.CreateUpload(t.Context(), request); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected invalid upload %#v, got %v", request, err)
		}
	}

	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	upload, err := store.CreateUpload(t.Context(), ports.UploadRequest{
		BlobKey: "files/data.txt", ContentType: "text/plain", Size: 4, ExpiresIn: time.Minute,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}

	missingToken := httptest.NewRecorder()
	store.ServeHTTP(missingToken, httptest.NewRequest(http.MethodPut, "/local-upload/files/data.txt", bytes.NewBufferString("data")))
	if missingToken.Code != http.StatusNotFound {
		t.Fatalf("expected missing token rejection, got %d", missingToken.Code)
	}

	now = now.Add(time.Minute)
	expired := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, upload.URL, bytes.NewBufferString("data"))
	request.Header.Set("Content-Type", "text/plain")
	store.ServeHTTP(expired, request)
	if expired.Code != http.StatusGone {
		t.Fatalf("expected expired upload, got %d", expired.Code)
	}
}
