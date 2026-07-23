// Package pgrbac provides role/permission db access functionality.
package pgrbac

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// SystemRoles returns every system role and the names of the permissions currently granted to
// it, ordered by role name.
func (s *Store) SystemRoles(ctx context.Context) ([]SystemRole, error) {
	var roles []SystemRole

	q := systemRolesQuery()

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

// SystemPermissions returns the names of the permissions userID holds through system-scope role
// assignments only.
func (s *Store) SystemPermissions(ctx context.Context, userID int) ([]string, error) {
	var names []string

	q := systemPermissionsQuery(userID)

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
// Returns [sql.ErrNoRows] if no system role named roleName exists.
func (s *Store) AssignSystemRole(ctx context.Context, userID int, roleName string) error {
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
