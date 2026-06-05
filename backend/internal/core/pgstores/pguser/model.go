package pguser

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user in the database.
type User struct {
	ExternalID uuid.UUID  `db:"external_id"`
	Email      string     `db:"email"`
	CreatedAt  time.Time  `db:"created_at"`
	UpdatedAt  *time.Time `db:"updated_at"`
	ETag       uuid.UUID  `db:"etag"`
}

// NewUser holds all fields required to create a new user in the database.
type NewUser struct {
	Email string `db:"email"`
}

// Filter specifies how users should be filtered.
type UserFilter struct{}
