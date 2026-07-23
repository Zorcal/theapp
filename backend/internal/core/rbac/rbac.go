// Package rbac provides the core business logic for the permissions and roles domain.
package rbac

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/pkg/set"
)

//go:generate moq -rm -fmt goimports -out role_storer_moq_test.go . RoleStorer:MockedRoleStorer

// RoleStorer defines the database operations the Core requires.
type RoleStorer interface {
	// LockSystemRoleManagement serializes system-role revokes that could remove management access.
	LockSystemRoleManagement(ctx context.Context) error
	// LockSystemRoleUser acquires a transaction-level lock that serializes system-role assignment
	// changes for userID.
	LockSystemRoleUser(ctx context.Context, userID uuid.UUID) error
	// SystemRoles returns a page of system roles and their permissions.
	SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]pgrbac.SystemRole, error)
	// SystemRoleByName returns the system role named name and its permissions.
	// Returns [sql.ErrNoRows] if no such system role exists.
	SystemRoleByName(ctx context.Context, name string) (pgrbac.SystemRole, error)
	// SystemRoleCount returns the number of system roles.
	SystemRoleCount(ctx context.Context) (int, error)
	// UserSystemRolesByExternalID returns a page of system roles assigned to userID.
	UserSystemRolesByExternalID(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]pgrbac.SystemRole, error)
	// UserSystemRoleCountByExternalID returns the number of system roles assigned to userID.
	// Returns [sql.ErrNoRows] if no such user exists.
	UserSystemRoleCountByExternalID(ctx context.Context, userID uuid.UUID) (int, error)
	// UserSystemPermissionsByExternalID returns the names of the permissions userID holds through
	// system-role assignments.
	UserSystemPermissionsByExternalID(ctx context.Context, userID uuid.UUID) ([]string, error)
	// SystemPermissionsRemainAfterUnassign reports whether every permission in permissionNames is
	// carried by another system-role assignment.
	// Returns [sql.ErrNoRows] if the assignment does not exist.
	SystemPermissionsRemainAfterUnassign(ctx context.Context, userID uuid.UUID, roleName string, permissionNames []string) (bool, error)
	// AssignSystemRole grants userID the system role named roleName at system scope.
	// Returns [sql.ErrNoRows] if no user with that ID or system role named roleName exists.
	// Returns [pgdb.ErrAlreadyExists] if userID already has the system role.
	AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error
	// UnassignSystemRole revokes the system role named roleName from userID.
	// Returns [sql.ErrNoRows] if userID does not have that system role or no such user exists.
	UnassignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error
}

// Transactor runs a function inside a database transaction.
type Transactor interface {
	RunTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Core holds the business logic for the permissions and roles domain.
type Core struct {
	roleStorer RoleStorer
	transactor Transactor
}

// NewCore constructs a Core backed by the provided role store and transactor.
func NewCore(rs RoleStorer, tr Transactor) *Core {
	return &Core{roleStorer: rs, transactor: tr}
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
	rs, err := c.roleStorer.UserSystemRolesByExternalID(ctx, userID, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("user system roles: %w", err)
	}

	count, err := c.roleStorer.UserSystemRoleCountByExternalID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("user system role count: %w", err)
	}

	return systemRolesFromPg(rs), count, nil
}

