// Package pgauth provides auth token db access functionality.
package pgauth

import (
	"time"

	"github.com/google/uuid"
)

// MagicLinkToken is a row from magic_link_tokens joined with the owning user's external_id.
type MagicLinkToken struct {
	ID             int       `db:"id"`
	UserID         int       `db:"user_id"`
	UserExternalID uuid.UUID `db:"user_external_id"`
	ExpiresAt      time.Time `db:"expires_at"`
	CreatedAt      time.Time `db:"created_at"`
}

// CreateMagicLinkToken holds the fields required to create a new magic-link token.
type CreateMagicLinkToken struct {
	UserID    int
	TokenHash string
	ExpiresAt time.Time
}

// RefreshToken is a row from refresh_tokens joined with the owning user's external_id.
type RefreshToken struct {
	ID             int       `db:"id"`
	UserID         int       `db:"user_id"`
	UserExternalID uuid.UUID `db:"user_external_id"`
	ExpiresAt      time.Time `db:"expires_at"`
	CreatedAt      time.Time `db:"created_at"`
}

// CreateRefreshToken holds the fields required to create a new refresh token.
type CreateRefreshToken struct {
	UserID    int
	TokenHash string
	ExpiresAt time.Time
}
