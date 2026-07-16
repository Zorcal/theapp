package mdl

import (
	"context"
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

type contextKeyAuthUser struct{}

// ContextWithAuthUser returns a copy of ctx carrying u as the authenticated caller's identity.
func ContextWithAuthUser(ctx context.Context, u AuthUser) context.Context {
	return context.WithValue(ctx, contextKeyAuthUser{}, u)
}

// AuthUserFromContext extracts the authenticated caller's identity from ctx.
// Returns the zero AuthUser and false when no user is present (unauthenticated request).
func AuthUserFromContext(ctx context.Context) (AuthUser, bool) {
	u, ok := ctx.Value(contextKeyAuthUser{}).(AuthUser)
	return u, ok
}

// AuthSession is resolved once per request and threaded through the call stack, pairing the
// caller's identity with the project it's currently operating in.
type AuthSession struct {
	User      AuthUser
	ProjectID int
}

// RequestMagicLink holds the fields needed to send a magic-link sign-in token.
type RequestMagicLink struct {
	Email string
}

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

func (rt RefreshToken) Validate() error {
	if rt.Token == "" {
		return validationError("token required")
	}
	return nil
}
