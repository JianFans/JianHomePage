package config

import (
	"fmt"
	"strings"
	"unicode"
)

// OIDCRoleNames maps external identity-provider names to the site's fixed
// permission levels. An empty field retains that level's canonical name.
type OIDCRoleNames struct {
	Editor    string
	Reviewer  string
	Publisher string
	Admin     string
}

// WithDefaults resolves missing names and rejects ambiguous or untokenizable
// mappings before authentication starts. Names match claims case-sensitively.
func (names OIDCRoleNames) WithDefaults() (OIDCRoleNames, error) {
	seen := make(map[string]string, 4)
	for _, entry := range []struct {
		key      string
		fallback string
		value    *string
	}{
		{"OIDC_ROLE_EDITOR", "editor", &names.Editor},
		{"OIDC_ROLE_REVIEWER", "reviewer", &names.Reviewer},
		{"OIDC_ROLE_PUBLISHER", "publisher", &names.Publisher},
		{"OIDC_ROLE_ADMIN", "admin", &names.Admin},
	} {
		*entry.value = valueOrDefault(*entry.value, entry.fallback)
		if strings.ContainsFunc(*entry.value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return OIDCRoleNames{}, fmt.Errorf("%w: %s must not contain whitespace or control characters", ErrInvalidConfig, entry.key)
		}
		if previous, duplicate := seen[*entry.value]; duplicate {
			return OIDCRoleNames{}, fmt.Errorf("%w: %s and %s must name different roles", ErrInvalidConfig, previous, entry.key)
		}
		seen[*entry.value] = entry.key
	}
	return names, nil
}
