package mdl

import (
	"time"

	"github.com/google/uuid"
	"github.com/zorcal/theapp/backend/internal/core/data/order"
)

// User represents a user in the system.
type User struct {
	ID        uuid.UUID
	Email     string
	CreatedAt time.Time
	UpdatedAt *time.Time
	ETag      string
}

// UserFilter holds the available fields a user query can be filtered on.
type UserFilter struct{}

// DefaultUserOrderBy represents the default way we sort users.
var DefaultUserOrderBy = []order.By[UserOrderByField]{
	{Field: UserOrderByFieldEmail, Direction: order.DirectionAsc},
	{Field: UserOrderByFieldUpdatedAt, Direction: order.DirectionDesc},
}

// UserOrderByField represents a field that user query results can be ordered by.
type UserOrderByField string

// Set of fields that user query results can be ordered by.
const (
	UserOrderByFieldEmail     UserOrderByField = "email"
	UserOrderByFieldUpdatedAt UserOrderByField = "updated_at"
)
