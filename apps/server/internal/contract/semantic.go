package contract

import "time"

// The JSON Schema describes the shape of a snapshot. These checks describe
// relationships between records that JSON Schema intentionally cannot express
// without making the content contract difficult to author.
func validateSnapshotSemantics(value any) error {
	root, ok := value.(map[string]any)
	if !ok {
		return validationFailure("", "root must be an object")
	}
	assets, err := indexRecords(root, "assets", "asset_", "/assets")
	if err != nil {
		return err
	}
	heroes, err := indexRecords(root, "heroSlides", "hero_", "/heroSlides")
	if err != nil {
		return err
	}
	releases, err := indexRecords(root, "releases", "release_", "/releases")
	if err != nil {
		return err
	}
	tracks, err := indexRecords(root, "tracks", "track_", "/tracks")
	if err != nil {
		return err
	}
	videos, err := indexRecords(root, "videos", "video_", "/videos")
	if err != nil {
		return err
	}
	events, err := indexRecords(root, "events", "event_", "/events")
	if err != nil {
		return err
	}
	moments, err := indexRecords(root, "moments", "moment_", "/moments")
	if err != nil {
		return err
	}
	contentIDs := renderedContentIDs(root, heroes, releases, videos, events, moments)

	if err := validateSiteReferences(root, assets); err != nil {
		return err
	}
	if err := validateHomepageReferences(root, heroes, releases, videos, events, moments); err != nil {
		return err
	}
	if err := validateHeroReferences(root, heroes, assets, releases, contentIDs); err != nil {
		return err
	}
	if err := validateReleaseReferences(root, releases, tracks, assets); err != nil {
		return err
	}
	if err := validateTrackReferences(root, tracks, releases, assets); err != nil {
		return err
	}
	if err := validateVideoReferences(root, videos, assets); err != nil {
		return err
	}
	if err := validateEventReferences(root, events, assets); err != nil {
		return err
	}
	if err := validateMomentReferences(root, moments, assets, contentIDs); err != nil {
		return err
	}
	if err := validateArtistReferences(root, assets); err != nil {
		return err
	}
	return validateAssetReferences(root, assets)
}

func indexRecords(root map[string]any, key, prefix, path string) (map[string]map[string]any, error) {
	result := make(map[string]map[string]any)
	list, _ := root[key].([]any)
	for index, raw := range list {
		record, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := record["id"].(string)
		if _, exists := result[id]; exists {
			return nil, validationFailure(schemaPath(path, itoa(index))+"/id", "duplicate id")
		}
		if len(prefix) > 0 && len(id) > 0 && !hasPrefix(id, prefix) {
			return nil, validationFailure(schemaPath(path, itoa(index))+"/id", "has an invalid id prefix")
		}
		result[id] = record
	}
	return result, nil
}

func validateSiteReferences(root map[string]any, assets map[string]map[string]any) error {
	site, _ := root["site"].(map[string]any)
	seo, _ := site["seo"].(map[string]any)
	return requireAssetKind(seo["ogAssetId"], "/site/seo/ogAssetId", assets, "image", "gif")
}

func validateHomepageReferences(root map[string]any, heroes, releases, videos, events, moments map[string]map[string]any) error {
	homepage, _ := root["homepage"].(map[string]any)
	sections, _ := homepage["sections"].([]any)
	for index, raw := range sections {
		section, _ := raw.(map[string]any)
		sectionType, _ := section["type"].(string)
		itemIDs, _ := section["itemIds"].([]any)
		var records map[string]map[string]any
		switch sectionType {
		case "hero":
			records = heroes
		case "music":
			records = releases
		case "video":
			records = videos
		case "event":
			records = events
		case "moment":
			records = moments
		case "artist":
			for itemIndex, rawID := range itemIDs {
				if rawID != "artist_primary" {
					return validationFailure(schemaPath(schemaPath("/homepage/sections", itoa(index)), "itemIds")+"/"+itoa(itemIndex), "unknown artist reference")
				}
			}
			continue
		}
		for itemIndex, rawID := range itemIDs {
			id, _ := rawID.(string)
			if _, exists := records[id]; !exists {
				return validationFailure("/homepage/sections/"+itoa(index)+"/itemIds/"+itoa(itemIndex), "references an unknown record")
			}
		}
	}
	return nil
}

