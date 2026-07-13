package pgrbac

// Role is a role and the names of every permission currently granted to it.
type Role struct {
	Name            string   `db:"name"`
	IsStatic        bool     `db:"is_static"`
	PermissionNames []string `db:"permission_names"`
}
