package mdl

import (
	"context"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"
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

// AuthContext contains authorization data for the authenticated caller and selected project.
type AuthContext struct {
	UserID                  uuid.UUID
	Email                   string
	ProjectPermissions      []Permission
	OrganizationPermissions []Permission
	SystemPermissions       []Permission
}

// AuthUser is the authenticated caller's identity and resolved permissions.
type AuthUser struct {
	UserID uuid.UUID
	Email  string
	// Permissions is the distinct union of permissions granted through every role UserID holds.
	Permissions []Permission
}

// AuthProject identifies the project selected for an authenticated request and its organization.
type AuthProject struct {
	ID      int
	OrgID   int
	OrgName string
	// IsControl reports whether this is the organization's structurally designated control
	// project. Control-project identity does not depend on the project's name.
	IsControl bool
	// IsOrgMember reports whether the authenticated user belongs to the project's organization. It
	// can be false because resolving a session for an existing project does not itself require
	// organization membership.
	IsOrgMember bool
}

// IsSystemControlProject reports whether ap is the system organization's control project.
func (ap AuthProject) IsSystemControlProject() bool {
	return ap.OrgName == SystemOrgName && ap.IsControl
}

// AuthSession is resolved once per request and threaded through the call stack, pairing the
// caller's identity with the project it's currently operating in, if any.
type AuthSession struct {
	User AuthUser
	// Project is nil for a request without project context, in which case User.Permissions is
	// resolved from system-scope role assignments only.
	Project *AuthProject
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
