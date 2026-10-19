package postgres

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"

	"yujian.me/server/internal/domain"
	snapshotdata "yujian.me/server/internal/snapshot"
)

// TestSnapshotStoreValidatesKeyAndCanonicalContent rejects mismatched stored identities.
func TestSnapshotStoreValidatesKeyAndCanonicalContent(t *testing.T) {
	raw, err := snapshotdata.CanonicalJSON([]byte(`{"schemaVersion":"1.0.0","releaseId":"rel_test"}`))
	if err != nil {
		t.Fatal(err)
	}
	checksum := snapshotdata.Checksum(raw)
	key := "snapshots/rel_test/" + checksum + ".json"
	if _, _, err := validateStoredSnapshot(key, raw); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
	for _, test := range []struct {
		name, key string
		content   []byte
	}{
		{"media key", "assets/asset_1/source.webp", raw},
		{"different checksum", "snapshots/rel_test/sha256:" + strings.Repeat("0", 64) + ".json", raw},
		{"noncanonical JSON", "snapshots/rel_test/" + snapshotdata.Checksum([]byte(`{"releaseId":"rel_test", "schemaVersion":"1.0.0"}`)) + ".json", []byte(`{"releaseId":"rel_test", "schemaVersion":"1.0.0"}`)},
		{"wrong release", "snapshots/rel_other/" + checksum + ".json", raw},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := validateStoredSnapshot(test.key, test.content); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected invalid snapshot, got %v", err)
			}
		})
	}
}

// TestSnapshotStorePersistsAndReadsCanonicalBytes covers immutable writes and verified reads.
func TestSnapshotStorePersistsAndReadsCanonicalBytes(t *testing.T) {
	raw, err := snapshotdata.CanonicalJSON([]byte(`{"schemaVersion":"1.0.0","releaseId":"rel_test"}`))
	if err != nil {
		t.Fatal(err)
	}
	checksum := snapshotdata.Checksum(raw)
	key := "snapshots/rel_test/" + checksum + ".json"
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	store := NewSnapshotStore(executor)
	if err := store.PutSnapshot(context.Background(), key, raw); err != nil {
		t.Fatalf("put snapshot: %v", err)
	}
	if len(executor.execArgs) != 1 || executor.execArgs[0][0] != key || executor.execArgs[0][1] != "rel_test" || executor.execArgs[0][2] != checksum {
		t.Fatalf("unexpected persisted snapshot metadata %#v", executor.execArgs)
	}

	executor.row = recordingRow{values: []any{"rel_test", checksum, raw}}
	executor.result = recordingResult{affected: 0}
	if err := store.PutSnapshot(context.Background(), key, raw); err != nil {
		t.Fatalf("idempotent put snapshot: %v", err)
	}
	reader, err := store.ReadSnapshot(context.Background(), key)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	got, err := io.ReadAll(reader)
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil || string(got) != string(raw) {
		t.Fatalf("unexpected snapshot bytes %s: %v", got, err)
	}
	if err := store.PutSnapshot(context.Background(), key, []byte("replacement")); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("replacement with a mismatched digest should be rejected: %v", err)
	}

	executor.row = recordingRow{err: sql.ErrNoRows}
	if _, err := store.ReadSnapshot(context.Background(), "snapshots/rel_test/"+checksum+".json"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

// TestPostgresSnapshotStoreRestartRoundTrip covers the durable adapter using a
// real isolated schema when PostgreSQL integration tests are enabled.
func TestPostgresSnapshotStoreRestartRoundTrip(t *testing.T) {
	database := assetIntegrationDatabase(t)
	ctx := t.Context()
	raw, err := snapshotdata.CanonicalJSON([]byte(`{"schemaVersion":"1.0.0","releaseId":"rel_store_test"}`))
	if err != nil {
		t.Fatal(err)
	}
	checksum := snapshotdata.Checksum(raw)
	key := "snapshots/rel_store_test/" + checksum + ".json"
	first := NewSnapshotStore(database)
	if err := first.PutSnapshot(ctx, key, raw); err != nil {
		t.Fatal(err)
	}
	if err := first.PutSnapshot(ctx, key, raw); err != nil {
		t.Fatalf("idempotent snapshot write: %v", err)
	}

	reader, err := NewSnapshotStore(database).ReadSnapshot(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil || string(got) != string(raw) {
		t.Fatalf("persisted snapshot changed after reopening adapter: %s %v %v", got, err, closeErr)
	}
}
