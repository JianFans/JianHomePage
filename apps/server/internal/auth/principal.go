package auth

import (
	"context"

	"yujian.me/server/internal/domain"
)

type principalContextKey struct{}

func PrincipalFromContext(ctx context.Context) (domain.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(domain.Principal)
	return principal, ok
}

func withPrincipal(ctx context.Context, principal domain.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func HasPermission(principal domain.Principal, permission Permission) bool {
	for _, role := range principal.Roles {
		if Can(role, permission) {
			return true
		}
	}
	return false
}
