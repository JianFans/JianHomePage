// Code generated from packages/schema/schema/content-snapshot.schema.json.
// DO NOT EDIT BY HAND. Run the schema sync/generation command instead.
package snapshot

import "encoding/json"

// YujianContentSnapshot is the stable envelope exchanged by the public site,
// management console and content service. Nested records stay as JSON values
// here so the generated envelope remains forward-compatible until the pinned
// Go schema generator is available in the build environment.
type YujianContentSnapshot struct {
	SchemaVersion string            `json:"schemaVersion"`
	ReleaseID     string            `json:"releaseId"`
	GeneratedAt   string            `json:"generatedAt"`
	Site          json.RawMessage   `json:"site"`
	Homepage      json.RawMessage   `json:"homepage"`
	HeroSlides    []json.RawMessage `json:"heroSlides"`
	Releases      []json.RawMessage `json:"releases"`
	Tracks        []json.RawMessage `json:"tracks"`
	Videos        []json.RawMessage `json:"videos"`
	Events        []json.RawMessage `json:"events"`
	Moments       []json.RawMessage `json:"moments"`
	Artist        json.RawMessage   `json:"artist"`
	Assets        []json.RawMessage `json:"assets"`
}