func renderedContentIDs(root map[string]any, heroes, releases, videos, events, moments map[string]map[string]any) map[string]struct{} {
	result := make(map[string]struct{})
	referenceTime, _ := time.Parse(time.RFC3339, root["generatedAt"].(string))
	homepage, _ := root["homepage"].(map[string]any)
	sections, _ := homepage["sections"].([]any)
	artist, _ := root["artist"].(map[string]any)
	artistID, _ := artist["id"].(string)

	for _, raw := range sections {
		section, _ := raw.(map[string]any)
		enabled, _ := section["enabled"].(bool)
		if !enabled {
			continue
		}
		sectionType, _ := section["type"].(string)
		itemIDs, _ := section["itemIds"].([]any)
		limit, _ := schemaIntegerValue(section["limit"])

		switch sectionType {
		case "hero":
			for _, rawID := range itemIDs {
				id, _ := rawID.(string)
				if slide, exists := heroes[id]; exists && heroVisibleAt(slide, referenceTime) {
					result[id] = struct{}{}
				}
			}
		case "music":
			for _, rawID := range limitedIDs(itemIDs, limit) {
				id, _ := rawID.(string)
				release, exists := releases[id]
				if !exists {
					continue
				}
				result[id] = struct{}{}
				trackIDs, _ := release["trackIds"].([]any)
				for _, rawTrackID := range trackIDs {
					trackID, _ := rawTrackID.(string)
					result[trackID] = struct{}{}
				}
			}
		case "video":
			addRenderedIDs(result, limitedIDs(itemIDs, limit), videos)
		case "event":
			visible := make([]any, 0, len(itemIDs))
			for _, rawID := range itemIDs {
				id, _ := rawID.(string)
				event, exists := events[id]
				if !exists || event["status"] != "scheduled" {
					continue
				}
				dateTime, _ := time.Parse(time.RFC3339, event["dateTime"].(string))
				if dateTime.After(referenceTime) {
					visible = append(visible, rawID)
				}
			}
			addRenderedIDs(result, limitedIDs(visible, limit), events)
		case "moment":
			addRenderedIDs(result, limitedIDs(itemIDs, limit), moments)
		case "artist":
			if len(itemIDs) > 0 && itemIDs[0] == artistID {
				result[artistID] = struct{}{}
			}
		}
	}
	return result
}

func heroVisibleAt(slide map[string]any, referenceTime time.Time) bool {
	if value, exists := slide["startsAt"].(string); exists {
		startsAt, _ := time.Parse(time.RFC3339, value)
		if startsAt.After(referenceTime) {
			return false
		}
	}
	if value, exists := slide["endsAt"].(string); exists {
		endsAt, _ := time.Parse(time.RFC3339, value)
		if !referenceTime.Before(endsAt) {
			return false
		}
	}
	return true
}

func limitedIDs(values []any, limit int) []any {
	if limit <= 0 || limit >= len(values) {
		return values
	}
	return values[:limit]
}

func addRenderedIDs(result map[string]struct{}, values []any, records map[string]map[string]any) {
	for _, rawID := range values {
		id, _ := rawID.(string)
		if _, exists := records[id]; exists {
			result[id] = struct{}{}
		}
	}
}

func validateHeroReferences(root map[string]any, heroes, assets, releases map[string]map[string]any, contentIDs map[string]struct{}) error {
	list, _ := root["heroSlides"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		base := "/heroSlides/" + itoa(index)
		mediaKind, _ := record["mediaKind"].(string)
		for _, field := range []string{"assetId", "mobileAssetId"} {
			if value, exists := record[field]; exists {
				if err := requireAssetKind(value, base+"/"+field, assets, mediaKind); err != nil {
					return err
				}
			}
		}
		if value, exists := record["posterAssetId"]; exists {
			if err := requireAssetKind(value, base+"/posterAssetId", assets, "image", "gif"); err != nil {
				return err
			}
		}
		if value, exists := record["releaseId"]; exists {
			if err := requireReference(value, base+"/releaseId", releases); err != nil {
				return err
			}
		}
		if target, exists := record["target"].(map[string]any); exists && target["kind"] == "internal" {
			if id, _ := target["contentId"].(string); id != "" {
				if _, exists := contentIDs[id]; !exists {
					return validationFailure(base+"/target/contentId", "references an unknown record")
				}
			}
		}
	}
	return nil
}

