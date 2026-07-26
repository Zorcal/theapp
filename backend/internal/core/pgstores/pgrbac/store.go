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

// ProjectPermissions returns projectID's org and the names of the permissions userID holds for
// projectID, resolved from project-, org-, and system-scope role assignments.
// Returns [sql.ErrNoRows] if no such project exists.
func (s *Store) ProjectPermissions(ctx context.Context, userID, projectID int) (ProjectPermissions, error) {
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
