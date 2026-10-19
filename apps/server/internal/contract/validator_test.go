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

// TestValidatorAcceptsInitialContent permits a real first draft without fabricated media.
func TestValidatorAcceptsInitialContent(t *testing.T) {
	var value map[string]any
	if err := json.Unmarshal(readFixture(t), &value); err != nil {
		t.Fatal(err)
	}
	delete(value["site"].(map[string]any)["seo"].(map[string]any), "ogAssetId")
	delete(value["artist"].(map[string]any), "portraitAssetId")
	for _, field := range []string{"heroSlides", "releases", "tracks", "videos", "events", "moments", "assets"} {
		value[field] = []any{}
	}
	for _, raw := range value["homepage"].(map[string]any)["sections"].([]any) {
		section := raw.(map[string]any)
		if section["type"] != "artist" {
			section["itemIds"] = []any{}
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewValidator().Validate(raw); err != nil {
		t.Fatalf("initial content should be valid: %v", err)
	}
}

// TestValidatorEventCalendarDates keeps the server's date contract aligned with
// the shared editor validator, including leap years and explicit UTC offsets.
func TestValidatorEventCalendarDates(t *testing.T) {
	cases := []struct {
		value string
		valid bool
	}{
		{"2030-02-30T20:00:00Z", false},
		{"2030-02-29T20:00:00+08:00", false},
		{"1900-02-29T20:00:00Z", false},
		{"2030-04-31T20:00:00-05:00", false},
		{"2030-01-02T24:00:00Z", false},
		{"2030-01-02T24:00:00+08:00", false},
		{"2030-01-02T20:60:00Z", false},
		{"2030-01-02T20:00:00", false},
		{"2032-02-29T20:00:00+08:00", true},
		{"2000-02-29T23:59:59.123-05:00", true},
		{"2030-01-02T12:00:00Z", true},
		{"0096-02-29T00:00:00Z", true},
		{"0000-02-29T00:00:00Z", true},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			var fixture map[string]any
			if err := json.Unmarshal(readFixture(t), &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			fixture["events"].([]any)[0].(map[string]any)["dateTime"] = tc.value
			raw, err := json.Marshal(fixture)
			if err != nil {
				t.Fatalf("encode fixture: %v", err)
			}
			err = NewValidator().Validate(raw)
			if tc.valid && err != nil {
				t.Fatalf("valid date rejected: %v", err)
			}
			if !tc.valid && (err == nil || !strings.Contains(err.Error(), "/events/0/dateTime")) {
				t.Fatalf("expected invalid date pointer, got %v", err)
			}
		})
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
	raw := bytes.Replace(readFixture(t), []byte(`"track_e60c46c7-28d6-418e-8905-167307dc8354"`), []byte(`"track_fd44d76e-d3d0-4969-86c2-605e9606b85a"`), 1)
	if err := NewValidator().Validate(raw); err == nil || !strings.Contains(err.Error(), "/releases/0/trackIds/0") {
		t.Fatalf("expected broken track reference pointer, got %v", err)
	}
}

func TestValidatorRejectsBrokenAssetReference(t *testing.T) {
	raw := bytes.Replace(readFixture(t), []byte(`"ogAssetId": "asset_e020cc5b-3c4e-4779-803e-8ccd74d7c0ad"`), []byte(`"ogAssetId": "asset_773e3dac-5b2c-40b5-83d6-1eaa5f4def9c"`), 1)
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
	moments[0].(map[string]any)["target"] = map[string]any{"kind": "internal", "contentId": "video_b2c95663-9e0f-4947-8325-365131897183"}
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
	heroes[2].(map[string]any)["target"].(map[string]any)["contentId"] = "release_f9462a63-70d9-40b3-a0cc-a792f1b5499e"
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
	heroes[2].(map[string]any)["target"].(map[string]any)["contentId"] = "artist_eb908750-faf8-418f-94c6-97f769c1cf31"
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
	heroes[2].(map[string]any)["target"].(map[string]any)["contentId"] = "event_dc200c4c-abbb-476e-984d-405470f2dbea"
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
