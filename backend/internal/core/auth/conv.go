package auth

import (
	"time"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
)

func createMagicLinkTokenToPg(userID int, hash string, expiresAt time.Time) pgauth.CreateMagicLinkToken {
	return pgauth.CreateMagicLinkToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}
}

func createRefreshTokenToPg(userID int, hash string, expiresAt time.Time) pgauth.CreateRefreshToken {
	return pgauth.CreateRefreshToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}
}
