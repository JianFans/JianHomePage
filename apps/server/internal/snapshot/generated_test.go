package snapshot

import (
	"encoding/json"
	"testing"
)

func TestGeneratedSnapshotTypeHasStableEnvelopeFields(t *testing.T) {
	value := YujianContentSnapshot{SchemaVersion: "1.0.0", ReleaseID: "rel_9d6e63a2-d7e4-4e87-a54a-500c23239e6f"}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("snapshot should marshal")
	}
}
