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

// validationError wraps msg, a short field-level description, with ErrValidation.
func validationError(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrValidation)
}
