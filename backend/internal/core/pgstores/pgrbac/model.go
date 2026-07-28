package pgrbac

import (
	"time"

	"github.com/google/uuid"
)

// CustomRole is an organization-owned role and the permissions currently granted to it.
type CustomRole struct {
	ID              int        `db:"id"`
	ExternalID      uuid.UUID  `db:"external_id"`
	Name            string     `db:"name"`
	PermissionNames []string   `db:"permission_names"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       *time.Time `db:"updated_at"`
	ETag            uuid.UUID  `db:"etag"`
}

// CreateCustomRole holds the fields required to create a custom role.
type CreateCustomRole struct {
	OrgID           int
	Name            string
	PermissionNames []string
}

// UpdateCustomRole holds the fields to update on a custom role.
// Fields controls which fields are applied; fields not listed are left unchanged.
type UpdateCustomRole struct {
	OrgID           int
	ExternalID      uuid.UUID
	Fields          CustomRoleUpdateFields
	Name            string
	PermissionNames []string
}

// CustomRoleUpdateFields specifies which fields on an UpdateCustomRole should be applied.
type CustomRoleUpdateFields struct {
	Name            bool
	PermissionNames bool
}

// ModifyCustomRolePermissions holds permission-set changes for a custom role. AddPermissionNames
// and RemovePermissionNames must not overlap.
type ModifyCustomRolePermissions struct {
	OrgID                 int
	ExternalID            uuid.UUID
	AddPermissionNames    []string
	RemovePermissionNames []string
}

// SystemRole is a system role and the names of every permission currently granted to it.
type SystemRole struct {
	Name            string   `db:"name"`
	PermissionNames []string `db:"permission_names"`
}

// ProjectPermissions is a user's resolved permissions for a project, alongside the project's org.
type ProjectPermissions struct {
	OrgID           int      `db:"org_id"`
	PermissionNames []string `db:"permission_names"`
}

// OrgPermissions is a user's resolved permissions for an organization.
type OrgPermissions struct {
	OrgID           int      `db:"org_id"`
	PermissionNames []string `db:"permission_names"`
}

// PermissionsByScope contains a user's resolved permission names by scope.
type PermissionsByScope struct {
	ProjectPermissionNames []string
	OrgPermissionNames     []string
	SystemPermissionNames  []string
}
