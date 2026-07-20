package pgrbac

import (
	"time"

	"github.com/google/uuid"
)

// RoleCustom is a custom role, owned by one organization, and the names of every permission
// currently granted to it. ID is the internal serial primary key, used for foreign keys and
// joins; ExternalID is the client-facing identifier.
type RoleCustom struct {
	ID              int        `db:"id"`
	ExternalID      uuid.UUID  `db:"external_id"`
	Name            string     `db:"name"`
	OrgID           int        `db:"org_id"`
	PermissionNames []string   `db:"permission_names"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       *time.Time `db:"updated_at"`
	ETag            uuid.UUID  `db:"etag"`
}

// RoleStatic is a static role and the names of every permission currently granted to it. ID is
// the internal serial primary key, used for foreign keys and joins; ExternalID is the
// client-facing identifier. Static roles are seed data — our system doesn't support mutating one
// directly.
type RoleStatic struct {
	ID              int        `db:"id"`
	ExternalID      uuid.UUID  `db:"external_id"`
	Name            string     `db:"name"`
	PermissionNames []string   `db:"permission_names"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       *time.Time `db:"updated_at"`
}

// CreateRole holds the fields needed to insert a new custom role owned by OrgID.
type CreateRole struct {
	OrgID           int
	Name            string
	PermissionNames []string
}

// UpdateRole holds the fields needed to replace a custom role's name and permission set. OrgID
// scopes the update to the role owned by that organization.
type UpdateRole struct {
	ID              int
	OrgID           int
	Name            string
	PermissionNames []string
}

// ProjectPermissions is a user's resolved permissions for a project, alongside the project's org.
type ProjectPermissions struct {
	OrgID           int      `db:"org_id"`
	PermissionNames []string `db:"permission_names"`
}

// RoleAssignment is a custom role a user holds at a particular scope. Exactly one of ProjectID or
// OrgID is set, identifying the scope.
type RoleAssignment struct {
	RoleExternalID uuid.UUID `db:"role_external_id"`
	RoleName       string    `db:"role_name"`
	ProjectID      *int      `db:"project_id"`
	OrgID          *int      `db:"org_id"`
}
