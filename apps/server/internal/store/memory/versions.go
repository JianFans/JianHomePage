package memory

import (
	"context"
	"slices"
	"strings"
	"yujian.me/server/internal/content"
)

// ListVersions returns metadata in the same descending tuple order as PostgreSQL.
func (repo *ContentRepository) ListVersions(ctx context.Context, query content.ListQuery) ([]content.VersionSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items := make([]content.VersionSummary, 0, query.Limit)
	err := repo.withRead(func() error {
		for _, version := range repo.state.versions {
			if query.BeforeCreatedAt != nil && !(version.CreatedAt.Before(*query.BeforeCreatedAt) || (version.CreatedAt.Equal(*query.BeforeCreatedAt) && version.ID < query.BeforeID)) {
				continue
			}
			items = append(items, content.SummarizeVersion(version))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(items, func(left, right content.VersionSummary) int {
		if compared := right.CreatedAt.Compare(left.CreatedAt); compared != 0 {
			return compared
		}
		return -strings.Compare(left.ID, right.ID)
	})
	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}
