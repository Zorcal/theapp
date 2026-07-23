package mdl

import "github.com/zorcal/theapp/backend/pkg/set"

// The full set of permissions defined by the system. Every protected endpoint's required permissions are drawn from
// this list. This list and AllPermissions below must stay in sync with what's seeded into the database.

// Permission is a single named capability an endpoint can require the caller to hold.
type Permission string

// All user service permissions. User permissions are system-wide rather than project- or org-scoped — they can only be
// granted through a system-scope role assignment.
const (
	PermissionUserRead   Permission = "user:read"
	PermissionUserCreate Permission = "user:create"
	PermissionUserUpdate Permission = "user:update"
)

// All system role service permissions.
const (
	PermissionSystemRoleRead     Permission = "system-role:read"
	PermissionSystemRoleAssign   Permission = "system-role:assign"
	PermissionSystemRoleUnassign Permission = "system-role:unassign"
)

// AllPermissions lists all permissions.
var AllPermissions = []Permission{
	PermissionUserRead,
	PermissionUserCreate,
	PermissionUserUpdate,
	PermissionSystemRoleRead,
	PermissionSystemRoleAssign,
	PermissionSystemRoleUnassign,
}

// IsPermissionSuperset reports whether held contains every permission in required.
func IsPermissionSuperset(held, required []Permission) bool {
	return set.FromSlice(held).IsSuperset(set.FromSlice(required))
}
