package mdl

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// ErrAlreadyExists is returned when a resource cannot be created because it
// conflicts with an existing one (e.g. duplicate email).
var ErrAlreadyExists = errors.New("already exists")

// ErrTokenInvalid is returned when a magic-link or refresh token is expired,
// consumed, revoked, or not found. A single sentinel avoids leaking which
// condition applies.
var ErrTokenInvalid = errors.New("token invalid")

// ErrRateLimited is returned when a request is rejected because the caller
// issued a prior request too recently.
var ErrRateLimited = errors.New("rate limited")

// ErrValidation is returned by an input type's Validate method when the value is invalid.
var ErrValidation = errors.New("validation failed")

// ErrControlProjectNameConflict is returned when attempting to create an organization
// and the requested default project name collides with the org's automatically created
// control project.
var ErrControlProjectNameConflict = errors.New("project name conflicts with control project")

// ErrNotOrgMember is returned when attempting an org-scoped role assignment for a user who isn't
// a member of that organization.
var ErrNotOrgMember = errors.New("not a member of the organization")

// ErrRoleScopeConflict is returned when attempting a project-scoped role assignment for a
// user/role that already holds an org-scope assignment of the same role for that project's
// organization — the org-scope grant already covers every project under it, including this one.
var ErrRoleScopeConflict = errors.New("role already assigned at org scope")

// validationError wraps msg, a short field-level description, with ErrValidation.
func validationError(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrValidation)
}
