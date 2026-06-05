// Package user provides the core business logic for the user domain.
package user

import (
	"context"
	"fmt"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/order"
)

//go:generate moq -rm -fmt goimports -out storer_moq_test.go . Storer:MockedStorer

// Storer defines the database operations the Core requires.
type Storer interface {
	QueryUsers(ctx context.Context, orderBys []order.By[pguser.OrderByField], pageSize, pageOffset int) ([]pguser.User, error)
	UserCount(ctx context.Context) (int, error)
	InsertUser(ctx context.Context, cu pguser.CreateUser) (pguser.User, error)
}

// Core holds the business logic for the user domain.
type Core struct {
	storer Storer
}

// NewCore constructs a Core backed by the provided Storer.
func NewCore(storer Storer) *Core {
	return &Core{storer: storer}
}

// CreateUser creates a new user and returns the created user.
func (c *Core) CreateUser(ctx context.Context, cu mdl.CreateUser) (mdl.User, error) {
	pgCreateUser := createUserToPG(cu)

	pgUser, err := c.storer.InsertUser(ctx, pgCreateUser)
	if err != nil {
		return mdl.User{}, fmt.Errorf("insert user: %w", err)
	}

	return userFromPG(pgUser), nil
}

// ListUsers returns a page of users ordered by orderBys, along with the total count of users in the system.
func (c *Core) ListUsers(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
	pgOrderBys, err := orderBysToPG(orderBys)
	if err != nil {
		return nil, 0, fmt.Errorf("convert order bys: %w", err)
	}

	pgUsers, err := c.storer.QueryUsers(ctx, pgOrderBys, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}

	count, err := c.storer.UserCount(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("user count: %w", err)
	}

	return usersFromPG(pgUsers), count, nil
}
