package local

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"yujian.me/server/internal/ports"
)

// TestMediaRangeRead preserves seeking when opening platform-specific handles.
func TestMediaRangeRead(t *testing.T) {
	store := newTestBlobStore(t)
	payload := []byte("0123456789")
	if err := store.Put(t.Context(), "files/range.mp3", bytes.NewReader(payload), ports.BlobMetadata{
		ContentType: "audio/mpeg", Size: int64(len(payload)), Checksum: checksumFor(payload),
	}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/media/files/range.mp3", nil)
	request.Header.Set("Range", "bytes=2-5")
	response := httptest.NewRecorder()
	store.ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || response.Body.String() != "2345" || response.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("range read: status=%d body=%s headers=%v", response.Code, response.Body.String(), response.Header())
	}
}

// TestOpenMediaFileErrors keeps OS failures inspectable without leaking handles.
func TestOpenMediaFileErrors(t *testing.T) {
	for _, name := range []string{filepath.Join(t.TempDir(), "missing"), "invalid\x00path"} {
		file, err := openMediaFile(name)
		if file != nil {
			_ = file.Close()
			t.Fatalf("opened invalid path %q", name)
		}
		var pathError *os.PathError
		if !errors.As(err, &pathError) || pathError.Op != "open" || pathError.Path != name {
			t.Fatalf("expected original path error for %q, got %v", name, err)
		}
	}
}
