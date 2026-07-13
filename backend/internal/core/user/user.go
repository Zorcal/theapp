// Package user provides the core business logic for the user domain.
package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

//go:generate moq -rm -fmt goimports -out user_storer_moq_test.go . UserStorer:MockedUserStorer

// UserStorer defines the database operations the Core requires.
type UserStorer interface {
	// UserByExternalID returns the user with the given external ID.
	// Returns [sql.ErrNoRows] if no such user exists.
	UserByExternalID(ctx context.Context, id uuid.UUID) (pguser.User, error)
	// UserByEmail returns the user with the given email address.
	// Returns [sql.ErrNoRows] if no such user exists.
	UserByEmail(ctx context.Context, email string) (pguser.User, error)
	Users(ctx context.Context, filter pguser.Filter, orderBys []order.By[pguser.OrderByField], pageSize, pageOffset int) ([]pguser.User, error)
	UserCount(ctx context.Context, filter pguser.Filter) (int, error)
	// CreateUser inserts a new user and returns it.
	// Returns [pgdb.ErrAlreadyExists] if a user with the same email already exists.
	CreateUser(ctx context.Context, cu pguser.CreateUser) (pguser.User, error)
	// UpdateUser updates the user with the given external ID and returns the updated user.
	// Returns [sql.ErrNoRows] if no such user exists.
	UpdateUser(ctx context.Context, uu pguser.UpdateUser) (pguser.User, error)
}

// Core holds the business logic for the user domain.
type Core struct {
	userStorer UserStorer
}

// NewCore constructs a Core backed by the provided UserStorer.
func NewCore(us UserStorer) *Core {
	return &Core{userStorer: us}
}

// UserByID returns the user with the given ID.
// Returns [mdl.ErrNotFound] if no user with that ID exists.
func (c *Core) UserByID(ctx context.Context, id uuid.UUID) (mdl.User, error) {
	pgUser, err := c.userStorer.UserByExternalID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.User{}, mdl.ErrNotFound
		}
		return mdl.User{}, fmt.Errorf("user by external id: %w", err)
	}

	return userFromPg(pgUser), nil
}

// UserByEmail returns the user with the given email address.
// Returns [mdl.ErrNotFound] if no user with that email exists.
func (c *Core) UserByEmail(ctx context.Context, email string) (mdl.User, error) {
	pgUser, err := c.userStorer.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.User{}, mdl.ErrNotFound
		}
		return mdl.User{}, fmt.Errorf("user by email: %w", err)
	}

	return userFromPg(pgUser), nil
}

// CreateUser creates a new user and returns the created user.
// Returns [mdl.ErrAlreadyExists] if a user with the same email already exists.
// Returns [mdl.ErrValidation] if cu is invalid.
func (c *Core) CreateUser(ctx context.Context, cu mdl.CreateUser) (mdl.User, error) {
	cu.Email = strings.ToLower(strings.TrimSpace(cu.Email))

	if err := cu.Validate(); err != nil {
		return mdl.User{}, fmt.Errorf("validate: %w", err)
	}

	pgCreateUser := createUserToPg(cu)

	pgUser, err := c.userStorer.CreateUser(ctx, pgCreateUser)
	if err != nil {
		if errors.Is(err, pgdb.ErrAlreadyExists) {
			return mdl.User{}, mdl.ErrAlreadyExists
		}
		return mdl.User{}, fmt.Errorf("create user: %w", err)
	}

	return userFromPg(pgUser), nil
}

// UpdateUser updates the name of the user with the given ID and returns the updated user.
// Returns [mdl.ErrNotFound] if no user with that ID exists.
// Returns [mdl.ErrValidation] if uu is invalid.
func (c *Core) UpdateUser(ctx context.Context, uu mdl.UpdateUser) (mdl.User, error) {
	if err := uu.Validate(); err != nil {
		return mdl.User{}, fmt.Errorf("validate: %w", err)
	}

	pgUpdateUser := updateUserToPg(uu)

	pgUser, err := c.userStorer.UpdateUser(ctx, pgUpdateUser)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.User{}, mdl.ErrNotFound
		}
		return mdl.User{}, fmt.Errorf("update user: %w", err)
	}

	return userFromPg(pgUser), nil
}

// Users returns a page of users matching filter ordered by orderBys, along with the total count of matching users.
func (c *Core) Users(ctx context.Context, filter mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
	pgOrderBys, err := orderBysToPg(orderBys)
	if err != nil {
		return nil, 0, fmt.Errorf("convert order bys: %w", err)
	}

	pgFilter := filterToPg(filter)

	pgUsers, err := c.userStorer.Users(ctx, pgFilter, pgOrderBys, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}

	count, err := c.userStorer.UserCount(ctx, pgFilter)
	if err != nil {
		return nil, 0, fmt.Errorf("user count: %w", err)
	}

	return usersFromPg(pgUsers), count, nil
}
