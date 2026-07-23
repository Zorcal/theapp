package mdl

// SystemRole is a system role and the permissions currently granted to it. System roles are seed
// data and cannot be mutated directly.
type SystemRole struct {
	Name        string
	Permissions []Permission
}
