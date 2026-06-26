package mdl

import "errors"

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
