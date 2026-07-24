package mdl

import "github.com/zorcal/theapp/backend/pkg/set"

// The full set of permissions defined by the system. Every protected endpoint's required permissions are drawn from
// this list. This list and AllPermissions() below must stay in sync with what's seeded into the database.

// Permission is a single named capability an endpoint can require the caller to hold.
type Permission string

// All user service permissions. User permissions are system-wide rather than project- or org-scoped — they can only be
// granted through a system-scope role assignment.
const (
	PermissionUserRead   Permission = "user:read"
	PermissionUserCreate Permission = "user:create"
	PermissionUserUpdate Permission = "user:update"
)

// All system role service permissions. System role permissions are system-wide rather than
// project- or org-scoped — they can only be granted through a system-scope role assignment.
const (
	PermissionSystemRoleRead     Permission = "system-role:read"
	PermissionSystemRoleAssign   Permission = "system-role:assign"
	PermissionSystemRoleUnassign Permission = "system-role:unassign"
)

// All custom role service permissions. They authorize role management only within the organization
// resolved from the request's project context.
const (
	PermissionCustomRoleCreate Permission = "custom-role:create"
	PermissionCustomRoleRead   Permission = "custom-role:read"
	PermissionCustomRoleUpdate Permission = "custom-role:update"
	PermissionCustomRoleDelete Permission = "custom-role:delete"
)

// AllPermissions returns all permissions.
func AllPermissions() []Permission {
	return []Permission{
		PermissionUserRead,
		PermissionUserCreate,
		PermissionUserUpdate,
		PermissionSystemRoleRead,
		PermissionSystemRoleAssign,
		PermissionSystemRoleUnassign,
		PermissionCustomRoleCreate,
		PermissionCustomRoleRead,
		PermissionCustomRoleUpdate,
		PermissionCustomRoleDelete,
	}
}

// SystemOnlyPermissions returns permissions that may only be granted through system-scope role
// assignments.
func SystemOnlyPermissions() []Permission {
	return []Permission{
		PermissionUserRead,
		PermissionUserCreate,
		PermissionUserUpdate,
		PermissionSystemRoleRead,
		PermissionSystemRoleAssign,
		PermissionSystemRoleUnassign,
	}
}

// SystemRoleManagementPermissions returns the permissions required to manage system-role
// assignments.
func SystemRoleManagementPermissions() []Permission {
	return []Permission{
		PermissionSystemRoleAssign,
		PermissionSystemRoleUnassign,
	}
}

// IsPermissionSuperset reports whether held contains every permission in required.
func IsPermissionSuperset(held, required []Permission) bool {
	return set.FromSlice(held).IsSuperset(set.FromSlice(required))
}
