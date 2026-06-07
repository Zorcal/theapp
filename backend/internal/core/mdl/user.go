package mdl

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user in the system.
type User struct {
	ID        uuid.UUID
	Email     string
	Name      string
	CreatedAt time.Time
	UpdatedAt *time.Time
	ETag      string
}

// CreateUser holds the fields required to create a new user.
type CreateUser struct {
	Email string
	Name  string
}

// UpdateUser holds the fields to update on a user.
// ID identifies the user to update and is not itself updated.
// Fields controls which fields are applied; fields not listed are left unchanged.
type UpdateUser struct {
	ID     uuid.UUID
	Fields UserUpdateFields
	Name   string
}

// UserUpdateFields specifies which fields on an UpdateUser should be applied.
type UserUpdateFields struct {
	Name bool
}

// UserOrderByField represents a field that user query results can be ordered by.
type UserOrderByField string

// Set of fields that user query results can be ordered by.
const (
	UserOrderByFieldEmail     UserOrderByField = "email"
	UserOrderByFieldUpdatedAt UserOrderByField = "updated_at"
	UserOrderByFieldCreatedAt UserOrderByField = "created_at"
)

// UserFilter holds optional prefix-match criteria for listing users.
// Empty string fields are ignored.
type UserFilter struct {
	Email string
	Name  string
}
