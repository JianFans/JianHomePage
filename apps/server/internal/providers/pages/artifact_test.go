package pages

import (
	"context"
	"testing"
)

// TestPrepareArchiveRejectsSingleFilesOverPlatformLimit enforces Pages' per-file upload bound.
func TestPrepareArchiveRejectsSingleFilesOverPlatformLimit(t *testing.T) {
	files := map[string][]byte{"index.html": make([]byte, (25<<20)+1)}
	if _, err := prepareArchive(context.Background(), files); err == nil {
		t.Fatal("Pages archive accepted a file larger than the platform limit")
	}
}
