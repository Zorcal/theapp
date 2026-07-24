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
