package pgauth

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func createMagicLinkTokenQuery(cm CreateMagicLinkToken) pgdb.TypedQuery[MagicLinkToken] {
	params := pgx.NamedArgs{
		"user_id":    cm.UserID,
		"token_hash": cm.TokenHash,
		"expires_at": cm.ExpiresAt,
	}
	// INSERT … SELECT to handle the unknown-user case: if no row matches WHERE id = @user_id,
	// the SELECT returns 0 rows, nothing is inserted, and the RETURNING clause yields sql.ErrNoRows.
	const sql = `
		WITH ins AS (
			INSERT INTO useraccess.magic_link_tokens (user_id, token_hash, expires_at)
			SELECT id, @token_hash, @expires_at
			FROM useraccess.users
			WHERE id = @user_id
			RETURNING id, user_id, expires_at, created_at
		)
		SELECT ins.id, ins.user_id, u.external_id AS user_external_id, ins.expires_at, ins.created_at
		FROM ins
		JOIN useraccess.users u ON u.id = ins.user_id`

	return pgdb.TypedQuery[MagicLinkToken]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[MagicLinkToken],
		Expect: pgdb.ExpectOne,
	}
}

func magicLinkTokenByHashQuery(hash string) pgdb.TypedQuery[MagicLinkToken] {
	params := pgx.NamedArgs{"token_hash": hash}
	const sql = `
		SELECT mlt.id, mlt.user_id, u.external_id AS user_external_id, mlt.expires_at, mlt.created_at
		FROM useraccess.magic_link_tokens mlt
		JOIN useraccess.users u ON u.id = mlt.user_id
		WHERE mlt.token_hash = @token_hash
		  AND mlt.used_at IS NULL
		  AND mlt.expires_at > NOW()`

	return pgdb.TypedQuery[MagicLinkToken]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[MagicLinkToken],
		Expect: pgdb.ExpectOne,
	}
}

func latestMagicLinkTokenCreatedAtQuery(userID int) pgdb.TypedQuery[time.Time] {
	params := pgx.NamedArgs{"user_id": userID}
	const sql = `
		SELECT created_at
		FROM useraccess.magic_link_tokens
		WHERE user_id = @user_id
		ORDER BY created_at DESC
		LIMIT 1`

	return pgdb.TypedQuery[time.Time]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (time.Time, error) {
			var t time.Time
			return t, row.Scan(&t)
		},
		Expect: pgdb.ExpectOne,
	}
}

func invalidateUserMagicLinkTokensQuery(userID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID}
	const sql = `
		UPDATE useraccess.magic_link_tokens
		SET used_at = NOW()
		WHERE user_id = @user_id
		  AND used_at IS NULL
		  AND expires_at > NOW()
		RETURNING id`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var id int
			return id, row.Scan(&id)
		},
		Expect: pgdb.ExpectMany,
	}
}

func consumeMagicLinkTokenQuery(id int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"id": id}
	const sql = `
		UPDATE useraccess.magic_link_tokens
		SET used_at = NOW()
		WHERE id = @id AND used_at IS NULL AND expires_at > NOW()
		RETURNING id`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var id int
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
		"user_id":    cr.UserID,
		"token_hash": cr.TokenHash,
		"expires_at": cr.ExpiresAt,
	}
	// INSERT … SELECT to handle the unknown-user case identically to createMagicLinkTokenQuery.
	const sql = `
		WITH ins AS (
			INSERT INTO useraccess.refresh_tokens (user_id, token_hash, expires_at)
			SELECT id, @token_hash, @expires_at
			FROM useraccess.users
			WHERE id = @user_id
			RETURNING id, user_id, expires_at, created_at
		)
		SELECT ins.id, ins.user_id, u.external_id AS user_external_id, ins.expires_at, ins.created_at
		FROM ins
		JOIN useraccess.users u ON u.id = ins.user_id`

	return pgdb.TypedQuery[RefreshToken]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RefreshToken],
		Expect: pgdb.ExpectOne,
	}
}

func consumeRefreshTokenQuery(hash string) pgdb.TypedQuery[RefreshToken] {
	params := pgx.NamedArgs{"token_hash": hash}
	// UPDATE … FROM to atomically revoke and return the token with its owner's external_id in one
	// round-trip. Two concurrent requests for the same token both attempt the UPDATE; the database
	// serializes the writes at the row level so exactly one succeeds and gets the RETURNING row —
	// the other gets zero rows (sql.ErrNoRows) without triggering false reuse detection.
	const sql = `
		UPDATE useraccess.refresh_tokens rt
		SET revoked_at = NOW()
		FROM useraccess.users u
		WHERE rt.token_hash = @token_hash
		  AND rt.revoked_at IS NULL
		  AND rt.expires_at > NOW()
		  AND u.id = rt.user_id
		RETURNING rt.id, rt.user_id, u.external_id AS user_external_id, rt.expires_at, rt.created_at`

	return pgdb.TypedQuery[RefreshToken]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RefreshToken],
		Expect: pgdb.ExpectOne,
	}
}

func revokeAllUserRefreshTokensQuery(userExternalID uuid.UUID) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"external_id": userExternalID}
	const sql = `
		UPDATE useraccess.refresh_tokens rt
		SET revoked_at = NOW()
		FROM useraccess.users u
		WHERE u.external_id = @external_id
		  AND rt.user_id = u.id
		  AND rt.revoked_at IS NULL
		RETURNING rt.id`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var id int
			if err := row.Scan(&id); err != nil {
				return 0, fmt.Errorf("scan id: %w", err)
			}
			return id, nil
		},
		Expect: pgdb.ExpectMany,
	}
}
