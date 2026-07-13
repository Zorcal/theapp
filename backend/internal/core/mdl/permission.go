package mdl

// Permission is a single named capability an endpoint can require the caller to hold.
type Permission string

// The full set of permissions defined by the system. Every protected endpoint's required permissions are drawn from
// this list. This list and AllPermissions below must stay in sync with what's seeded into the database — see
// internal/core/rbac/README.md.
const (
	PermissionUserRead   Permission = "user:read"
	PermissionUserCreate Permission = "user:create"
	PermissionUserUpdate Permission = "user:update"
)

// AllPermissions lists all permissions.
var AllPermissions = []Permission{
	PermissionUserRead,
	PermissionUserCreate,
	PermissionUserUpdate,
}
