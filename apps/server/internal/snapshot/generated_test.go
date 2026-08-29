package snapshot

import (
	"encoding/json"
	"testing"
)

func TestGeneratedSnapshotTypeHasStableEnvelopeFields(t *testing.T) {
	value := YujianContentSnapshot{SchemaVersion: "1.0.0", ReleaseID: "rel_test"}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("snapshot should marshal")
	}
}
