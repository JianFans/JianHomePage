package local

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yujian.me/server/internal/ports"
)

func TestUploadHandlerAcceptsReservedUploadAndPersistsMetadata(t *testing.T) {
	store := NewBlobStore()
	payload := []byte("image-data")
	checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
	upload, err := store.CreateUpload(t.Context(), ports.UploadRequest{
		BlobKey: "assets/asset_1/source.webp", ContentType: "image/webp",
		Size: int64(len(payload)), Checksum: checksum, ExpiresIn: time.Minute,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	request := httptest.NewRequest(http.MethodPut, upload.URL, bytes.NewReader(payload))
	for key, value := range upload.Headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()

	store.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	metadata, err := store.Stat(t.Context(), "assets/asset_1/source.webp")
	if err != nil {
		t.Fatalf("stat uploaded object: %v", err)
	}
	if metadata.ContentType != "image/webp" || metadata.Size != int64(len(payload)) || metadata.Checksum != checksum {
		t.Fatalf("unexpected metadata %#v", metadata)
	}
}

func TestBlobStorePublishesUploadedObjectAtStableLocalURL(t *testing.T) {
	store := NewBlobStore()
	payload := []byte("audio-data")
	key := "assets/asset_1/source.mp3"
	if err := store.Put(t.Context(), key, bytes.NewReader(payload), ports.BlobMetadata{
		ContentType: "audio/mpeg",
		Size:        int64(len(payload)),
		Checksum:    fmt.Sprintf("sha256:%x", sha256.Sum256(payload)),
	}); err != nil {
		t.Fatalf("put local object: %v", err)
	}

	publicURL, err := store.PublicURL(t.Context(), key)
	if err != nil {
		t.Fatalf("public URL: %v", err)
	}
	if publicURL != "/media/assets/asset_1/source.mp3" {
		t.Fatalf("unexpected public URL %q", publicURL)
	}

	request := httptest.NewRequest(http.MethodGet, publicURL, nil)
	recorder := httptest.NewRecorder()
	store.ServeHTTP(recorder, request)

	response := recorder.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "audio/mpeg" || !bytes.Equal(body, payload) {
		t.Fatalf("unexpected local read response status=%d type=%q body=%q", response.StatusCode, response.Header.Get("Content-Type"), body)
	}
}

func TestUploadHandlerRejectsInvalidPayloads(t *testing.T) {
	for _, test := range []struct {
		name        string
		contentType string
		body        []byte
		status      int
	}{
		{name: "mime", contentType: "image/jpeg", body: []byte("data"), status: http.StatusBadRequest},
		{name: "size", contentType: "image/webp", body: []byte("too-large"), status: http.StatusRequestEntityTooLarge},
		{name: "checksum", contentType: "image/webp", body: []byte("xxxx"), status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewBlobStore()
			upload, err := store.CreateUpload(t.Context(), ports.UploadRequest{
				BlobKey: "assets/asset_1/source.webp", ContentType: "image/webp", Size: 4,
				Checksum: "sha256:" + fmt.Sprintf("%x", sha256.Sum256([]byte("data"))), ExpiresIn: time.Minute,
			})
			if err != nil {
				t.Fatalf("create upload: %v", err)
			}
			request := httptest.NewRequest(http.MethodPut, upload.URL, bytes.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			request.Header.Set("X-Yujian-Checksum", upload.Headers["X-Yujian-Checksum"])
			recorder := httptest.NewRecorder()
			store.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("expected %d, got %d", test.status, recorder.Code)
			}
		})
	}
}
