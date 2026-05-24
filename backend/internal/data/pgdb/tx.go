package pgdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

	return tx, ctx, nil
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
