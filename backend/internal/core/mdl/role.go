package mdl

// Role is a role and the permissions currently granted to it.
type Role struct {
	Name        string
	IsStatic    bool
	Permissions []Permission
}
