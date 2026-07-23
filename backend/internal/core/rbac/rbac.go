// Package rbac provides the core business logic for the permissions and roles domain.
package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

//go:generate moq -rm -fmt goimports -out role_storer_moq_test.go . RoleStorer:MockedRoleStorer

// RoleStorer defines the database operations the Core requires.
type RoleStorer interface {
	// SystemRoles returns a page of system roles and their permissions.
	SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]pgrbac.SystemRole, error)
	// SystemRoleCount returns the number of system roles.
	SystemRoleCount(ctx context.Context) (int, error)
	// UserSystemRoles returns a page of system roles assigned to userID.
	UserSystemRoles(ctx context.Context, userID, pageSize, pageOffset int) ([]pgrbac.SystemRole, error)
	// UserSystemRoleCount returns the number of system roles assigned to userID.
	UserSystemRoleCount(ctx context.Context, userID int) (int, error)
	// AssignSystemRole grants userID the system role named roleName at system scope.
	// Returns [sql.ErrNoRows] if no system role named roleName exists.
	// Returns [pgdb.ErrAlreadyExists] if userID already has the system role.
	AssignSystemRole(ctx context.Context, userID int, roleName string) error
	// UnassignSystemRole revokes the system role named roleName from userID.
	// Returns [sql.ErrNoRows] if userID does not have that system role.
	UnassignSystemRole(ctx context.Context, userID int, roleName string) error
}

//go:generate moq -rm -fmt goimports -out user_storer_moq_test.go . UserStorer:MockedUserStorer

// UserStorer defines the user database operations the Core requires.
type UserStorer interface {
	// UserByExternalID returns the user with the given external ID.
	// Returns [sql.ErrNoRows] if no such user exists.
	UserByExternalID(ctx context.Context, id uuid.UUID) (pguser.User, error)
}

// Core holds the business logic for the permissions and roles domain.
type Core struct {
	roleStorer RoleStorer
	userStorer UserStorer
}

// NewCore constructs a Core backed by the provided RoleStorer and UserStorer.
func NewCore(rs RoleStorer, us UserStorer) *Core {
	return &Core{roleStorer: rs, userStorer: us}
}

// SystemRoles returns a page of system roles and their permissions, along with the total count.
func (c *Core) SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]mdl.SystemRole, int, error) {
	rs, err := c.roleStorer.SystemRoles(ctx, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("system roles: %w", err)
	}

	count, err := c.roleStorer.SystemRoleCount(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("system role count: %w", err)
	}

	return systemRolesFromPg(rs), count, nil
}

// UserSystemRoles returns a page of system roles assigned to userID, along with the total count.
// Returns [mdl.ErrNotFound] if no user with that ID exists.
func (c *Core) UserSystemRoles(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.SystemRole, int, error) {
	u, err := c.userStorer.UserByExternalID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("user by external id: %w", err)
	}

	rs, err := c.roleStorer.UserSystemRoles(ctx, u.ID, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("user system roles: %w", err)
	}

	count, err := c.roleStorer.UserSystemRoleCount(ctx, u.ID)
	if err != nil {
		return nil, 0, fmt.Errorf("user system role count: %w", err)
	}

	return systemRolesFromPg(rs), count, nil
}

// AssignSystemRole grants userID the system role named roleName at system scope.
// Returns [mdl.ErrNotFound] if no user with that ID, or no system role named roleName, exists.
// Returns [mdl.ErrAlreadyExists] if the user already has the system role.
func (c *Core) AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	u, err := c.userStorer.UserByExternalID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrNotFound
		}
		return fmt.Errorf("user by external id: %w", err)
	}

	if err := c.roleStorer.AssignSystemRole(ctx, u.ID, roleName); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return mdl.ErrNotFound
		case errors.Is(err, pgdb.ErrAlreadyExists):
			return mdl.ErrAlreadyExists
		}
		return fmt.Errorf("assign system role: %w", err)
	}

	return nil
}

// UnassignSystemRole revokes the system role named roleName from userID.
// Returns [mdl.ErrNotFound] if no user with that ID exists or the user does not have that system role.
func (c *Core) UnassignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	u, err := c.userStorer.UserByExternalID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrNotFound
		}
		return fmt.Errorf("user by external id: %w", err)
	}

	if err := c.roleStorer.UnassignSystemRole(ctx, u.ID, roleName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrNotFound
		}
		return fmt.Errorf("unassign system role: %w", err)
	}

	return nil
}
