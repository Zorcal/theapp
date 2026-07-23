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
)

//go:generate moq -rm -fmt goimports -out role_storer_moq_test.go . RoleStorer:MockedRoleStorer

// RoleStorer defines the database operations the Core requires.
type RoleStorer interface {
	// SystemRoles returns every system role and the names of the permissions currently granted to
	// it.
	SystemRoles(ctx context.Context) ([]pgrbac.SystemRole, error)
	// AssignSystemRole grants userID the system role named roleName at system scope.
	// Returns [sql.ErrNoRows] if no system role named roleName exists.
	AssignSystemRole(ctx context.Context, userID int, roleName string) error
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

// SystemRoles returns every system role and the permissions currently granted to it.
func (c *Core) SystemRoles(ctx context.Context) ([]mdl.SystemRole, error) {
	rs, err := c.roleStorer.SystemRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("system roles: %w", err)
	}

	return systemRolesFromPg(rs), nil
}

// AssignSystemRole grants userID the system role named roleName at system scope.
// Returns [mdl.ErrNotFound] if no user with that ID, or no system role named roleName, exists.
func (c *Core) AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	u, err := c.userStorer.UserByExternalID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrNotFound
		}
		return fmt.Errorf("user by external id: %w", err)
	}

	if err := c.roleStorer.AssignSystemRole(ctx, u.ID, roleName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrNotFound
		}
		return fmt.Errorf("assign system role: %w", err)
	}

	return nil
}
