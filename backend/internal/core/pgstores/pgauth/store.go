package pgauth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

// Store provides auth token database access.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store backed by pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateMagicToken inserts a new magic-link token and returns the created row.
func (s *Store) CreateMagicToken(ctx context.Context, cm CreateMagicToken) (MagicToken, error) {
	var token MagicToken

	q := createMagicTokenQuery(cm)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &token); err != nil {
			return fmt.Errorf("create magic token: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return MagicToken{}, err
	}

	return token, nil
}

// MagicTokenByHash returns the valid (unexpired, unconsumed) magic-link token
// with the given hash.
// Returns [sql.ErrNoRows] if no such token exists.
func (s *Store) MagicTokenByHash(ctx context.Context, hash string) (MagicToken, error) {
	var token MagicToken

	q := magicTokenByHashQuery(hash)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &token); err != nil {
			return fmt.Errorf("magic token by hash: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return MagicToken{}, err
	}

	return token, nil
}

// ConsumeMagicToken marks the token with the given ID as used.
// Returns [sql.ErrNoRows] if the token was already consumed (concurrent request).
func (s *Store) ConsumeMagicToken(ctx context.Context, id int64) error {
	var consumed int64

	q := consumeMagicTokenQuery(id)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &consumed); err != nil {
			return fmt.Errorf("consume magic token: %w", err)
		}
		return nil
	}

	return pgdb.RunBatch(ctx, s.pool, doInBatch)
}

// CreateRefreshToken inserts a new refresh token and returns the created row.
func (s *Store) CreateRefreshToken(ctx context.Context, cr CreateRefreshToken) (RefreshToken, error) {
	var token RefreshToken

	q := createRefreshTokenQuery(cr)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &token); err != nil {
			return fmt.Errorf("create refresh token: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return RefreshToken{}, err
	}

	return token, nil
}

// RefreshTokenByHash returns the valid (unexpired, unrevoked) refresh token
// with the given hash.
// Returns [sql.ErrNoRows] if no such token exists.
func (s *Store) RefreshTokenByHash(ctx context.Context, hash string) (RefreshToken, error) {
	var token RefreshToken

	q := refreshTokenByHashQuery(hash)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &token); err != nil {
			return fmt.Errorf("refresh token by hash: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return RefreshToken{}, err
	}

	return token, nil
}

// RevokeRefreshToken marks the token with the given ID as revoked.
// Returns [sql.ErrNoRows] if the token was already revoked (concurrent request).
func (s *Store) RevokeRefreshToken(ctx context.Context, id int64) error {
	var revoked int64

	q := revokeRefreshTokenQuery(id)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &revoked); err != nil {
			return fmt.Errorf("revoke refresh token: %w", err)
		}
		return nil
	}

	return pgdb.RunBatch(ctx, s.pool, doInBatch)
}
