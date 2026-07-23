package pgrbac

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
