// Package pgrbac provides role/permission db access functionality.
package pgrbac

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
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

// SystemRoles returns a page of system roles and their permissions, ordered by role name.
func (s *Store) SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]SystemRole, error) {
	var roles []SystemRole

	q := systemRolesQuery(pageSize, pageOffset)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("system roles: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return roles, nil
}

// SystemRoleByName returns the system role named name and its permissions.
// Returns [sql.ErrNoRows] if no such system role exists.
func (s *Store) SystemRoleByName(ctx context.Context, name string) (SystemRole, error) {
	var role SystemRole

	q := systemRoleByNameQuery(name)

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

// SystemRoleCount returns the number of system roles.
func (s *Store) SystemRoleCount(ctx context.Context) (int, error) {
	var count int

	q := systemRoleCountQuery()

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("system role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return 0, err
	}

	return count, nil
}

// UserSystemRolesByExternalID returns a page of system roles assigned to userID, ordered by role name.
func (s *Store) UserSystemRolesByExternalID(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]SystemRole, error) {
	var roles []SystemRole

	q := userSystemRolesByExternalIDQuery(userID, pageSize, pageOffset)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("user system roles: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return roles, nil
}

// UserSystemRoleCountByExternalID returns the number of system roles assigned to userID.
// Returns [sql.ErrNoRows] if no such user exists.
func (s *Store) UserSystemRoleCountByExternalID(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int

	q := userSystemRoleCountByExternalIDQuery(userID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("user system role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return 0, err
	}

	return count, nil
}

// UserSystemPermissionsByExternalID returns the names of the permissions userID holds through
// system-scope role assignments only.
func (s *Store) UserSystemPermissionsByExternalID(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var names []string

	q := userSystemPermissionsByExternalIDQuery(userID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &names); err != nil {
			return fmt.Errorf("system permissions: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return names, nil
}

// ProjectPermissions returns projectID's org and the names of the permissions userID holds for
// projectID, resolved from project-, org-, and system-scope role assignments.
// Returns [sql.ErrNoRows] if no such project exists.
func (s *Store) ProjectPermissions(ctx context.Context, userID, projectID int) (ProjectPermissions, error) {
	var perms ProjectPermissions

	q := projectPermissionsQuery(userID, projectID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &perms); err != nil {
			return fmt.Errorf("project permissions: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return ProjectPermissions{}, err
	}

	return perms, nil
}

// AssignSystemRole grants userID the system role named roleName at system scope.
// Returns [sql.ErrNoRows] if no user with that ID or system role named roleName exists.
// Returns [pgdb.ErrAlreadyExists] if userID already has the system role.
func (s *Store) AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	var roleID int

	q := assignSystemRoleQuery(userID, roleName)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &roleID); err != nil {
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
	var roleID int

	q := unassignSystemRoleQuery(userID, roleName)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &roleID); err != nil {
			return fmt.Errorf("unassign system role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}
