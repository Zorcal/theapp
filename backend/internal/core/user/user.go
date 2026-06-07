// Package user provides the core business logic for the user domain.
package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/order"
)

//go:generate moq -rm -fmt goimports -out storer_moq_test.go . Storer:MockedStorer

// Storer defines the database operations the Core requires.
type Storer interface {
	UserByExternalID(ctx context.Context, id uuid.UUID) (pguser.User, error)
	Users(ctx context.Context, orderBys []order.By[pguser.OrderByField], pageSize, pageOffset int) ([]pguser.User, error)
	UserCount(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, cu pguser.CreateUser) (pguser.User, error)
}

// Core holds the business logic for the user domain.
type Core struct {
	storer Storer
}

// NewCore constructs a Core backed by the provided Storer.
func NewCore(storer Storer) *Core {
	return &Core{storer: storer}
}

// UserByID retrieves a user by ID. Returns mdl.ErrNotFound if the user does not exist.
func (c *Core) UserByID(ctx context.Context, id uuid.UUID) (mdl.User, error) {
	pgUser, err := c.storer.UserByExternalID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.User{}, mdl.ErrNotFound
		}
		return mdl.User{}, fmt.Errorf("user by external id: %w", err)
	}

	return userFromPG(pgUser), nil
}

// CreateUser creates a new user and returns the created user.
func (c *Core) CreateUser(ctx context.Context, cu mdl.CreateUser) (mdl.User, error) {
	pgCreateUser := createUserToPG(cu)

	pgUser, err := c.storer.CreateUser(ctx, pgCreateUser)
	if err != nil {
		return mdl.User{}, fmt.Errorf("create user: %w", err)
	}

	return userFromPG(pgUser), nil
}

// Users returns a page of users ordered by orderBys, along with the total count of users in the system.
func (c *Core) Users(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
	pgOrderBys, err := orderBysToPG(orderBys)
	if err != nil {
		return nil, 0, fmt.Errorf("convert order bys: %w", err)
	}

	pgUsers, err := c.storer.Users(ctx, pgOrderBys, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}

	count, err := c.storer.UserCount(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("user count: %w", err)
	}

	return usersFromPG(pgUsers), count, nil
}
