package pguser

import (
	"time"

	"github.com/google/uuid"
)

// OrderByField represents a field that user query results can be ordered by.
type OrderByField string

// Set of fields that user query results can be ordered by.
const (
	OrderByFieldEmail     OrderByField = "email"
	OrderByFieldCreatedAt OrderByField = "created_at"
	OrderByFieldUpdatedAt OrderByField = "updated_at"
)

// User represents a user in the database.
type User struct {
	ID              int        `db:"id"`
	ExternalID      uuid.UUID  `db:"external_id"`
	Email           string     `db:"email"`
	Name            string     `db:"name"`
	EmailVerifiedAt *time.Time `db:"email_verified_at"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       *time.Time `db:"updated_at"`
	ETag            uuid.UUID  `db:"etag"`
}

// CreateUser holds all fields required to create a new user in the database.
type CreateUser struct {
	Email string `db:"email"`
	Name  string `db:"name"`
}

// UpdateUser holds the fields to update on a user in the database.
// Fields controls which fields are applied; fields not listed are left unchanged.
type UpdateUser struct {
	ExternalID uuid.UUID
	Fields     UserUpdateFields
	Name       string
}

// UserUpdateFields specifies which fields on an UpdateUser should be applied.
type UserUpdateFields struct {
	Name bool
}

// Filter holds optional prefix-match criteria for listing users.
// Empty string fields are ignored.
type Filter struct {
	Email string
	Name  string
}
