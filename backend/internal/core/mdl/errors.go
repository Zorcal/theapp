package mdl

import "errors"

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// ErrTokenInvalid is returned when a magic-link or refresh token is expired,
// consumed, revoked, or not found. A single sentinel avoids leaking which
// condition applies.
var ErrTokenInvalid = errors.New("token invalid")
