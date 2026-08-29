package contract

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
	contentIDs := mergeIndexes(heroes, releases, tracks, videos, events, moments)
	contentIDs["artist_primary"] = struct{}{}

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
	if err := validateMomentReferences(root, moments, assets); err != nil {
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
	return requireReference(seo["ogAssetId"], "/site/seo/ogAssetId", assets)
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

func validateHeroReferences(root map[string]any, heroes, assets, releases map[string]map[string]any, contentIDs map[string]struct{}) error {
	list, _ := root["heroSlides"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		base := "/heroSlides/" + itoa(index)
		for _, field := range []string{"assetId", "mobileAssetId", "posterAssetId"} {
			if value, exists := record[field]; exists {
				if err := requireReference(value, base+"/"+field, assets); err != nil {
					return err
				}
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
		if err := requireReference(record["coverAssetId"], base+"/coverAssetId", assets); err != nil {
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
			if err := requireReference(value, base+"/previewAssetId", assets); err != nil {
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
		if err := requireReference(record["posterAssetId"], base+"/posterAssetId", assets); err != nil {
			return err
		}
		if value, exists := record["videoAssetId"]; exists {
			if err := requireReference(value, base+"/videoAssetId", assets); err != nil {
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
			if err := requireReference(value, "/events/"+itoa(index)+"/posterAssetId", assets); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMomentReferences(root map[string]any, moments, assets map[string]map[string]any) error {
	list, _ := root["moments"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		if err := requireReference(record["assetId"], "/moments/"+itoa(index)+"/assetId", assets); err != nil {
			return err
		}
	}
	return nil
}

func validateArtistReferences(root map[string]any, assets map[string]map[string]any) error {
	artist, _ := root["artist"].(map[string]any)
	return requireReference(artist["portraitAssetId"], "/artist/portraitAssetId", assets)
}

func validateAssetReferences(root map[string]any, assets map[string]map[string]any) error {
	list, _ := root["assets"].([]any)
	for index, raw := range list {
		record, _ := raw.(map[string]any)
		if value, exists := record["posterAssetId"]; exists {
			if err := requireReference(value, "/assets/"+itoa(index)+"/posterAssetId", assets); err != nil {
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

func mergeIndexes(values ...map[string]map[string]any) map[string]struct{} {
	result := make(map[string]struct{})
	for _, value := range values {
		for key := range value {
			result[key] = struct{}{}
		}
	}
	return result
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
