package auth

import (
	"time"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
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

func permissionsFromPg(names []string) []mdl.Permission {
	return slicesx.Map(names, func(n string) mdl.Permission { return mdl.Permission(n) })
}
