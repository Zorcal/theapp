// Package dbostest provides DBOS workflow test helpers.
package dbostest

import (
	"context"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/testingx"
)

// shutdownTimeout bounds how long Launch's cleanup waits for in-flight workflow steps to finish.
const shutdownTimeout = 5 * time.Second

// New constructs a DBOS context backed by pool's database. It uses DatabaseURL rather than SystemDBPool so DBOS
// opens its own connection pool: pgtest pools are capped at MaxConns: 1, and sharing that single connection with
// DBOS's own polling would deadlock.
//
// AppName is set to t.Name() purely for identification in logs — DBOS doesn't use it for schema naming or
// isolation, so this has no effect on test correctness, only on being able to tell which test a log line came from.
//
// Register all workflows against the returned context, then call Launch.
func New(t *testing.T, ctx context.Context, pool *pgxpool.Pool) dbos.DBOSContext {
	t.Helper()

	dbosCtx, err := dbos.NewDBOSContext(ctx, dbos.Config{
		AppName:     t.Name(),
		DatabaseURL: pool.Config().ConnString(),
		Logger:      testingx.NewLogger(t),
	})
	if err != nil {
		t.Fatalf("dbostest.New: %v", err)
	}
	return dbosCtx
}

// Launch starts dbosCtx and registers a cleanup that shuts it down. Call it only after registering all workflows
// on dbosCtx — DBOS rejects workflow registration once launched.
func Launch(t *testing.T, dbosCtx dbos.DBOSContext) {
	t.Helper()

	if err := dbos.Launch(dbosCtx); err != nil {
		t.Fatalf("dbostest.Launch: %v", err)
	}
	t.Cleanup(func() { dbos.Shutdown(dbosCtx, shutdownTimeout) })
}
