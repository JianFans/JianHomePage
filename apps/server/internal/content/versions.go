package content

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
	snapshotdata "yujian.me/server/internal/snapshot"
)

// VersionSummary omits snapshots so the library remains bounded by page size.
type VersionSummary struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	SourceVersionID string               `json:"sourceVersionId,omitempty"`
	Status          domain.ContentStatus `json:"status"`
	Revision        int64                `json:"revision"`
	Checksum        string               `json:"checksum"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
	ReviewApproved  bool                 `json:"reviewApproved,omitempty"`
}

// ListOptions is the external bounded keyset pagination input.
type ListOptions struct {
	Limit  int
	Cursor string
}

// ListQuery is the validated repository boundary using a strict tuple cursor.
type ListQuery struct {
	Limit           int
	BeforeCreatedAt *time.Time
	BeforeID        string
}

// ListPage carries metadata summaries and an opaque cursor for the next page.
type ListPage struct {
	Items      []VersionSummary `json:"items"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type versionCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

// ListVersions authorizes content managers and requests only bounded summaries.
func (service *Service) ListVersions(ctx context.Context, actor domain.Principal, options ListOptions) (ListPage, error) {
	if actor.Subject == "" || !canRead(actor) {
		return ListPage{}, domain.ErrForbidden
	}
	query, err := parseListOptions(options)
	if err != nil {
		return ListPage{}, err
	}
	items, err := service.store.ListVersions(ctx, query)
	if err != nil {
		return ListPage{}, err
	}
	limit := query.Limit - 1
	page := ListPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		raw, _ := json.Marshal(versionCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	if page.Items == nil {
		page.Items = []VersionSummary{}
	}
	return page, nil
}

// parseListOptions rejects malformed cursors instead of silently resetting pages.
func parseListOptions(options ListOptions) (ListQuery, error) {
	limit := options.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return ListQuery{}, domain.ErrInvalidInput
	}
	query := ListQuery{Limit: limit + 1}
	if options.Cursor == "" {
		return query, nil
	}
	if len(options.Cursor) > 1024 {
		return ListQuery{}, domain.ErrInvalidInput
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(options.Cursor)
	if err != nil {
		return ListQuery{}, domain.ErrInvalidInput
	}
	var cursor versionCursor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || decoder.Decode(new(any)) != io.EOF || cursor.CreatedAt.IsZero() || cursor.ID == "" || len(cursor.ID) > 128 {
		return ListQuery{}, domain.ErrInvalidInput
	}
	canonical, err := json.Marshal(cursor)
	if err != nil || string(canonical) != string(raw) {
		return ListQuery{}, domain.ErrInvalidInput
	}
	for _, character := range cursor.ID {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '_' && character != '-' {
			return ListQuery{}, domain.ErrInvalidInput
		}
	}
	query.BeforeCreatedAt = &cursor.CreatedAt
	query.BeforeID = cursor.ID
	return query, nil
}

// normalizeName permits optional concise labels while rejecting controls.
func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 120 {
		return "", domain.ErrInvalidInput
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", domain.ErrInvalidInput
		}
	}
	return name, nil
}

// SummarizeVersion copies only library metadata, including legacy name fallback.
func SummarizeVersion(version domain.ContentVersion) VersionSummary {
	name := version.Name
	if name == "" {
		name = version.ID
	}
	return VersionSummary{ID: version.ID, Name: name, SourceVersionID: version.SourceVersionID, Status: version.Status, Revision: version.Revision, Checksum: version.Checksum, CreatedAt: version.CreatedAt, UpdatedAt: version.UpdatedAt, ReviewApproved: version.ReviewApproved}
}

// CloneVersion validates the persisted source and creates an independent draft
// and audit atomically without touching publication state or approval grants.
func (service *Service) CloneVersion(ctx context.Context, actor domain.Principal, sourceID, name string) (domain.ContentVersion, error) {
	if actor.Subject == "" || !hasPermission(actor, auth.PermissionEditDraft) {
		return domain.ContentVersion{}, domain.ErrForbidden
	}
	name, err := normalizeName(name)
	if err != nil {
		return domain.ContentVersion{}, err
	}
	var clone domain.ContentVersion
	err = service.store.WithinTransaction(ctx, func(repo Repository) error {
		source, err := repo.GetVersion(ctx, sourceID)
		if err != nil {
			return err
		}
		canonical, err := snapshotdata.CanonicalJSON(source.Snapshot)
		if err != nil {
			return fmt.Errorf("%w: invalid source snapshot", domain.ErrConflict)
		}
		if snapshotdata.Checksum(canonical) != source.Checksum {
			return fmt.Errorf("%w: source checksum mismatch", domain.ErrConflict)
		}
		if err := service.validator.Validate(canonical); err != nil {
			return fmt.Errorf("%w: invalid source snapshot", domain.ErrInvalidInput)
		}
		now := service.now().UTC()
		clone = domain.ContentVersion{ID: service.newID("ver_"), Name: name, SourceVersionID: source.ID, Status: domain.StatusDraft, Revision: 1, Snapshot: cloneJSON(canonical), Checksum: snapshotdata.Checksum(canonical), CreatedBy: actor.Subject, UpdatedBy: actor.Subject, CreatedAt: now, UpdatedAt: now}
		if clone.Name == "" {
			clone.Name = clone.ID
		}
		if err := repo.CreateVersion(ctx, clone); err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]string{"sourceVersionId": source.ID})
		return repo.AppendAudit(ctx, audit(actor, "content.clone_version", clone.ID, now, metadata))
	})
	if err != nil {
		return domain.ContentVersion{}, err
	}
	return clone, nil
}
