package pgrbac

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

// LockSystemRoleManagement serializes system-role revokes that could remove management access.
// It must be called within a transaction.
func (s *Store) LockSystemRoleManagement(ctx context.Context) error {
	if err := pgdb.RunExec(ctx, s.pool, "SELECT pg_advisory_xact_lock(hashtext('rbac.system-role-management'), 0)"); err != nil {
		return fmt.Errorf("lock system-role management: %w", err)
	}

	return nil
}

// LockSystemRoleUser acquires a transaction-level advisory lock that serializes system-role
// assignment changes for userID. It must be called within a transaction.
func (s *Store) LockSystemRoleUser(ctx context.Context, userID uuid.UUID) error {
	const query = `
		SELECT pg_advisory_xact_lock(hashtext('rbac.system-role-user'), id)
		FROM useraccess.users
		WHERE external_id = $1`

	if err := pgdb.RunExec(ctx, s.pool, query, userID); err != nil {
		return fmt.Errorf("lock system-role user: %w", err)
	}

	return nil
}

// SystemRoles returns a page of system roles and their permissions, ordered by role name, along
// with the total count.
func (s *Store) SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]SystemRole, int, error) {
	rolesQ := systemRolesQuery(pageSize, pageOffset)
	countQ := systemRoleCountQuery()

	var (
		roles []SystemRole
		count int
	)
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := rolesQ.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("system roles: %w", err)
		}
		if err := countQ.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("system role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, 0, err
	}

	return roles, count, nil
}

// SystemRoleByName returns the system role named name and its permissions.
// Returns [sql.ErrNoRows] if no such system role exists.
func (s *Store) SystemRoleByName(ctx context.Context, name string) (SystemRole, error) {
	q := systemRoleByNameQuery(name)

	var role SystemRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("system role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return SystemRole{}, err
	}

	return role, nil
}

// UserSystemRolesByExternalID returns a page of system roles assigned to userID, ordered by role
// name, along with the total count.
// Returns [sql.ErrNoRows] if no such user exists.
func (s *Store) UserSystemRolesByExternalID(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]SystemRole, int, error) {
	rolesQ := userSystemRolesByExternalIDQuery(userID, pageSize, pageOffset)
	countQ := userSystemRoleCountByExternalIDQuery(userID)

	var (
		roles []SystemRole
		count int
	)
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := rolesQ.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("user system roles: %w", err)
		}
		if err := countQ.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("user system role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, 0, err
	}

	return roles, count, nil
}

// SystemPermissions returns the names of the permissions userID holds through system-scope role assignments only.
// Returns [sql.ErrNoRows] if no such user exists.
func (s *Store) SystemPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	q := systemPermissionNamesQuery(userID)

	var names []string
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &names); err != nil {
			return fmt.Errorf("system permissions: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return names, nil
}

// FullyPrivilegedUserRemainsAfterSystemRoleUnassign reports whether at least one user will hold
// every registered permission through the remaining system-role assignments.
// Returns [sql.ErrNoRows] if the assignment does not exist.
func (s *Store) FullyPrivilegedUserRemainsAfterSystemRoleUnassign(ctx context.Context, userID uuid.UUID, roleName string) (bool, error) {
	q := fullyPrivilegedUserRemainsAfterSystemRoleUnassignQuery(userID, roleName)

	var hasFullyPrivilegedUser bool
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &hasFullyPrivilegedUser); err != nil {
			return fmt.Errorf("fully privileged system user remains after unassign: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return false, err
	}

	return hasFullyPrivilegedUser, nil
}

// AssignSystemRole grants userID the system role named roleName at system scope.
// Returns [sql.ErrNoRows] if no user with that ID or system role named roleName exists.
// Returns [pgdb.ErrAlreadyExists] if userID already has the system role.
func (s *Store) AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	q := assignSystemRoleQuery(userID, roleName)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		// The ID is only a result sink so ExpectOne returns sql.ErrNoRows when no role was assigned.
		var roleIDSink int
		if err := q.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("assign system role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

// UnassignSystemRole revokes the system role named roleName from userID.
// Returns [sql.ErrNoRows] if userID does not have that system role or no such user exists.
func (s *Store) UnassignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	q := unassignSystemRoleQuery(userID, roleName)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		// The ID is only a result sink so ExpectOne returns sql.ErrNoRows when no role was unassigned.
		var roleIDSink int
		if err := q.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("unassign system role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}
