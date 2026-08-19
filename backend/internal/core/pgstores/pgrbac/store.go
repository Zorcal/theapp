// Package pgrbac provides role/permission db access functionality.
package pgrbac

import (
	"context"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// PermissionsByScope returns userID's resolved permission names for projectID at project,
// organization, and system scope in one database batch.
// Returns [sql.ErrNoRows] if no such user or project exists.
func (s *Store) PermissionsByScope(ctx context.Context, userID uuid.UUID, projectID int) (PermissionsByScope, error) {
	projectPermsQ := projectPermissionsQuery(userID, projectID)
	orgPermsQ := orgPermissionsByProjectIDQuery(userID, projectID)
	systemPermNamesQ := systemPermissionNamesQuery(userID)

	var projectPerms ProjectPermissions
	var orgPerms OrgPermissions
	var systemPermNames []string
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := projectPermsQ.Queue(ctx, b, &projectPerms); err != nil {
			return fmt.Errorf("permissions by scope project permissions: %w", err)
		}
		if err := orgPermsQ.Queue(ctx, b, &orgPerms); err != nil {
			return fmt.Errorf("permissions by scope organization permissions: %w", err)
		}
		if err := systemPermNamesQ.Queue(ctx, b, &systemPermNames); err != nil {
			return fmt.Errorf("permissions by scope system permissions: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return PermissionsByScope{}, err
	}

	return PermissionsByScope{
		ProjectPermissionNames: projectPerms.PermissionNames,
		OrgPermissionNames:     orgPerms.PermissionNames,
		SystemPermissionNames:  systemPermNames,
	}, nil
}

// ProjectPermissions returns projectID's org and the names of the permissions userID holds for
// projectID, resolved from project-, org-, and system-scope role assignments.
// Returns [sql.ErrNoRows] if no such user or project exists.
func (s *Store) ProjectPermissions(ctx context.Context, userID uuid.UUID, projectID int) (ProjectPermissions, error) {
	q := projectPermissionsQuery(userID, projectID)

	var perms ProjectPermissions
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

// OrgPermissionsByProjectID returns projectID's org and the names of the permissions userID holds
// there through organization- and system-scope role assignments.
// Returns [sql.ErrNoRows] if no such user or project exists.
func (s *Store) OrgPermissionsByProjectID(ctx context.Context, userID uuid.UUID, projectID int) (OrgPermissions, error) {
	q := orgPermissionsByProjectIDQuery(userID, projectID)

	var perms OrgPermissions
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &perms); err != nil {
			return fmt.Errorf("organization permissions by project id: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return OrgPermissions{}, err
	}

	return perms, nil
}
