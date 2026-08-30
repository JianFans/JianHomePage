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

func TestValidatorRejectsHTTPSURLWithoutValidHost(t *testing.T) {
	for _, value := range []string{
		"https://",
		"https://?q=x",
		"https:///assets/cover.webp",
		"https:// /cover.webp",
		"https://:443/path",
		"https://@/path",
		"https://%/path",
		"https://[::1/path",
		"https://example.com:0/path",
		"https://example.com:65536/path",
		"https://@example.com/path",
		"https://user@example.com/path",
		"https://user:pass@example.com/path",
	} {
		t.Run(value, func(t *testing.T) {
			var fixture map[string]any
			if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			site := fixture["site"].(map[string]any)
			socialLinks := site["socialLinks"].([]any)
			socialLinks[0].(map[string]any)["url"] = value
			raw, err := json.Marshal(fixture)
			if err != nil {
				t.Fatalf("encode fixture: %v", err)
			}
			if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/site/socialLinks/0/url") {
				t.Fatalf("expected invalid HTTPS host error, got %v", err)
			}
		})
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

func TestValidatorRejectsInternalTargetHiddenBySectionLimit(t *testing.T) {
	var fixture map[string]any
	if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	heroes := fixture["heroSlides"].([]any)
	heroes[2].(map[string]any)["target"].(map[string]any)["contentId"] = "release_02"
	sections := fixture["homepage"].(map[string]any)["sections"].([]any)
	sections[1].(map[string]any)["limit"] = float64(1)
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/heroSlides/2/target/contentId") {
		t.Fatalf("expected unreachable target error, got %v", err)
	}
}

func TestValidatorRejectsInternalTargetInDisabledSection(t *testing.T) {
	var fixture map[string]any
	if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	heroes := fixture["heroSlides"].([]any)
	heroes[2].(map[string]any)["target"].(map[string]any)["contentId"] = "artist_primary"
	sections := fixture["homepage"].(map[string]any)["sections"].([]any)
	sections[5].(map[string]any)["enabled"] = false
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/heroSlides/2/target/contentId") {
		t.Fatalf("expected unreachable target error, got %v", err)
	}
}

func TestValidatorRejectsInternalTargetOutsideHomepageComposition(t *testing.T) {
	var fixture map[string]any
	if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	sections := fixture["homepage"].(map[string]any)["sections"].([]any)
	musicSection := sections[1].(map[string]any)
	musicSection["itemIds"] = musicSection["itemIds"].([]any)[1:]
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/heroSlides/2/target/contentId") {
		t.Fatalf("expected unreachable target error, got %v", err)
	}
}

func TestValidatorRejectsInternalTargetPastDisplayWindow(t *testing.T) {
	var fixture map[string]any
	if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	heroes := fixture["heroSlides"].([]any)
	heroes[2].(map[string]any)["target"].(map[string]any)["contentId"] = "event_01"
	events := fixture["events"].([]any)
	events[0].(map[string]any)["dateTime"] = fixture["generatedAt"]
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/heroSlides/2/target/contentId") {
		t.Fatalf("expected unreachable target error, got %v", err)
	}
}

func TestValidatorRejectsNonAudioPreviewAsset(t *testing.T) {
	var fixture map[string]any
	if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	tracks := fixture["tracks"].([]any)
	track := tracks[0].(map[string]any)
	previewID, _ := track["previewAssetId"].(string)
	assets := fixture["assets"].([]any)
	for _, raw := range assets {
		asset := raw.(map[string]any)
		if asset["id"] == previewID {
			asset["kind"] = "image"
			asset["mimeType"] = "image/webp"
		}
	}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/tracks/0/previewAssetId") {
		t.Fatalf("expected preview asset kind error, got %v", err)
	}
}

func TestValidatorRejectsAudioCoverAsset(t *testing.T) {
	var fixture map[string]any
	if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	assets := fixture["assets"].([]any)
	var audioID string
	for _, raw := range assets {
		asset := raw.(map[string]any)
		if asset["kind"] == "audio" {
			audioID, _ = asset["id"].(string)
			break
		}
	}
	releases := fixture["releases"].([]any)
	releases[0].(map[string]any)["coverAssetId"] = audioID
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/releases/0/coverAssetId") {
		t.Fatalf("expected cover asset kind error, got %v", err)
	}
}

func TestValidatorAppliesAssetConstraintsAlongsideOneOf(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(map[string]any)
		wantedPath string
	}{
		{
			name: "required field",
			mutate: func(asset map[string]any) {
				delete(asset, "checksum")
			},
			wantedPath: "/assets/0/checksum",
		},
		{
			name: "unknown field",
			mutate: func(asset map[string]any) {
				asset["unexpected"] = true
			},
			wantedPath: "/assets/0/unexpected",
		},
		{
			name: "property type",
			mutate: func(asset map[string]any) {
				asset["byteSize"] = "large"
			},
			wantedPath: "/assets/0/byteSize",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var fixture map[string]any
			if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			asset := fixture["assets"].([]any)[0].(map[string]any)
			test.mutate(asset)
			raw, err := json.Marshal(fixture)
			if err != nil {
				t.Fatalf("encode fixture: %v", err)
			}
			if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), test.wantedPath) {
				t.Fatalf("expected %s error, got %v", test.wantedPath, err)
			}
		})
	}
}

func TestValidatorRequiresExactlyOneMatchingOneOfBranch(t *testing.T) {
	validator := &Validator{}
	schema := map[string]any{
		"oneOf": []any{
			map[string]any{"type": "string"},
			map[string]any{"minLength": float64(1)},
		},
	}
	if err := validator.validateSchema("value", schema, ""); err == nil {
		t.Fatal("overlapping oneOf branches should be rejected")
	}
}
