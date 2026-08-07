package mdl

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user in the system.
type User struct {
	ID              uuid.UUID
	Email           string
	Name            string
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       *time.Time
	ETag            string
}

// CreateUser holds the fields needed to create a new user.
type CreateUser struct {
	Email string
	Name  string
}

func (cu CreateUser) Validate() error {
	if cu.Email == "" {
		return validationError("email required")
	}
	if !IsValidEmail(cu.Email) {
		return validationError("email invalid")
	}
	if cu.Name == "" {
		return validationError("name required")
	}
	return nil
}

// UpdateUser holds the fields that can be updated on a user.
// ID identifies the user to update and is not itself updated.
// Fields controls which fields are applied; fields not listed are left unchanged.
type UpdateUser struct {
	ID     uuid.UUID
	Fields UserUpdateFields
	Name   string
}

func (uu UpdateUser) Validate() error {
	if uu.Fields.Name && uu.Name == "" {
		return validationError("name required")
	}
	return nil
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
