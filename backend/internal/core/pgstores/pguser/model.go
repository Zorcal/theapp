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
	ExternalID uuid.UUID  `db:"external_id"`
	Email      string     `db:"email"`
	CreatedAt  time.Time  `db:"created_at"`
	UpdatedAt  *time.Time `db:"updated_at"`
	ETag       uuid.UUID  `db:"etag"`
}

// CreateUser holds all fields required to create a new user in the database.
type CreateUser struct {
	Email string `db:"email"`
}
