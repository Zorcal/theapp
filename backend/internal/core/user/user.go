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

//go:generate moq -rm -fmt goimports -out user_storer_moq_test.go . UserStorer:MockedUserStorer RBACStorer:MockedRBACStorer

// UserStorer defines the database operations the Core requires.
type UserStorer interface {
	// SoftDeleteUser marks id as deleted.
	// Returns [sql.ErrNoRows] if id does not identify an active user.
	SoftDeleteUser(ctx context.Context, id uuid.UUID) error
	// RestoreUser restores id and returns it.
	// Returns [sql.ErrNoRows] if id does not identify a deleted user.
	RestoreUser(ctx context.Context, id uuid.UUID) (pguser.User, error)
	// UserByExternalID returns the active user with the given external ID.
	// Returns [sql.ErrNoRows] if no such active user exists.
	UserByExternalID(ctx context.Context, id uuid.UUID) (pguser.User, error)
	// UserByEmail returns the active user with the given email address.
	// Returns [sql.ErrNoRows] if no such active user exists.
	UserByEmail(ctx context.Context, email string) (pguser.User, error)
	Users(ctx context.Context, filter pguser.Filter, orderBys []order.By[pguser.OrderByField], pageSize, pageOffset int) ([]pguser.User, int, error)
	// CreateUser inserts a new user and returns it.
	// Returns [pgdb.ErrAlreadyExists] if a user with the same email already exists.
	CreateUser(ctx context.Context, cu pguser.CreateUser) (pguser.User, error)
	// UpdateUser updates the active user with the given external ID and returns the updated user.
	// Returns [sql.ErrNoRows] if no such active user exists.
	UpdateUser(ctx context.Context, uu pguser.UpdateUser) (pguser.User, error)
}

// RBACStorer defines the role and permission database operations the Core requires.
type RBACStorer interface {
	// LockSystemRoleManagement serializes changes that could remove system management access.
	LockSystemRoleManagement(ctx context.Context) error
	// FullyPrivilegedUserRemainsAfterDelete reports whether another active user holds every
	// registered permission through system-scope assignments.
	// Returns [sql.ErrNoRows] if id does not identify an active user.
	FullyPrivilegedUserRemainsAfterDelete(ctx context.Context, id uuid.UUID) (bool, error)
}

// Transactor runs a function inside a database transaction.
type Transactor interface {
	RunTx(ctx context.Context, fn func(context.Context) error) error
}

// Core holds the business logic for the user domain.
type Core struct {
	userStorer UserStorer
	rbacStorer RBACStorer
	transactor Transactor
}

// NewCore constructs a Core backed by the provided user, RBAC, and transaction stores.
func NewCore(us UserStorer, rs RBACStorer, tr Transactor) *Core {
	return &Core{userStorer: us, rbacStorer: rs, transactor: tr}
}

// UserByID returns the active user with the given ID.
// Returns [mdl.ErrNotFound] if no active user with that ID exists.
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

// UserByEmail returns the active user with the given email address.
// Returns [mdl.ErrNotFound] if no active user with that email exists.
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
		switch {
		case errors.Is(err, pgdb.ErrAlreadyExists):
			return mdl.User{}, mdl.ErrAlreadyExists
		case errors.Is(err, pguser.ErrDeleted):
			return mdl.User{}, mdl.ErrUserDeleted
		}
		return mdl.User{}, fmt.Errorf("create user: %w", err)
	}

	return userFromPg(pgUser), nil
}

// UpdateUser updates the name of the active user with the given ID and returns the updated user.
// Returns [mdl.ErrNotFound] if no active user with that ID exists.
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

// DeleteUser soft-deletes the user with the given ID.
// Returns [mdl.ErrNotFound] if no active user with that ID exists.
// Returns [mdl.ErrLastFullyPrivilegedSystemAdmin] if deletion would leave no fully privileged
// active system administrator.
func (c *Core) DeleteUser(ctx context.Context, id uuid.UUID) error {
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.rbacStorer.LockSystemRoleManagement(ctx); err != nil {
			return fmt.Errorf("lock system-role management: %w", err)
		}

		remains, err := c.rbacStorer.FullyPrivilegedUserRemainsAfterDelete(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("fully privileged user remains after delete: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("fully privileged user remains after delete: %w", err)
		}
		if !remains {
			return mdl.ErrLastFullyPrivilegedSystemAdmin
		}

		if err := c.userStorer.SoftDeleteUser(ctx, id); err != nil {
			// The target was established earlier in the transaction, so sql.ErrNoRows is an
			// impossible state that must remain an internal error.
			return fmt.Errorf("soft delete user: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return nil
}

// RestoreUser restores the soft-deleted user with the given ID and returns it.
// Returns [mdl.ErrNotFound] if no deleted user with that ID exists.
func (c *Core) RestoreUser(ctx context.Context, id uuid.UUID) (mdl.User, error) {
	pgUser, err := c.userStorer.RestoreUser(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.User{}, mdl.ErrNotFound
		}
		return mdl.User{}, fmt.Errorf("restore user: %w", err)
	}

	return userFromPg(pgUser), nil
}

// Users returns a page of active users matching filter ordered by orderBys, along with the total
// count of matching active users.
func (c *Core) Users(ctx context.Context, filter mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
	pgOrderBys, err := orderBysToPg(orderBys)
	if err != nil {
		return nil, 0, fmt.Errorf("convert order bys: %w", err)
	}

	pgFilter := filterToPg(filter)

	pgUsers, count, err := c.userStorer.Users(ctx, pgFilter, pgOrderBys, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}

	return usersFromPg(pgUsers), count, nil
}
