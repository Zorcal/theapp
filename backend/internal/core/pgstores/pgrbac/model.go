package pgrbac

// Role is a role and the names of every permission currently granted to it.
type Role struct {
	Name            string   `db:"name"`
	IsStatic        bool     `db:"is_static"`
	PermissionNames []string `db:"permission_names"`
}

// ProjectPermissions is a user's resolved permissions for a project, alongside the project's org.
type ProjectPermissions struct {
	OrgID           int      `db:"org_id"`
	PermissionNames []string `db:"permission_names"`
}
