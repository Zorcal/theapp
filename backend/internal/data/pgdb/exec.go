package pgdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/telemetry"
)

// RunExec executes a single statement, joining the transaction from ctx when one
// is present, or using the pool directly otherwise.
func RunExec(ctx context.Context, p *pgxpool.Pool, sql string, args ...any) error {
	ctx, span := telemetry.StartSpan(ctx, "pgdb.RunExec")
	defer span.End()

	if tx := txFromCtx(ctx); tx != nil {
		_, err := tx.Exec(ctx, sql, args...)
		return translatePgErr(err)
	}
	_, err := p.Exec(ctx, sql, args...)
	return translatePgErr(err)
}
