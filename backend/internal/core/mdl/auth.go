package mdl

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AuthClaims represents the authorization claims transmitted via a JWT.
type AuthClaims struct {
	jwt.RegisteredClaims

	// UserID is the authenticated user's external UUID.
	UserID uuid.UUID `json:"uid"`
}

// AuthTokenPair holds a short-lived access token and its paired long-lived refresh token.
type AuthTokenPair struct {
	AccessToken  string
	RefreshToken string
	// ExpiresIn is the access token's remaining lifetime.
	ExpiresIn time.Duration
}
