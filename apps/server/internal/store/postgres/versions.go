package postgres

import (
	"context"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
)

// ListVersions selects metadata only and applies a bounded descending keyset.
func (repo *ContentRepository) ListVersions(ctx context.Context, query content.ListQuery) ([]content.VersionSummary, error) {
	var boundary any
	if query.BeforeCreatedAt != nil {
		boundary = *query.BeforeCreatedAt
	}
	rows, err := repo.exec.QueryContext(ctx, `SELECT id, COALESCE(NULLIF(name,''),id), COALESCE(source_version_id,''), status, revision, checksum, review_approved, created_at, updated_at FROM content_versions WHERE ($1::timestamptz IS NULL OR (created_at,id)<($1,$2)) ORDER BY created_at DESC, id DESC LIMIT $3`, boundary, query.BeforeID, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]content.VersionSummary, 0, query.Limit)
	for rows.Next() {
		var value content.VersionSummary
		var status string
		if err := rows.Scan(&value.ID, &value.Name, &value.SourceVersionID, &status, &value.Revision, &value.Checksum, &value.ReviewApproved, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		value.Status = domain.ContentStatus(status)
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
