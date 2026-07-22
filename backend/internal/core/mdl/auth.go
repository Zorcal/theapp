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

// AuthSession is resolved once per request and threaded through the call stack, pairing the
// caller's identity with the project it's currently operating in, if any.
type AuthSession struct {
	User AuthUser
	// ProjectID is the project the caller is currently operating in. Nil for a request with no
	// project context, in which case User.Permissions is resolved from system-scope role
	// assignments only.
	ProjectID *int
	// OrgID is the organization ProjectID belongs to. Nil exactly when ProjectID is nil.
	OrgID *int
}

// MustProjectID returns the project ID.
//
// It panics if the session has no project context. Use this only when the
// caller has already established that the request is project-scoped.
func (as AuthSession) MustProjectID() int {
	if as.ProjectID == nil {
		panic("MustProjectID called on an AuthSession without project context")
	}
	return *as.ProjectID
}

// MustOrgID returns the organization ID.
//
// It panics if the session has no project context. Use this only when the
// caller has already established that the request is project-scoped.
func (as AuthSession) MustOrgID() int {
	if as.OrgID == nil {
		panic("MustOrgID called on an AuthSession without project context")
	}
	return *as.OrgID
}

type contextKeyAuthSession struct{}

// ContextWithAuthSession returns a copy of ctx carrying s as the current request's auth session.
func ContextWithAuthSession(ctx context.Context, s AuthSession) context.Context {
	return context.WithValue(ctx, contextKeyAuthSession{}, s)
}

// AuthSessionFromContext extracts the current request's auth session from ctx.
// Returns the zero AuthSession and false when no session is present (unauthenticated request).
func AuthSessionFromContext(ctx context.Context) (AuthSession, bool) {
	s, ok := ctx.Value(contextKeyAuthSession{}).(AuthSession)
	return s, ok
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
