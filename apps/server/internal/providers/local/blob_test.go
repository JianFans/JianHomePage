package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"yujian.me/server/internal/ports"
)

func TestUploadHandlerAcceptsReservedUploadAndPersistsMetadata(t *testing.T) {
	store := newTestBlobStore(t)
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

func TestUploadHandlerStreamsPayloadWithBoundedReads(t *testing.T) {
	store := newTestBlobStore(t)
	payload := bytes.Repeat([]byte("streamed-media"), 128*1024)
	checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
	upload, err := store.CreateUpload(t.Context(), ports.UploadRequest{
		BlobKey: "assets/asset_stream/source.mp4", ContentType: "video/mp4",
		Size: int64(len(payload)), Checksum: checksum, ExpiresIn: time.Minute,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	body := &boundedReadReader{reader: bytes.NewReader(payload), maxRead: 64 * 1024}
	request := httptest.NewRequest(http.MethodPut, upload.URL, body)
	request.ContentLength = int64(len(payload))
	for key, value := range upload.Headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()

	store.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected streamed upload to return 204, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if body.largestRead > body.maxRead {
		t.Fatalf("upload requested %d-byte read, limit is %d", body.largestRead, body.maxRead)
	}
}

func TestBlobStorePublishesUploadedObjectAtStableLocalURL(t *testing.T) {
	store := newTestBlobStore(t)
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

func TestBlobStoreCloseRemovesLocalObjects(t *testing.T) {
	store := NewBlobStore()
	payload := []byte("temporary-data")
	key := "files/temporary.txt"
	if err := store.Put(t.Context(), key, bytes.NewReader(payload), ports.BlobMetadata{
		ContentType: "text/plain",
		Size:        int64(len(payload)),
		Checksum:    fmt.Sprintf("sha256:%x", sha256.Sum256(payload)),
	}); err != nil {
		t.Fatalf("put local object: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close local store: %v", err)
	}
	if _, err := store.Stat(t.Context(), key); err == nil {
		t.Fatal("closed local store retained object metadata")
	}
}

// TestBlobStoreCloseWaitsForConcurrentRootCreationAndRemovesIt verifies that
// shutdown cannot delete the temporary root underneath an active writer.
func TestBlobStoreCloseWaitsForConcurrentRootCreationAndRemovesIt(t *testing.T) {
	store := NewBlobStore()
	started := make(chan struct{})
	release := make(chan struct{})
	store.createRoot = func() (string, error) {
		close(started)
		<-release
		return os.MkdirTemp("", "yujian-local-blobs-race-*")
	}

	putDone := make(chan error, 1)
	go func() {
		putDone <- store.Put(t.Context(), "files/race.txt", bytes.NewBufferString("data"), ports.BlobMetadata{
			ContentType: "text/plain",
			Size:        4,
			Checksum:    checksumFor([]byte("data")),
		})
	}()
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- store.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("close returned during root creation: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)

	if err := <-putDone; !errors.Is(err, errBlobStoreClosed) {
		t.Fatalf("expected concurrent put to observe closed store, got %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("close local store: %v", err)
	}
	if store.rootPath == "" {
		t.Fatal("test did not create a temporary root")
	}
	if _, err := os.Stat(store.rootPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary root still exists after close: %v", err)
	}
}

func TestBlobStorePutHonorsCanceledContext(t *testing.T) {
	store := newTestBlobStore(t)
	payload := []byte("canceled-data")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := store.Put(ctx, "files/canceled.txt", bytes.NewReader(payload), ports.BlobMetadata{
		ContentType: "text/plain",
		Size:        int64(len(payload)),
		Checksum:    fmt.Sprintf("sha256:%x", sha256.Sum256(payload)),
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled put, got %v", err)
	}
}

func TestBlobStorePutWithoutChecksumRemainsIdempotent(t *testing.T) {
	store := newTestBlobStore(t)
	metadata := ports.BlobMetadata{ContentType: "text/plain", Size: 4}
	if err := store.Put(t.Context(), "files/data.txt", bytes.NewBufferString("data"), metadata); err != nil {
		t.Fatalf("put object without checksum: %v", err)
	}
	if err := store.Put(t.Context(), "files/data.txt", bytes.NewBufferString("data"), metadata); err != nil {
		t.Fatalf("repeat idempotent put without checksum: %v", err)
	}
}

func TestUploadHandlerRejectsUnknownLengthPayloadTooLarge(t *testing.T) {
	store := newTestBlobStore(t)
	upload, err := store.CreateUpload(t.Context(), ports.UploadRequest{
		BlobKey: "assets/asset_1/source.webp", ContentType: "image/webp", Size: 4,
		Checksum: "sha256:" + fmt.Sprintf("%x", sha256.Sum256([]byte("data"))), ExpiresIn: time.Minute,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	request := httptest.NewRequest(http.MethodPut, upload.URL, bytes.NewBufferString("too-large"))
	request.ContentLength = -1
	request.Header.Set("Content-Type", upload.Headers["Content-Type"])
	request.Header.Set("X-Yujian-Checksum", upload.Headers["X-Yujian-Checksum"])
	recorder := httptest.NewRecorder()

	store.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", recorder.Code, recorder.Body.String())
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
			store := newTestBlobStore(t)
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

func newTestBlobStore(t *testing.T) *BlobStore {
	t.Helper()
	store := NewBlobStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close local blob store: %v", err)
		}
	})
	return store
}

type boundedReadReader struct {
	reader      io.Reader
	maxRead     int
	largestRead int
}

func (reader *boundedReadReader) Read(buffer []byte) (int, error) {
	if len(buffer) > reader.largestRead {
		reader.largestRead = len(buffer)
	}
	if len(buffer) > reader.maxRead {
		return 0, errors.New("read buffer exceeded streaming limit")
	}
	return reader.reader.Read(buffer)
}
