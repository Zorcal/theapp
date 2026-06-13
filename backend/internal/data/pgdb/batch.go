package pgdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/telemetry"
)

// Batch wraps a pgx.Batch. It is used to queue multiple database operations
// and execute them together as a single batch.
type Batch struct {
	b *pgx.Batch
}

// RunBatch creates a new Batch, passes it to f for query queueing, and then
// executes the batch against the provided pool.
//
// If f returns an error, the batch is not sent. If sending or closing the
// batch results fails, RunBatch returns an error.
func RunBatch(ctx context.Context, p *pgxpool.Pool, queueFunc func(ctx context.Context, b *Batch) error) error {
	ctx, span := telemetry.StartSpan(ctx, "pgdb.RunBatch")
	defer span.End()

	b := &Batch{
		b: &pgx.Batch{},
	}

	if err := queueFunc(ctx, b); err != nil {
		return fmt.Errorf("queueFunc: %w", err)
	}

	var result pgx.BatchResults
	if tx := txFromCtx(ctx); tx != nil {
		result = tx.SendBatch(ctx, b.b)
	} else {
		result = p.SendBatch(ctx, b.b)
	}
	if err := result.Close(); err != nil {
		return fmt.Errorf("close batch result: %w", err)
	}

	return nil
}

// RunExec executes a single statement, joining the transaction from ctx when one
// is present, or using the pool directly otherwise.
func RunExec(ctx context.Context, p *pgxpool.Pool, sql string, args ...any) error {
	ctx, span := telemetry.StartSpan(ctx, "pgdb.RunExec")
	defer span.End()

	if tx := txFromCtx(ctx); tx != nil {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	}
	_, err := p.Exec(ctx, sql, args...)
	return err
}

// RunBatchTx creates a new Batch, passes it to queueFunc for query queueing,
// and executes the batch inside a database transaction.
func RunBatchTx(ctx context.Context, p *pgxpool.Pool, queueFunc func(ctx context.Context, b *Batch) error) (retErr error) {
	ctx, span := telemetry.StartSpan(ctx, "pgdb.RunBatchTx")
	defer span.End()

	tx, ctx, err := beginPoolTx(ctx, p)
	if err != nil {
		return fmt.Errorf("begin pool tx: %w", err)
	}
	defer func() {
		if retErr != nil {
			if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				retErr = errors.Join(retErr, fmt.Errorf("rollback tx: %w", err))
			}
		}
	}()

	b := &Batch{
		b: &pgx.Batch{},
	}

	if err := queueFunc(ctx, b); err != nil {
		return fmt.Errorf("queueFunc: %w", err)
	}

	result := tx.SendBatch(ctx, b.b)
	if err := result.Close(); err != nil {
		return fmt.Errorf("close batch result: %w", err)
	}

	if err := tx.Commit(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
