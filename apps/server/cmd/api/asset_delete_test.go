package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"yujian.me/server/internal/assets"
	"yujian.me/server/internal/config"
	"yujian.me/server/internal/domain"
)

// TestDeletePendingAssetDuringDownload keeps the media file open while deleting
// through the API, then checks new reads fail and the existing download finishes.
func TestDeletePendingAssetDuringDownload(t *testing.T) {
	dependencies := developmentDependencies()
	t.Cleanup(func() { _ = dependencies.Close() })
	handler, err := buildHandler(config.Config{Environment: "development", AllowDevIdentity: true}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("pending-media")
	created, err := dependencies.Assets.CreateUpload(t.Context(), domain.Principal{
		Subject: "editor-1", Roles: []domain.Role{domain.RoleEditor},
	}, assets.CreateUploadInput{
		FileName: "cover.webp", ContentType: "image/webp", Size: int64(len(payload)),
		Checksum: fmt.Sprintf("sha256:%x", sha256.Sum256(payload)), Rights: json.RawMessage(`{"source":{"zh-CN":"test"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	upload := httptest.NewRequest(http.MethodPut, created.Upload.URL, bytes.NewReader(payload))
	for key, value := range created.Upload.Headers {
		upload.Header.Set(key, value)
	}
	uploadResponse := httptest.NewRecorder()
	handler.ServeHTTP(uploadResponse, upload)
	if uploadResponse.Code != http.StatusNoContent {
		t.Fatalf("upload: status=%d body=%s", uploadResponse.Code, uploadResponse.Body.String())
	}

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	reader := &pausedMediaWriter{ResponseRecorder: httptest.NewRecorder(), started: started, release: release}
	resume := sync.OnceFunc(func() { close(release) })
	go func() {
		defer close(done)
		handler.ServeHTTP(reader, httptest.NewRequest(http.MethodGet, created.Asset.SourceURL, nil))
	}()
	t.Cleanup(func() {
		resume()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("download did not finish")
		}
	})
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not start")
	}

	deletion := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/"+created.Asset.ID, nil)
	devHeaders(request, "admin-1", "admin")
	handler.ServeHTTP(deletion, request)
	if deletion.Code != http.StatusNoContent {
		t.Fatalf("delete during download: status=%d body=%s", deletion.Code, deletion.Body.String())
	}
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, created.Asset.SourceURL, nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted media is still readable: status=%d", missing.Code)
	}
	page, err := dependencies.Assets.List(t.Context(), domain.Principal{
		Subject: "editor-1", Roles: []domain.Role{domain.RoleEditor},
	}, assets.ListOptions{Status: domain.AssetDeleted})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != created.Asset.ID {
		t.Fatalf("deleted asset not persisted: page=%#v err=%v", page, err)
	}
	resume()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not finish after deletion")
	}
	if reader.Code != http.StatusOK || !bytes.Equal(reader.Body.Bytes(), payload) {
		t.Fatalf("active download failed: status=%d body=%s", reader.Code, reader.Body.String())
	}
}

// pausedMediaWriter stops the response after the handler has opened its file.
type pausedMediaWriter struct {
	*httptest.ResponseRecorder
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

// Write exposes the open-file interval without relying on download timing.
func (writer *pausedMediaWriter) Write(data []byte) (int, error) {
	writer.once.Do(func() {
		close(writer.started)
		<-writer.release
	})
	return writer.ResponseRecorder.Write(data)
}
