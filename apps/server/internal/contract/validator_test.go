package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"yujian.me/server/internal/snapshot"
)

func readFixture(t *testing.T) []byte {
	t.Helper()
	value, err := os.ReadFile("../../../../content/fixtures/homepage.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return value
}

func TestValidatorAcceptsFixtureAndGeneratedType(t *testing.T) {
	raw := readFixture(t)
	if err := NewValidator().Validate(raw); err != nil {
		t.Fatalf("fixture should be valid: %v", err)
	}
	var value snapshot.YujianContentSnapshot
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("generated type should decode fixture: %v", err)
	}
	if value.SchemaVersion != "1.0.0" {
		t.Fatalf("unexpected schema version %q", value.SchemaVersion)
	}
}

func TestValidatorReportsJSONPointer(t *testing.T) {
	raw := bytes.Replace(readFixture(t), []byte(`"schemaVersion": "1.0.0"`), []byte(`"schemaVersion": "2.0.0"`), 1)
	err := NewValidator().Validate(raw)
	if err == nil || !strings.Contains(err.Error(), "/schemaVersion") {
		t.Fatalf("expected schema path, got %v", err)
	}
}

func TestValidatorRejectsUnknownRootField(t *testing.T) {
	raw := append(readFixture(t), '\n')
	raw = bytes.TrimSpace(raw)
	raw[len(raw)-1] = '}'
	raw = append(raw[:len(raw)-1], []byte(`,"unexpected":true}`)...)
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/unexpected") {
		t.Fatalf("expected unknown field pointer, got %v", err)
	}
}

func TestValidatorRejectsTrailingJSON(t *testing.T) {
	raw := append(readFixture(t), []byte("\n{}")...)
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("expected multiple-value error, got %v", err)
	}
}

func TestValidatorRejectsTrailingGarbage(t *testing.T) {
	raw := append(readFixture(t), []byte("\nnot-json")...)
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("expected trailing JSON error, got %v", err)
	}
}

func TestValidatorRejectsNestedUnknownFieldWithPointer(t *testing.T) {
	raw := readFixture(t)
	raw = bytes.Replace(raw, []byte(`"canonicalUrl": "https://yujian.me"`), []byte(`"canonicalUrl": "https://yujian.me", "unexpected": true`), 1)
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/site/unexpected") {
		t.Fatalf("expected nested unknown field pointer, got %v", err)
	}
}

func TestValidatorRejectsNonHTTPSPlatformLink(t *testing.T) {
	raw := bytes.Replace(readFixture(t), []byte(`"url": "https://music.163.com/#/artist?id=12382128"`), []byte(`"url": "http://music.163.com/#/artist?id=12382128"`), 1)
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/site/socialLinks/0/url") {
		t.Fatalf("expected HTTPS URL pointer, got %v", err)
	}
}

func TestValidatorRejectsInvalidNestedSectionVariant(t *testing.T) {
	raw := bytes.Replace(readFixture(t), []byte(`"layoutVariant": "cover-reel"`), []byte(`"layoutVariant": "immersive"`), 1)
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/homepage/sections/1/layoutVariant") {
		t.Fatalf("expected section variant pointer, got %v", err)
	}
}

func TestValidatorRejectsBrokenReleaseTrackReference(t *testing.T) {
	raw := bytes.Replace(readFixture(t), []byte(`"track_01"`), []byte(`"track_missing"`), 1)
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/releases/0/trackIds/0") {
		t.Fatalf("expected broken track reference pointer, got %v", err)
	}
}

func TestValidatorRejectsBrokenAssetReference(t *testing.T) {
	raw := bytes.Replace(readFixture(t), []byte(`"ogAssetId": "asset_hero_studio"`), []byte(`"ogAssetId": "asset_missing"`), 1)
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/site/seo/ogAssetId") {
		t.Fatalf("expected broken asset reference pointer, got %v", err)
	}
}

func TestValidatorRejectsBrokenMomentTargetReference(t *testing.T) {
	var fixture map[string]any
	if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	moments := fixture["moments"].([]any)
	moments[0].(map[string]any)["target"] = map[string]any{"kind": "internal", "contentId": "video_missing"}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/moments/0/target/contentId") {
		t.Fatalf("expected broken moment target pointer, got %v", err)
	}
}
