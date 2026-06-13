// Package pgauth provides auth token db access functionality.
package pgauth

import (
	"time"

	"github.com/google/uuid"
)

// MagicToken is a row from magic_link_tokens joined with the owning user's external_id.
type MagicToken struct {
	ID             int64      `db:"id"`
	UserExternalID uuid.UUID  `db:"user_external_id"`
	ExpiresAt      time.Time  `db:"expires_at"`
	UsedAt         *time.Time `db:"used_at"`
}

// CreateMagicToken holds the fields required to create a new magic-link token.
type CreateMagicToken struct {
	UserExternalID uuid.UUID
	TokenHash      string
	ExpiresAt      time.Time
}

// RefreshToken is a row from refresh_tokens joined with the owning user's external_id.
type RefreshToken struct {
	ID             int64      `db:"id"`
	UserExternalID uuid.UUID  `db:"user_external_id"`
	ExpiresAt      time.Time  `db:"expires_at"`
	RevokedAt      *time.Time `db:"revoked_at"`
	CreatedAt      time.Time  `db:"created_at"`
}

// CreateRefreshToken holds the fields required to create a new refresh token.
type CreateRefreshToken struct {
	UserExternalID uuid.UUID
	TokenHash      string
	ExpiresAt      time.Time
}
