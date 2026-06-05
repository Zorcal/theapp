package mdl

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user in the system.
type User struct {
	ID        uuid.UUID
	Email     string
	CreatedAt time.Time
	UpdatedAt *time.Time
	ETag      string
}

// UserOrderByField represents a field that user query results can be ordered by.
type UserOrderByField string

// Set of fields that user query results can be ordered by.
const (
	UserOrderByFieldEmail     UserOrderByField = "email"
	UserOrderByFieldUpdatedAt UserOrderByField = "updated_at"
	UserOrderByFieldCreatedAt UserOrderByField = "created_at"
)
