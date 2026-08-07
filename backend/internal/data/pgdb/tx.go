package pgdb

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/telemetry"
)

// Transactor runs a function inside a database transaction. It can be injected
// into cores via a narrow interface so cores do not need to import pgxpool.
type Transactor struct {
	pool *pgxpool.Pool
}

// NewTransactor returns a Transactor backed by the given pool.
func NewTransactor(pool *pgxpool.Pool) *Transactor {
	return &Transactor{pool: pool}
}

// RunTx begins a transaction, embeds it in ctx, and calls fn with the enriched context. A newly
// opened transaction receives transaction-local project, user, and trace settings from ctx. On
// success it commits; on error it rolls back.
// If ctx already carries a transaction, that transaction is reused and
// RunTx neither commits nor rolls back — the outer caller owns the lifecycle.
func (t *Transactor) RunTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := RunTx(ctx, t.pool, fn); err != nil {
		return err
	}
	return nil
}

// RunTx begins a transaction, embeds it in ctx, and calls fn with the enriched context. A newly
// opened transaction receives transaction-local project, user, and trace settings from ctx. On
// success it commits; on error it rolls back.
//
// If ctx already carries a transaction (i.e. RunTx is being called from within
// an outer RunTx), the existing transaction is reused and this call neither
// commits nor rolls back — the outermost caller owns the lifecycle. This means
// an inner RunTx returning nil does not guarantee a commit: if the outer caller
// later returns an error, the whole transaction is rolled back, including the
// work done inside the inner call.
func RunTx(ctx context.Context, p *pgxpool.Pool, fn func(ctx context.Context) error) (retErr error) {
	// owner tracks whether this call opened the transaction. Only the owner
	// commits or rolls back; inner (non-owner) calls are no-ops on lifecycle.
	owner := txFromCtx(ctx) == nil

	tx, ctx, err := beginPoolTx(ctx, p)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if !owner || retErr == nil {
			return
		}
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("rollback tx: %w", err))
		}
	}()

	if err := fn(ctx); err != nil {
		return err
	}

	if owner {
		if err := tx.Commit(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			return fmt.Errorf("commit tx: %w", err)
		}
	}

	return nil
}

// beginPoolTx returns the existing transaction from ctx or starts a new one
// on the pool. This avoids nested transactions by reusing a transaction
// already in the context.
func beginPoolTx(ctx context.Context, p *pgxpool.Pool) (pgx.Tx, context.Context, error) {
	if tx := txFromCtx(ctx); tx != nil {
		return tx, ctx, nil
	}

	tx, err := p.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}

	ctx = ctxtWithTx(ctx, tx)
	if err := setLocalSettings(ctx, tx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback tx: %w", rollbackErr))
		}
		return nil, nil, fmt.Errorf("set transaction-local settings: %w", err)
	}

	return tx, ctx, nil
}

func setLocalSettings(ctx context.Context, tx pgx.Tx) error {
	var (
		settings []string
		args     []any
	)
	add := func(name, value string) {
		args = append(args, value)
		settings = append(settings, fmt.Sprintf("set_config('%s', $%d, true)", name, len(args)))
	}

	if sess, ok := mdl.AuthSessionFromContext(ctx); ok {
		if sess.Project != nil {
			add("app.project_id", strconv.Itoa(sess.Project.ID))
		}
		if sess.User.UserID != uuid.Nil {
			add("app.user_id", sess.User.UserID.String())
		}
	}
	if traceID := telemetry.GetTraceID(ctx); traceID != "" {
		add("app.trace_id", traceID)
	}
	if len(settings) == 0 {
		return nil
	}

	if _, err := tx.Exec(ctx, "SELECT "+strings.Join(settings, ", "), args...); err != nil {
		return err
	}
	return nil
}

type txContextKey struct{}

func ctxtWithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func txFromCtx(ctx context.Context) pgx.Tx {
	tx, ok := ctx.Value(txContextKey{}).(pgx.Tx)
	if !ok || tx == nil {
		return nil
	}
	return tx
}
