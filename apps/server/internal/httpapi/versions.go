package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
)

// listVersions validates singular pagination inputs before service authorization.
func (handler *Handler) listVersions(w http.ResponseWriter, r *http.Request) {
	actor, ok := principal(w, r)
	if !ok {
		return
	}
	if handler.content == nil {
		writeError(w, r, 500, "internal_error", "Service unavailable.")
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeDomainError(w, r, domain.ErrInvalidInput)
		return
	}
	for _, name := range []string{"limit", "cursor"} {
		if len(query[name]) > 1 {
			writeDomainError(w, r, domain.ErrInvalidInput)
			return
		}
	}
	options := content.ListOptions{Cursor: query.Get("cursor")}
	if values, exists := query["limit"]; exists {
		limit, err := strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > 100 {
			writeDomainError(w, r, domain.ErrInvalidInput)
			return
		}
		options.Limit = limit
	}
	page, err := handler.content.ListVersions(r.Context(), actor, options)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, 200, page)
}

// cloneVersion accepts display metadata only and reads the source on the server.
func (handler *Handler) cloneVersion(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, ok := principal(w, r)
	if !ok {
		return
	}
	if handler.content == nil {
		writeError(w, r, 500, "internal_error", "Service unavailable.")
		return
	}
	version, err := handler.content.CloneVersion(r.Context(), actor, r.PathValue("versionId"), input.Name)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeVersion(w, 201, version)
}
