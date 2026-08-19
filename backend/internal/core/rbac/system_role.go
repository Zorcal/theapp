package rbac

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

// SystemRoles returns a page of system roles, along with the total count.
func (c *Core) SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]mdl.SystemRole, int, error) {
	rs, count, err := c.roleStorer.SystemRoles(ctx, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("system roles: %w", err)
	}

	return systemRolesFromPg(rs), count, nil
}

// UserSystemRoles returns a page of system roles assigned to userID, along with the total count.
// Returns [mdl.ErrNotFound] if no user with that ID exists.
func (c *Core) UserSystemRoles(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.SystemRole, int, error) {
	rs, count, err := c.roleStorer.UserSystemRolesByExternalID(ctx, userID, pageSize, pageOffset)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("user system roles: %w", err)
	}

	return systemRolesFromPg(rs), count, nil
}

// AssignSystemRole grants targetUserID the system role named roleName at system scope.
// The actor is read from the auth session in ctx.
// Returns [mdl.ErrNotFound] if the actor, target user, or system role does not exist.
// Returns [mdl.ErrPermissionDenied] if the actor's system-scope permissions are not a superset of the role's.
// Returns [mdl.ErrAlreadyExists] if the target user already has the system role.
func (c *Core) AssignSystemRole(ctx context.Context, targetUserID uuid.UUID, roleName string) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.lockSystemRoleUsers(ctx, sess.User.UserID, targetUserID); err != nil {
			return fmt.Errorf("lock users: %w", err)
		}

		if _, err := c.authorizeSystemRoleChange(ctx, sess.User.UserID, roleName); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("authorize system role change: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("authorize system role change: %w", err)
		}

		if err := c.roleStorer.AssignSystemRole(ctx, targetUserID, roleName); err != nil {
			switch {
			case errors.Is(err, sql.ErrNoRows):
				return fmt.Errorf("assign system role: %w", mdl.ErrNotFound)
			case errors.Is(err, pgdb.ErrAlreadyExists):
				return fmt.Errorf("assign system role: %w", mdl.ErrAlreadyExists)
			default:
				return fmt.Errorf("assign system role: %w", err)
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// UnassignSystemRole revokes the system role named roleName from targetUserID.
// The actor is read from the auth session in ctx.
// Returns [mdl.ErrNotFound] if the actor, target user, role, or assignment does not exist.
// Returns [mdl.ErrPermissionDenied] if the actor's system-scope permissions are not a superset of the role's.
// Returns [mdl.ErrLastFullyPrivilegedSystemAdmin] if the change would leave no fully privileged
// system administrator.
func (c *Core) UnassignSystemRole(ctx context.Context, targetUserID uuid.UUID, roleName string) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
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
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("authorize system role change: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("authorize system role change: %w", err)
		}

		if err := c.ensureFullyPrivilegedSystemUserRemains(ctx, targetUserID, role.Name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("ensure fully privileged system user remains: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("ensure fully privileged system user remains: %w", err)
		}

		if err := c.roleStorer.UnassignSystemRole(ctx, targetUserID, roleName); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unassign system role: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("unassign system role: %w", err)
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
			switch {
			case errors.Is(err, sql.ErrNoRows):
				return fmt.Errorf("assign system role: %w", mdl.ErrNotFound)
			case errors.Is(err, pgdb.ErrAlreadyExists):
				return fmt.Errorf("assign system role: %w", mdl.ErrAlreadyExists)
			default:
				return fmt.Errorf("assign system role: %w", err)
			}
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
		return pgrbac.SystemRole{}, fmt.Errorf("system role: %w", err)
	}

	actorPerms, err := c.roleStorer.SystemPermissions(ctx, actorUserID)
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

// ensureFullyPrivilegedSystemUserRemains rejects a revoke that would leave no user holding every
// registered permission at system scope. It must run inside the write transaction after the
// management and target user locks are acquired.
func (c *Core) ensureFullyPrivilegedSystemUserRemains(ctx context.Context, targetUserID uuid.UUID, roleName string) error {
	hasFullyPrivilegedUser, err := c.roleStorer.FullyPrivilegedUserRemainsAfterSystemRoleUnassign(ctx, targetUserID, roleName)
	if err != nil {
		return fmt.Errorf("fully privileged system user after unassign: %w", err)
	}

	if !hasFullyPrivilegedUser {
		return mdl.ErrLastFullyPrivilegedSystemAdmin
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
