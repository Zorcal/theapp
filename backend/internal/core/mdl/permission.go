package mdl

import (
	"fmt"
	"slices"

	"github.com/zorcal/theapp/backend/pkg/set"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

// The full set of permissions defined by the system. Every protected endpoint's required permissions are drawn from
// this list. This list and AllPermissions() below must stay in sync with what's seeded into the database.

// Permission is a single named capability an endpoint can require the caller to hold.
type Permission string

// AssignmentScope is the narrowest scope at which a permission or role is meaningful.
type AssignmentScope int

const (
	AssignmentScopeProject AssignmentScope = iota + 1
	AssignmentScopeOrganization
	AssignmentScopeSystem
)

// PermissionDescriptor describes a custom-role-assignable permission.
type PermissionDescriptor struct {
	Permission             Permission
	MinimumAssignmentScope AssignmentScope
}

// All user service permissions. User permissions are system-wide rather than project- or org-scoped — they can only be
// granted through a system-scope role assignment.
const (
	PermissionUserRead    Permission = "user:read"
	PermissionUserCreate  Permission = "user:create"
	PermissionUserUpdate  Permission = "user:update"
	PermissionUserDelete  Permission = "user:delete"
	PermissionUserRestore Permission = "user:restore"
)

// All system role service permissions are system-wide, can only be granted through a system-scope
// role assignment, and have endpoints anchored on the theapp organization's control project.
const (
	PermissionSystemRoleRead     Permission = "system-role:read"
	PermissionSystemRoleAssign   Permission = "system-role:assign"
	PermissionSystemRoleUnassign Permission = "system-role:unassign"
)

// PermissionProjectDiscoverAll allows a system-scoped role to make every project discoverable.
const PermissionProjectDiscoverAll Permission = "project:discover-all"

// Organization and project lifecycle permissions.
const (
	// PermissionOrgCreate is project-scoped and anchored on the theapp organization's control
	// project. This restricts organization creation to callers explicitly authorized through that
	// permanent system project.
	PermissionOrgCreate Permission = "org:create"

	// PermissionProjectCreate is organization-scoped and anchored on the target organization's
	// default project for request-context resolution. Creating a sibling project changes
	// organization-level state, so a project-scope role assignment cannot grant it.
	PermissionProjectCreate Permission = "project:create"

	// PermissionOrgUserCreate authorizes creating a system user when needed and adding
	// that user to the organization resolved from the request's control project.
	PermissionOrgUserCreate Permission = "org:user-create"

	// PermissionOrgUserRead authorizes listing users in the organization resolved from
	// the request's control project.
	PermissionOrgUserRead Permission = "org:user-read"

	// PermissionOrgUserRemove authorizes removing a user and their role assignments from the
	// organization resolved from the request's control project.
	PermissionOrgUserRemove Permission = "org:user-remove"
)

// All custom role service permissions. They authorize role management only within the organization
// resolved from the request's project context.
const (
	PermissionCustomRoleCreate                 Permission = "custom-role:create"
	PermissionCustomRoleRead                   Permission = "custom-role:read"
	PermissionCustomRoleUpdate                 Permission = "custom-role:update"
	PermissionCustomRoleDelete                 Permission = "custom-role:delete"
	PermissionCustomRoleAssignProject          Permission = "custom-role:assign-project"
	PermissionCustomRoleUnassignProject        Permission = "custom-role:unassign-project"
	PermissionCustomRoleAssignOrg              Permission = "custom-role:assign-org"
	PermissionCustomRoleUnassignOrg            Permission = "custom-role:unassign-org"
	PermissionCustomRoleReadProjectAssignments Permission = "custom-role:read-project-assignments"
	PermissionCustomRoleReadOrgAssignments     Permission = "custom-role:read-org-assignments"
)

// AllPermissions returns all permissions.
func AllPermissions() []Permission {
	return []Permission{
		PermissionUserRead,
		PermissionUserCreate,
		PermissionUserUpdate,
		PermissionUserDelete,
		PermissionUserRestore,
		PermissionSystemRoleRead,
		PermissionSystemRoleAssign,
		PermissionSystemRoleUnassign,
		PermissionProjectDiscoverAll,
		PermissionOrgCreate,
		PermissionProjectCreate,
		PermissionOrgUserCreate,
		PermissionOrgUserRead,
		PermissionOrgUserRemove,
		PermissionCustomRoleCreate,
		PermissionCustomRoleRead,
		PermissionCustomRoleUpdate,
		PermissionCustomRoleDelete,
		PermissionCustomRoleAssignProject,
		PermissionCustomRoleUnassignProject,
		PermissionCustomRoleAssignOrg,
		PermissionCustomRoleUnassignOrg,
		PermissionCustomRoleReadProjectAssignments,
		PermissionCustomRoleReadOrgAssignments,
	}
}

// SystemOnlyPermissions returns permissions that may only be granted through system-scope role
// assignments.
func SystemOnlyPermissions() []Permission {
	return []Permission{
		PermissionUserRead,
		PermissionUserCreate,
		PermissionUserUpdate,
		PermissionUserDelete,
		PermissionUserRestore,
		PermissionSystemRoleRead,
		PermissionSystemRoleAssign,
		PermissionSystemRoleUnassign,
		PermissionProjectDiscoverAll,
	}
}

// PermissionsAssignableToCustomRoles returns permissions that may be granted through custom roles.
func PermissionsAssignableToCustomRoles() []Permission {
	return []Permission{
		PermissionCustomRoleCreate,
		PermissionCustomRoleRead,
		PermissionCustomRoleUpdate,
		PermissionCustomRoleDelete,
		PermissionCustomRoleAssignProject,
		PermissionCustomRoleUnassignProject,
		PermissionCustomRoleAssignOrg,
		PermissionCustomRoleUnassignOrg,
		PermissionCustomRoleReadProjectAssignments,
		PermissionCustomRoleReadOrgAssignments,
		PermissionOrgCreate,
		PermissionProjectCreate,
		PermissionOrgUserCreate,
		PermissionOrgUserRead,
		PermissionOrgUserRemove,
	}
}

// OrganizationAdminPermissions returns the canonical permissions held by every managed
// organization administrator role.
func OrganizationAdminPermissions() []Permission {
	return []Permission{
		PermissionProjectCreate,
		PermissionOrgUserCreate,
		PermissionOrgUserRead,
		PermissionOrgUserRemove,
		PermissionCustomRoleCreate,
		PermissionCustomRoleRead,
		PermissionCustomRoleUpdate,
		PermissionCustomRoleDelete,
		PermissionCustomRoleAssignProject,
		PermissionCustomRoleUnassignProject,
		PermissionCustomRoleAssignOrg,
		PermissionCustomRoleUnassignOrg,
		PermissionCustomRoleReadProjectAssignments,
		PermissionCustomRoleReadOrgAssignments,
	}
}

// PermissionAssignmentScope returns the narrowest scope at which permission is meaningful.
func PermissionAssignmentScope(permission Permission) AssignmentScope {
	if slices.Contains(SystemOnlyPermissions(), permission) {
		return AssignmentScopeSystem
	}
	switch permission {
	case PermissionCustomRoleCreate,
		PermissionCustomRoleRead,
		PermissionCustomRoleUpdate,
		PermissionCustomRoleDelete,
		PermissionCustomRoleAssignOrg,
		PermissionCustomRoleUnassignOrg,
		PermissionCustomRoleReadOrgAssignments,
		PermissionProjectCreate,
		PermissionOrgUserCreate,
		PermissionOrgUserRead,
		PermissionOrgUserRemove:
		return AssignmentScopeOrganization
	case PermissionCustomRoleAssignProject,
		PermissionCustomRoleUnassignProject,
		PermissionCustomRoleReadProjectAssignments,
		PermissionOrgCreate:
		return AssignmentScopeProject
	default:
		// Permissions are defined by the backend's closed permission registry. Reaching this
		// branch means a permission was added without defining its assignment behavior.
		panic(fmt.Sprintf("unsupported permission: %q", permission))
	}
}

// CustomRolePermissionDescriptors returns the permissions available to custom roles with their
// minimum assignment scopes.
func CustomRolePermissionDescriptors() []PermissionDescriptor {
	return slicesx.Map(PermissionsAssignableToCustomRoles(), func(p Permission) PermissionDescriptor {
		return PermissionDescriptor{
			Permission:             p,
			MinimumAssignmentScope: PermissionAssignmentScope(p),
		}
	})
}

// MinimumAssignmentScope returns the broadest minimum scope required by permissions.
func MinimumAssignmentScope(permissions []Permission) AssignmentScope {
	scope := AssignmentScopeProject
	for _, permission := range permissions {
		scope = max(scope, PermissionAssignmentScope(permission))
	}
	return scope
}

// IsPermissionSuperset reports whether held contains every permission in required.
func IsPermissionSuperset(held, required []Permission) bool {
	return set.FromSlice(held).IsSuperset(set.FromSlice(required))
}
