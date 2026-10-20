package local

import (
	"context"
	"errors"
	"io"
	"testing"

	"yujian.me/server/internal/domain"
)

// TestSnapshotStoreCopiesBytesAndRejectsReplacement protects local snapshot immutability.
func TestSnapshotStoreCopiesBytesAndRejectsReplacement(t *testing.T) {
	store := NewSnapshotStore()
	input := []byte("canonical snapshot")
	if err := store.PutSnapshot(context.Background(), "snapshots/rel_test/key.json", input); err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'
	if err := store.PutSnapshot(context.Background(), "snapshots/rel_test/key.json", []byte("canonical snapshot")); err != nil {
		t.Fatalf("same immutable content was not idempotent: %v", err)
	}
	if err := store.PutSnapshot(context.Background(), "snapshots/rel_test/key.json", []byte("replacement")); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected replacement conflict, got %v", err)
	}
	reader, err := store.ReadSnapshot(context.Background(), "snapshots/rel_test/key.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil || string(got) != "canonical snapshot" {
		t.Fatalf("stored bytes were not isolated: %q %v %v", got, err, closeErr)
	}
}
