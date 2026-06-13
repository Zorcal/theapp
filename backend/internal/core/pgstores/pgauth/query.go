package pgauth

import (
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func createMagicTokenQuery(cm CreateMagicToken) pgdb.TypedQuery[MagicToken] {
	params := pgx.NamedArgs{
		"user_external_id": cm.UserExternalID,
		"token_hash":       cm.TokenHash,
		"expires_at":       cm.ExpiresAt,
	}
	// Use an INSERT … SELECT to resolve the internal user_id from the external
	// UUID within the same statement, then re-join to return it as user_external_id.
	const sql = `
		WITH ins AS (
			INSERT INTO useraccess.magic_link_tokens (user_id, token_hash, expires_at)
			SELECT id, @token_hash, @expires_at
			FROM useraccess.users
			WHERE external_id = @user_external_id
			RETURNING id, user_id, expires_at, used_at
		)
		SELECT ins.id, u.external_id AS user_external_id, ins.expires_at, ins.used_at
		FROM ins
		JOIN useraccess.users u ON u.id = ins.user_id`

	return pgdb.TypedQuery[MagicToken]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[MagicToken],
		Expect: pgdb.ExpectOne,
	}
}

// magicTokenByHashQuery returns an unexpired, unconsumed token.
// Returns sql.ErrNoRows when the token is not found, expired, or already used.
func magicTokenByHashQuery(hash string) pgdb.TypedQuery[MagicToken] {
	params := pgx.NamedArgs{"token_hash": hash}
	const sql = `
		SELECT mlt.id, u.external_id AS user_external_id, mlt.expires_at, mlt.used_at
		FROM useraccess.magic_link_tokens mlt
		JOIN useraccess.users u ON u.id = mlt.user_id
		WHERE mlt.token_hash = @token_hash
		  AND mlt.used_at IS NULL
		  AND mlt.expires_at > NOW()`

	return pgdb.TypedQuery[MagicToken]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[MagicToken],
		Expect: pgdb.ExpectOne,
	}
}

// consumeMagicTokenQuery marks a token as used.
// Returns sql.ErrNoRows when the token was already consumed (race condition).
func consumeMagicTokenQuery(id int64) pgdb.TypedQuery[int64] {
	params := pgx.NamedArgs{"id": id}
	const sql = `
		UPDATE useraccess.magic_link_tokens
		SET used_at = NOW()
		WHERE id = @id AND used_at IS NULL
		RETURNING id`

	return pgdb.TypedQuery[int64]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int64, error) {
			var id int64
			if err := row.Scan(&id); err != nil {
				return 0, fmt.Errorf("scan id: %w", err)
			}
			return id, nil
		},
		Expect: pgdb.ExpectOne,
	}
}

func createRefreshTokenQuery(cr CreateRefreshToken) pgdb.TypedQuery[RefreshToken] {
	params := pgx.NamedArgs{
		"user_external_id": cr.UserExternalID,
		"token_hash":       cr.TokenHash,
		"expires_at":       cr.ExpiresAt,
	}
	const sql = `
		WITH ins AS (
			INSERT INTO useraccess.refresh_tokens (user_id, token_hash, expires_at)
			SELECT id, @token_hash, @expires_at
			FROM useraccess.users
			WHERE external_id = @user_external_id
			RETURNING id, user_id, expires_at, revoked_at, created_at
		)
		SELECT ins.id, u.external_id AS user_external_id, ins.expires_at, ins.revoked_at, ins.created_at
		FROM ins
		JOIN useraccess.users u ON u.id = ins.user_id`

	return pgdb.TypedQuery[RefreshToken]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RefreshToken],
		Expect: pgdb.ExpectOne,
	}
}

// refreshTokenByHashQuery returns an unexpired, unrevoked token.
// Returns sql.ErrNoRows when the token is not found, expired, or revoked.
func refreshTokenByHashQuery(hash string) pgdb.TypedQuery[RefreshToken] {
	params := pgx.NamedArgs{"token_hash": hash}
	const sql = `
		SELECT rt.id, u.external_id AS user_external_id, rt.expires_at, rt.revoked_at, rt.created_at
		FROM useraccess.refresh_tokens rt
		JOIN useraccess.users u ON u.id = rt.user_id
		WHERE rt.token_hash = @token_hash
		  AND rt.revoked_at IS NULL
		  AND rt.expires_at > NOW()`

	return pgdb.TypedQuery[RefreshToken]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RefreshToken],
		Expect: pgdb.ExpectOne,
	}
}

// revokeRefreshTokenQuery marks a token as revoked.
// Returns sql.ErrNoRows when the token was already revoked (race condition).
func revokeRefreshTokenQuery(id int64) pgdb.TypedQuery[int64] {
	params := pgx.NamedArgs{"id": id}
	const sql = `
		UPDATE useraccess.refresh_tokens
		SET revoked_at = NOW()
		WHERE id = @id AND revoked_at IS NULL
		RETURNING id`

	return pgdb.TypedQuery[int64]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int64, error) {
			var id int64
			if err := row.Scan(&id); err != nil {
				return 0, fmt.Errorf("scan id: %w", err)
			}
			return id, nil
		},
		Expect: pgdb.ExpectOne,
	}
}
