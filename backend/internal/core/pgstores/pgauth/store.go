package pgauth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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

// LatestMagicLinkTokenCreatedAt returns the created_at of the most recently issued magic-link token for a user.
// Returns [sql.ErrNoRows] if the user has never been issued a token.
func (s *Store) LatestMagicLinkTokenCreatedAt(ctx context.Context, userID int) (time.Time, error) {
	var createdAt time.Time

	q := latestMagicLinkTokenCreatedAtQuery(userID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &createdAt); err != nil {
			return fmt.Errorf("latest magic link token created at: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return time.Time{}, err
	}

	return createdAt, nil
}

// InvalidateUserMagicLinkTokens marks all unexpired, unconsumed magic-link tokens for a user as used.
func (s *Store) InvalidateUserMagicLinkTokens(ctx context.Context, userID int) error {
	var invalidated []int

	q := invalidateUserMagicLinkTokensQuery(userID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &invalidated); err != nil {
			return fmt.Errorf("invalidate user magic link tokens: %w", err)
		}
		return nil
	}

	return pgdb.RunBatch(ctx, s.pool, doInBatch)
}

// CreateMagicLinkToken inserts a new magic-link token and returns the created row.
func (s *Store) CreateMagicLinkToken(ctx context.Context, cm CreateMagicLinkToken) (MagicLinkToken, error) {
	var token MagicLinkToken

	q := createMagicLinkTokenQuery(cm)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &token); err != nil {
			return fmt.Errorf("create magic link token: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return MagicLinkToken{}, err
	}

	return token, nil
}

// MagicLinkTokenByHash returns the valid (unexpired, unconsumed) magic-link token
// with the given hash.
// Returns [sql.ErrNoRows] if no such token exists.
func (s *Store) MagicLinkTokenByHash(ctx context.Context, hash string) (MagicLinkToken, error) {
	var token MagicLinkToken

	q := magicLinkTokenByHashQuery(hash)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &token); err != nil {
			return fmt.Errorf("magic link token by hash: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return MagicLinkToken{}, err
	}

	return token, nil
}

// ConsumeMagicLinkToken marks the token with the given ID as used.
// Returns [sql.ErrNoRows] if the token was already consumed (concurrent request).
func (s *Store) ConsumeMagicLinkToken(ctx context.Context, id int) error {
	var consumed int

	q := consumeMagicLinkTokenQuery(id)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &consumed); err != nil {
			return fmt.Errorf("consume magic link token: %w", err)
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

// LockUser acquires a transaction-level advisory lock keyed on userID, serializing concurrent
// operations for the same user. Must be called within a transaction; the lock is released
// automatically when the transaction ends.
//
// Uses the two-key form of pg_advisory_xact_lock, namespaced with hashtext('auth.user'), so this
// lock can never collide with an unrelated single-key advisory lock some other feature takes on the
// same small integer. Advisory locks share one keyspace per database; without a namespace, a future
// pg_advisory_xact_lock(n) call for something unrelated to auth would silently contend with the
// magic-link flow for user id n.
func (s *Store) LockUser(ctx context.Context, userID int) error {
	return pgdb.RunExec(ctx, s.pool, "SELECT pg_advisory_xact_lock(hashtext('auth.user'), $1)", userID)
}

// ConsumeRefreshToken atomically revokes the valid (unexpired, unrevoked) refresh token with the
// given hash and returns it.
// Returns [sql.ErrNoRows] if no matching valid token exists.
func (s *Store) ConsumeRefreshToken(ctx context.Context, hash string) (RefreshToken, error) {
	var token RefreshToken

	q := consumeRefreshTokenQuery(hash)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &token); err != nil {
			return fmt.Errorf("consume refresh token: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return RefreshToken{}, err
	}

	return token, nil
}

// RevokeAllUserRefreshTokens revokes all active refresh tokens for the user.
func (s *Store) RevokeAllUserRefreshTokens(ctx context.Context, userExternalID uuid.UUID) error {
	var revoked []int

	q := revokeAllUserRefreshTokensQuery(userExternalID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &revoked); err != nil {
			return fmt.Errorf("revoke all user refresh tokens: %w", err)
		}
		return nil
	}

	return pgdb.RunBatch(ctx, s.pool, doInBatch)
}