// AssignSystemRole grants targetUserID the system role named roleName at system scope.
// The actor is read from the auth session in ctx.
// Returns [mdl.ErrNotFound] if the target user or system role does not exist.
// Returns [mdl.ErrPermissionDenied] if the actor's system-scope permissions are not a superset of the role's.
// Returns [mdl.ErrAlreadyExists] if the target user already has the system role.
func (c *Core) AssignSystemRole(ctx context.Context, targetUserID uuid.UUID, roleName string) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("assign system role: auth session missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.lockSystemRoleUsers(ctx, sess.User.UserID, targetUserID); err != nil {
			return fmt.Errorf("lock users: %w", err)
		}

		if _, err := c.authorizeSystemRoleChange(ctx, sess.User.UserID, roleName); err != nil {
			return fmt.Errorf("authorize system role change: %w", err)
		}

		if err := c.roleStorer.AssignSystemRole(ctx, targetUserID, roleName); err != nil {
			return fmt.Errorf("assign system role: %w", handleAssignmentError(err))
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// UnassignSystemRole revokes the system role named roleName from targetUserID.
// The actor is read from the auth session in ctx.
// Returns [mdl.ErrNotFound] if the target user, role, or assignment does not exist.
// Returns [mdl.ErrPermissionDenied] if the actor's system-scope permissions are not a superset of the role's.
// Returns [mdl.ErrLastRoleManager] if the change would remove the last system-role management assignment.
func (c *Core) UnassignSystemRole(ctx context.Context, targetUserID uuid.UUID, roleName string) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("unassign system role: auth session missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.roleStorer.LockSystemRoleManagement(ctx); err != nil {
			return fmt.Errorf("lock system role management: %w", err)
		}

		if err := c.lockSystemRoleUsers(ctx, sess.User.UserID, targetUserID); err != nil {
			return fmt.Errorf("lock users: %w", err)
		}

		role, err := c.authorizeSystemRoleChange(ctx, sess.User.UserID, roleName)
		if err != nil {
			return fmt.Errorf("authorize system role change: %w", err)
		}

		if err := c.ensureSystemManagementAccessRemains(ctx, targetUserID, role); err != nil {
			return fmt.Errorf("ensure system management access remains: %w", err)
		}

		if err := c.roleStorer.UnassignSystemRole(ctx, targetUserID, roleName); err != nil {
			return fmt.Errorf("unassign system role: %w", handleAssignmentError(err))
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// BootstrapAssignSystemRole grants userID a system role without an actor permission check.
// It is reserved for the bootstrap CLI, which must be able to establish the first system administrator.
func (c *Core) BootstrapAssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.roleStorer.LockSystemRoleUser(ctx, userID); err != nil {
			return fmt.Errorf("lock user: %w", err)
		}

		if err := c.roleStorer.AssignSystemRole(ctx, userID, roleName); err != nil {
			return fmt.Errorf("assign system role: %w", handleAssignmentError(err))
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// authorizeSystemRoleChange verifies that the actor may change assignments for roleName.
// It must run inside the write transaction after the actor and target user locks are acquired.
func (c *Core) authorizeSystemRoleChange(ctx context.Context, actorUserID uuid.UUID, roleName string) (pgrbac.SystemRole, error) {
	// Resolve both sides of the superset check after locking the actor and target. Every
	// assignment change takes the same per-user locks, so the actor's authority cannot change
	// between this check and the target's update.
	role, err := c.roleStorer.SystemRoleByName(ctx, roleName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return pgrbac.SystemRole{}, mdl.ErrNotFound
		}
		return pgrbac.SystemRole{}, fmt.Errorf("system role: %w", err)
	}

	actorPerms, err := c.roleStorer.UserSystemPermissionsByExternalID(ctx, actorUserID)
	if err != nil {
		return pgrbac.SystemRole{}, fmt.Errorf("actor system permissions: %w", err)
	}

	// Requiring every permission carried by the role prevents the actor from granting or
	// revoking authority they do not hold themselves.
	if !mdl.IsPermissionSuperset(permissionsFromPg(actorPerms), permissionsFromPg(role.PermissionNames)) {
		return pgrbac.SystemRole{}, mdl.ErrPermissionDenied
	}

	return role, nil
}

// ensureSystemManagementAccessRemains rejects a revoke that would remove the last system-role
// management permission. It must run inside the write transaction after the management and target
// user locks are acquired.
func (c *Core) ensureSystemManagementAccessRemains(ctx context.Context, targetUserID uuid.UUID, role pgrbac.SystemRole) error {
	rolePerms := set.FromSlice(permissionsFromPg(role.PermissionNames))
	managementPerms := set.FromSlice(mdl.SystemRoleManagementPermissions())
	removedPerms := rolePerms.Intersection(managementPerms)
	if removedPerms.Len() == 0 {
		return nil
	}

	remain, err := c.roleStorer.SystemPermissionsRemainAfterUnassign(ctx, targetUserID, role.Name, permissionsToPg(removedPerms.Values()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrNotFound
		}
		return fmt.Errorf("system management permissions after unassign: %w", err)
	}

	if !remain {
		return mdl.ErrLastRoleManager
	}

	return nil
}

// lockSystemRoleUsers locks the actor and target in ascending UUID byte order so concurrent
// changes acquire shared locks consistently. It must run inside the write transaction.
func (c *Core) lockSystemRoleUsers(ctx context.Context, actorUserID, targetUserID uuid.UUID) error {
	firstUserID, secondUserID := actorUserID, targetUserID
	if bytes.Compare(firstUserID[:], secondUserID[:]) > 0 {
		firstUserID, secondUserID = secondUserID, firstUserID
	}

	if err := c.roleStorer.LockSystemRoleUser(ctx, firstUserID); err != nil {
		return fmt.Errorf("lock first user: %w", err)
	}
	if firstUserID != secondUserID {
		if err := c.roleStorer.LockSystemRoleUser(ctx, secondUserID); err != nil {
			return fmt.Errorf("lock second user: %w", err)
		}
	}

	return nil
}

func handleAssignmentError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return mdl.ErrNotFound
	case errors.Is(err, pgdb.ErrAlreadyExists):
		return mdl.ErrAlreadyExists
	default:
		return err
	}
}
