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

// AuthUser is the authenticated caller's identity and resolved permissions.
type AuthUser struct {
	UserID uuid.UUID
	// Permissions is the distinct union of permissions granted through every role UserID holds.
	Permissions []Permission
}

// RequestMagicLink holds the fields needed to send a magic-link sign-in token.
type RequestMagicLink struct {
	Email string
}

// Validate reports whether rml is valid.
func (rml RequestMagicLink) Validate() error {
	if rml.Email == "" {
		return validationError("email required")
	}
	if !IsValidEmail(rml.Email) {
		return validationError("email invalid")
	}
	return nil
}

// VerifyMagicLink holds the fields needed to verify a magic-link token.
type VerifyMagicLink struct {
	Token string
}

// Validate reports whether vml is valid.
func (vml VerifyMagicLink) Validate() error {
	if vml.Token == "" {
		return validationError("token required")
	}
	return nil
}

// RefreshToken holds the fields needed to mint a new access/refresh token pair or revoke an existing session.
type RefreshToken struct {
	Token string
}

// Validate reports whether rt is valid.
func (rt RefreshToken) Validate() error {
	if rt.Token == "" {
		return validationError("token required")
	}
	return nil
}
