package mdl

// RoleStatic is a static role and the permissions currently granted to it. Static roles are seed
// data — our system doesn't support mutating one directly.
type RoleStatic struct {
	Name        string
	Permissions []Permission
}
