// Package rbac provides the core business logic for the permissions and roles domain.
package rbac

import (
	"context"
	"fmt"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
)

//go:generate moq -rm -fmt goimports -out role_storer_moq_test.go . RoleStorer:MockedRoleStorer

// RoleStorer defines the database operations the Core requires.
type RoleStorer interface {
	// Roles returns every role and the names of the permissions currently granted to it.
	Roles(ctx context.Context) ([]pgrbac.Role, error)
}

// Core holds the business logic for the permissions and roles domain.
type Core struct {
	roleStorer RoleStorer
}

// NewCore constructs a Core backed by the provided RoleStorer.
func NewCore(rs RoleStorer) *Core {
	return &Core{roleStorer: rs}
}

// Roles returns every role and the permissions currently granted to it.
func (c *Core) Roles(ctx context.Context) ([]mdl.Role, error) {
	rs, err := c.roleStorer.Roles(ctx)
	if err != nil {
		return nil, fmt.Errorf("roles: %w", err)
	}

	return rolesFromPg(rs), nil
}
