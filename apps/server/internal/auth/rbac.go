package auth

import (
	"fmt"

	"yujian.me/server/internal/domain"
)

type Role = domain.Role

const (
	RoleEditor    = domain.RoleEditor
	RoleReviewer  = domain.RoleReviewer
	RolePublisher = domain.RolePublisher
	RoleAdmin     = domain.RoleAdmin
)

type Permission string

const (
	PermissionEditDraft    Permission = "edit_draft"
	PermissionSubmitReview Permission = "submit_review"
	PermissionReview       Permission = "review"
	PermissionPublish      Permission = "publish"
	PermissionRollback     Permission = "rollback"
	PermissionCreateAsset  Permission = "create_asset"
	PermissionDeleteAsset  Permission = "delete_asset"
	PermissionManageUsers  Permission = "manage_users"
)

var permissionsByRole = map[Role]map[Permission]struct{}{
	RoleEditor: permissionSet(
		PermissionEditDraft,
		PermissionSubmitReview,
		PermissionCreateAsset,
	),
	RoleReviewer: permissionSet(PermissionReview),
	RolePublisher: permissionSet(
		PermissionPublish,
		PermissionRollback,
	),
	RoleAdmin: permissionSet(
		PermissionEditDraft,
		PermissionSubmitReview,
		PermissionReview,
		PermissionPublish,
		PermissionRollback,
		PermissionCreateAsset,
		PermissionDeleteAsset,
		PermissionManageUsers,
	),
}

func Can(role Role, permission Permission) bool {
	_, allowed := permissionsByRole[role][permission]
	return allowed
}

func ParseRoles(values []string) ([]Role, error) {
	roles := make([]Role, 0, len(values))
	seen := make(map[Role]struct{}, len(values))

	for _, value := range values {
		role := Role(value)
		if _, known := permissionsByRole[role]; !known {
			return nil, fmt.Errorf("unknown role %q", value)
		}
		if _, duplicate := seen[role]; duplicate {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}

	return roles, nil
}

func permissionSet(values ...Permission) map[Permission]struct{} {
	set := make(map[Permission]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