func validateReleaseReferences(root map[string]any, releases, tracks, assets map[string]map[string]any) error {
	list, _ := root["releases"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		base := "/releases/" + itoa(index)
		if err := requireAssetKind(record["coverAssetId"], base+"/coverAssetId", assets, "image", "gif"); err != nil {
			return err
		}
		trackIDs, _ := record["trackIds"].([]any)
		releaseID, _ := record["id"].(string)
		for trackIndex, rawTrackID := range trackIDs {
			trackID, _ := rawTrackID.(string)
			track, exists := tracks[trackID]
			if !exists {
				return validationFailure(base+"/trackIds/"+itoa(trackIndex), "references an unknown record")
			}
			if trackReleaseID, _ := track["releaseId"].(string); trackReleaseID != releaseID {
				return validationFailure(base+"/trackIds/"+itoa(trackIndex), "track belongs to a different release")
			}
		}
	}
	return nil
}

func validateTrackReferences(root map[string]any, tracks, releases, assets map[string]map[string]any) error {
	list, _ := root["tracks"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		base := "/tracks/" + itoa(index)
		if err := requireReference(record["releaseId"], base+"/releaseId", releases); err != nil {
			return err
		}
		if value, exists := record["previewAssetId"]; exists {
			if err := requireAssetKind(value, base+"/previewAssetId", assets, "audio"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateVideoReferences(root map[string]any, videos, assets map[string]map[string]any) error {
	list, _ := root["videos"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		base := "/videos/" + itoa(index)
		if err := requireAssetKind(record["posterAssetId"], base+"/posterAssetId", assets, "image", "gif"); err != nil {
			return err
		}
		if value, exists := record["videoAssetId"]; exists {
			if err := requireAssetKind(value, base+"/videoAssetId", assets, "video"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateEventReferences(root map[string]any, events, assets map[string]map[string]any) error {
	list, _ := root["events"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		if value, exists := record["posterAssetId"]; exists {
			if err := requireAssetKind(value, "/events/"+itoa(index)+"/posterAssetId", assets, "image", "gif"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMomentReferences(root map[string]any, moments, assets map[string]map[string]any, contentIDs map[string]struct{}) error {
	list, _ := root["moments"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		base := "/moments/" + itoa(index)
		if err := requireAssetKind(record["assetId"], base+"/assetId", assets, "image", "gif"); err != nil {
			return err
		}
		if target, exists := record["target"].(map[string]any); exists && target["kind"] == "internal" {
			id, _ := target["contentId"].(string)
			if _, exists := contentIDs[id]; !exists {
				return validationFailure(base+"/target/contentId", "references an unknown record")
			}
		}
	}
	return nil
}

func validateArtistReferences(root map[string]any, assets map[string]map[string]any) error {
	artist, _ := root["artist"].(map[string]any)
	return requireAssetKind(artist["portraitAssetId"], "/artist/portraitAssetId", assets, "image", "gif")
}

func validateAssetReferences(root map[string]any, assets map[string]map[string]any) error {
	list, _ := root["assets"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		if value, exists := record["posterAssetId"]; exists {
			if err := requireAssetKind(value, "/assets/"+itoa(index)+"/posterAssetId", assets, "image", "gif"); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireReference(value any, path string, records map[string]map[string]any) error {
	id, ok := value.(string)
	if !ok || id == "" {
		return validationFailure(path, "reference must be a non-empty string")
	}
	if _, exists := records[id]; !exists {
		return validationFailure(path, "references an unknown record")
	}
	return nil
}

func requireAssetKind(value any, path string, assets map[string]map[string]any, allowedKinds ...string) error {
	if err := requireReference(value, path, assets); err != nil {
		return err
	}
	id, _ := value.(string)
	kind, _ := assets[id]["kind"].(string)
	for _, allowed := range allowedKinds {
		if kind == allowed {
			return nil
		}
	}
	return validationFailure(path, "references an asset with an incompatible kind")
}

func hasPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	result := make([]byte, 0, 6)
	for value > 0 {
		result = append([]byte{byte('0' + value%10)}, result...)
		value /= 10
	}
	return string(result)
}
