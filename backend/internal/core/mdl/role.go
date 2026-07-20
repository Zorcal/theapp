package mdl

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// RoleCustom is a custom role, owned by one organization, and the permissions currently granted
// to it.
type RoleCustom struct {
	ID          uuid.UUID
	Name        string
	OrgID       int
	Permissions []Permission
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	// ETag changes on every write, so a client can detect it's about to overwrite a change made
	// since it last read the role.
	ETag string
}

// RoleStatic is a static role and the permissions currently granted to it. Static roles are seed
// data — our system doesn't support mutating one directly.
type RoleStatic struct {
	ID          uuid.UUID
	Name        string
	Permissions []Permission
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}

// CreateRole holds the fields needed to create a new custom role. The role's owning organization
// is not part of this type — it's always the organization of the caller's current project, never
// caller-supplied.
type CreateRole struct {
	Name        string
	Permissions []Permission
}

func (cr CreateRole) Validate() error {
	if cr.Name == "" {
		return validationError("name required")
	}
	if err := validatePermissions(cr.Permissions); err != nil {
		return err
	}
	return nil
}

// UpdateRole holds the fields that can be updated on a custom role.
// ID identifies the role to update and is not itself updated. Permissions, when applied, replaces
// the role's entire permission set — see ModifyRolePermissions for an additive alternative that
// doesn't risk clobbering a concurrent edit to a different permission.
type UpdateRole struct {
	ID          uuid.UUID
	Fields      RoleUpdateFields
	Name        string
	Permissions []Permission
}

// RoleUpdateFields specifies which fields on an UpdateRole should be applied.
type RoleUpdateFields struct {
	Name        bool
	Permissions bool
}

func (ur UpdateRole) Validate() error {
	if ur.ID == (uuid.UUID{}) {
		return validationError("id required")
	}
	if !ur.Fields.Name && !ur.Fields.Permissions {
		return validationError("at least one field must be set")
	}
	if ur.Fields.Name && ur.Name == "" {
		return validationError("name required")
	}
	if ur.Fields.Permissions {
		if err := validatePermissions(ur.Permissions); err != nil {
			return err
		}
	}
	return nil
}

// ModifyRolePermissions holds the fields needed to add and/or remove permissions from a custom
// role's permission set. Unlike UpdateRole's Permissions field, this names what changes, not the
// resulting set, so a concurrent edit to a different permission on the same role isn't silently
// dropped the way a full-list replace would drop it.
type ModifyRolePermissions struct {
	ID                uuid.UUID
	AddPermissions    []Permission
	RemovePermissions []Permission
}

func (m ModifyRolePermissions) Validate() error {
	if m.ID == (uuid.UUID{}) {
		return validationError("id required")
	}
	if len(m.AddPermissions) == 0 && len(m.RemovePermissions) == 0 {
		return validationError("add_permissions or remove_permissions required")
	}
	if err := validateAssignablePermissions(m.AddPermissions); err != nil {
		return err
	}
	if err := validateAssignablePermissions(m.RemovePermissions); err != nil {
		return err
	}
	for _, p := range m.AddPermissions {
		if slices.Contains(m.RemovePermissions, p) {
			return validationError(fmt.Sprintf("permission %q in both add_permissions and remove_permissions", p))
		}
	}
	return nil
}

// RoleScope selects the scope a role assignment applies to: a single project, or every project
// under an organization. Exactly one of ProjectID or OrgID must be set.
type RoleScope struct {
	ProjectID *int
	OrgID     *int
}

func (s RoleScope) Validate() error {
	if (s.ProjectID == nil) == (s.OrgID == nil) {
		return validationError("exactly one of project_id or org_id required")
	}
	return nil
}

// AssignRole holds the fields needed to assign RoleID to UserID at the given Scope.
type AssignRole struct {
	RoleID uuid.UUID
	UserID uuid.UUID
	Scope  RoleScope
}

func (a AssignRole) Validate() error {
	if a.RoleID == (uuid.UUID{}) {
		return validationError("role_id required")
	}
	if a.UserID == (uuid.UUID{}) {
		return validationError("user_id required")
	}
	if err := a.Scope.Validate(); err != nil {
		return err
	}
	return nil
}

// UnassignRole holds the fields needed to unassign RoleID from UserID at the given Scope.
type UnassignRole struct {
	RoleID uuid.UUID
	UserID uuid.UUID
	Scope  RoleScope
}

func (u UnassignRole) Validate() error {
	if u.RoleID == (uuid.UUID{}) {
		return validationError("role_id required")
	}
	if u.UserID == (uuid.UUID{}) {
		return validationError("user_id required")
	}
	if err := u.Scope.Validate(); err != nil {
		return err
	}
	return nil
}

// RoleAssignment is a role a user holds at a particular scope.
type RoleAssignment struct {
	RoleID   uuid.UUID
	RoleName string
	Scope    RoleScope
}

func validatePermissions(perms []Permission) error {
	if len(perms) == 0 {
		return validationError("at least one permission required")
	}
	return validateAssignablePermissions(perms)
}

func validateAssignablePermissions(perms []Permission) error {
	for _, p := range perms {
		if !slices.Contains(AssignablePermissions, p) {
			return validationError(fmt.Sprintf("unassignable permission %q", p))
		}
	}
	return nil
}
