package mdl

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

// All role service permissions.
const (
	PermissionRoleRead     Permission = "role:read"
	PermissionRoleCreate   Permission = "role:create"
	PermissionRoleUpdate   Permission = "role:update"
	PermissionRoleDelete   Permission = "role:delete"
	PermissionRoleAssign   Permission = "role:assign"
	PermissionRoleUnassign Permission = "role:unassign"
)

// SystemRoleService permissions. Distinct from PermissionRoleRead/PermissionRoleAssign/
// PermissionRoleUnassign above given the difference in scope: those apply to a role scoped to a
// single project or org, while these apply to a role — including superadmin — assignable
// system-wide. Only superadmin holds these (see internal/data/pgschema/seed.sql).
const (
	PermissionRoleReadSystem     Permission = "role:read-system"
	PermissionRoleAssignSystem   Permission = "role:assign-system"
	PermissionRoleUnassignSystem Permission = "role:unassign-system"
)

// AllPermissions lists all permissions.
var AllPermissions = []Permission{
	PermissionUserRead,
	PermissionUserCreate,
	PermissionUserUpdate,
	PermissionRoleRead,
	PermissionRoleCreate,
	PermissionRoleUpdate,
	PermissionRoleDelete,
	PermissionRoleAssign,
	PermissionRoleUnassign,
	PermissionRoleReadSystem,
	PermissionRoleAssignSystem,
	PermissionRoleUnassignSystem,
}

// AssignablePermissions lists the permissions a custom role can hold — every permission actually
// checked at project or org scope. This excludes:
//
//   - Every SystemRoleService permission (PermissionRole*System): a custom role can never be
//     assigned at system scope (see internal/core/rbac/README.md), so one of these in a custom
//     role's permission set could never take effect there, and would only invite confusion about
//     what the role actually grants.
//   - PermissionUser*: these are resolved system-scope-only today (UserService is a
//     project-independent directory, see internal/api/grpc/server.go's noProjectMethods), so a
//     project- or org-scoped custom role granting one would never actually take effect either —
//     the same reasoning as the system-only exclusion above. Once org-scoped user management
//     exists (see docs/permissions-and-roles-tasks.md phase 29) with its own project/org-checked
//     permissions, those belong here; PermissionUser* itself doesn't move.
var AssignablePermissions = []Permission{
	PermissionRoleRead,
	PermissionRoleCreate,
	PermissionRoleUpdate,
	PermissionRoleDelete,
	PermissionRoleAssign,
	PermissionRoleUnassign,
}
