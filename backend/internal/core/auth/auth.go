// Package auth provides magic-link authentication and JWT/refresh-token issuance.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
)

// Transactor runs a function inside a database transaction.
type Transactor interface {
	RunTx(ctx context.Context, fn func(ctx context.Context) error) error
}

//go:generate moq -rm -fmt goimports -out auth_storer_moq_test.go . AuthStorer:MockedAuthStorer

// AuthStorer defines the auth-token database operations required by Core.
type AuthStorer interface {
	// AuthSessionData returns user identity and optional project and organization metadata.
	// Returns [sql.ErrNoRows] if the user or requested project does not exist.
	AuthSessionData(ctx context.Context, userID uuid.UUID, projectID *int) (pgauth.AuthSessionData, error)
	// LatestMagicLinkTokenCreatedAt returns the created_at of the most recently issued token for a user.
	// Returns [sql.ErrNoRows] if the user has never been issued a token.
	LatestMagicLinkTokenCreatedAt(ctx context.Context, userID int) (time.Time, error)
	// InvalidateUserMagicLinkTokens marks all unexpired, unconsumed magic-link tokens for a user as used.
	InvalidateUserMagicLinkTokens(ctx context.Context, userID int) error
	CreateMagicLinkToken(ctx context.Context, cm pgauth.CreateMagicLinkToken) (pgauth.MagicLinkToken, error)
	// MagicLinkTokenByHash returns the valid (unexpired, unconsumed) token with the given hash.
	// Returns [sql.ErrNoRows] if no such token exists.
	MagicLinkTokenByHash(ctx context.Context, hash string) (pgauth.MagicLinkToken, error)
	// ConsumeMagicLinkToken marks the token as used.
	// Returns [sql.ErrNoRows] if the token was already consumed.
	ConsumeMagicLinkToken(ctx context.Context, id int) error
	// LockUser acquires a transaction-level advisory lock for the user, serializing
	// concurrent operations on the same user. Must be called within a transaction.
	LockUser(ctx context.Context, userID int) error

	CreateRefreshToken(ctx context.Context, cr pgauth.CreateRefreshToken) (pgauth.RefreshToken, error)
	// ConsumeRefreshToken atomically revokes the valid (unexpired, unrevoked) token with the given
	// hash and returns it. Returns [sql.ErrNoRows] if no such token exists.
	ConsumeRefreshToken(ctx context.Context, hash string) (pgauth.RefreshToken, error)
	// RevokeAllUserRefreshTokens revokes all active refresh tokens for the user.
	RevokeAllUserRefreshTokens(ctx context.Context, userExternalID uuid.UUID) error
}

//go:generate moq -rm -fmt goimports -out user_storer_moq_test.go . UserStorer:MockedUserStorer

// UserStorer defines the user database operations required by Core.
type UserStorer interface {
	// GetOrCreateUserByEmail returns the user with the given email, creating one if none exists.
	// Safe under concurrent calls for the same email.
	GetOrCreateUserByEmail(ctx context.Context, email string) (pguser.User, error)
	// MarkEmailVerified marks the email as verified for the user with the given external ID.
	// Returns [sql.ErrNoRows] if no such user exists.
	MarkEmailVerified(ctx context.Context, externalID uuid.UUID) error
}

//go:generate moq -rm -fmt goimports -out permission_storer_moq_test.go . PermissionStorer:MockedPermissionStorer

// PermissionStorer defines the permission database operations required by Core.
type PermissionStorer interface {
	// PermissionsByScope returns userID's resolved permission names for projectID at project,
	// organization, and system scope.
	// Returns [sql.ErrNoRows] if no such user or project exists.
	PermissionsByScope(ctx context.Context, userID uuid.UUID, projectID int) (pgrbac.PermissionsByScope, error)
	// SystemPermissions returns the names of the permissions held through system-scope role
	// assignments only.
	// Returns [sql.ErrNoRows] if no such user exists.
	SystemPermissions(ctx context.Context, userID uuid.UUID) ([]string, error)
	// ProjectPermissions returns projectID's org and the names of the permissions userID holds for
	// projectID, resolved from project-, org-, and system-scope role assignments.
	// Returns [sql.ErrNoRows] if no such user or project exists.
	ProjectPermissions(ctx context.Context, userID uuid.UUID, projectID int) (pgrbac.ProjectPermissions, error)
	// OrgPermissionsByProjectID returns projectID's org and the names of the permissions userID
	// holds there through organization- and system-scope role assignments.
	// Returns [sql.ErrNoRows] if no such user or project exists.
	OrgPermissionsByProjectID(ctx context.Context, userID uuid.UUID, projectID int) (pgrbac.OrgPermissions, error)
}

// Config holds tunables for Core.
type Config struct {
	JWTKey             []byte
	JWTIssuer          string // e.g. "theapp"
	JWTAudience        string // e.g. "theapp-api"
	MagicLinkFromEmail string // sender address, e.g. "App <noreply@theapp.com>"
	// MagicLinkBaseURL is prepended to the magic-link token parameter,
	// e.g. "https://theapp.com/auth/verify".
	MagicLinkBaseURL string
	MagicLinkTTL     time.Duration
	// MagicLinkRateLimit is the minimum interval between magic-link requests for the same email.
	// Zero disables rate limiting.
	MagicLinkRateLimit time.Duration
	AccessTokenTTL     time.Duration // JWT access token lifetime
	RefreshTokenTTL    time.Duration // refresh token lifetime
}

// Core holds the business logic for authentication.
type Core struct {
	authStorer       AuthStorer
	userStorer       UserStorer
	permissionStorer PermissionStorer
	transactor       Transactor
	cfg              Config
}

// NewCore constructs a Core with the given dependencies and configuration.
func NewCore(as AuthStorer, us UserStorer, ps PermissionStorer, tr Transactor, cfg Config) *Core {
	return &Core{
		authStorer:       as,
		userStorer:       us,
		permissionStorer: ps,
		transactor:       tr,
		cfg:              cfg,
	}
}

// MagicLinkToken ensures the user exists, rate-checks, invalidates prior tokens, and creates a
// new one inside a transaction. Returns the raw token.
// Returns [mdl.ErrRateLimited] if a token was already issued to rml.Email within the rate-limit window.
// Returns [mdl.ErrValidation] if rml is invalid.
func (c *Core) MagicLinkToken(ctx context.Context, rml mdl.RequestMagicLink) (string, error) {
	if err := rml.Validate(); err != nil {
		return "", fmt.Errorf("validate: %w", err)
	}

	emailAddr := strings.ToLower(rml.Email)

	pgUser, err := c.userStorer.GetOrCreateUserByEmail(ctx, emailAddr)
	if err != nil {
		return "", fmt.Errorf("get or create user: %w", err)
	}

	var rawTok string
	var rateLimited bool
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.authStorer.LockUser(ctx, pgUser.ID); err != nil {
			return fmt.Errorf("lock user: %w", err)
		}

		if c.cfg.MagicLinkRateLimit > 0 {
			lastSent, err := c.authStorer.LatestMagicLinkTokenCreatedAt(ctx, pgUser.ID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("check magic link rate limit: %w", err)
			}
			if err == nil && time.Since(lastSent) < c.cfg.MagicLinkRateLimit {
				rateLimited = true
				return nil
			}
		}

		if err := c.authStorer.InvalidateUserMagicLinkTokens(ctx, pgUser.ID); err != nil {
			return fmt.Errorf("invalidate existing magic link tokens: %w", err)
		}

		tok, hash, err := generateToken()
		if err != nil {
			return fmt.Errorf("generate magic link token: %w", err)
		}

		if _, err := c.authStorer.CreateMagicLinkToken(ctx, createMagicLinkTokenToPg(pgUser.ID, hash, time.Now().Add(c.cfg.MagicLinkTTL))); err != nil {
			return fmt.Errorf("store magic link token: %w", err)
		}
		rawTok = tok

		return nil
	}); err != nil {
		return "", fmt.Errorf("run tx: %w", err)
	}

	if rateLimited {
		return "", mdl.ErrRateLimited
	}

	return rawTok, nil
}

// VerifyMagicLink validates vml and returns an access/refresh token pair.
// Returns [mdl.ErrTokenInvalid] if the token is expired, consumed, or not found.
// Returns [mdl.ErrValidation] if vml is invalid.
func (c *Core) VerifyMagicLink(ctx context.Context, vml mdl.VerifyMagicLink) (mdl.AuthTokenPair, error) {
	if err := vml.Validate(); err != nil {
		return mdl.AuthTokenPair{}, fmt.Errorf("validate: %w", err)
	}

	hash := hashToken(vml.Token)

	tok, err := c.authStorer.MagicLinkTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.AuthTokenPair{}, mdl.ErrTokenInvalid
		}
		return mdl.AuthTokenPair{}, fmt.Errorf("look up magic link token: %w", err)
	}

	var pair mdl.AuthTokenPair
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.authStorer.ConsumeMagicLinkToken(ctx, tok.ID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Token was consumed by a concurrent request.
				return mdl.ErrTokenInvalid
			}
			return fmt.Errorf("consume magic link token: %w", err)
		}

		if err := c.userStorer.MarkEmailVerified(ctx, tok.UserExternalID); err != nil {
			// The persisted token still references the user through a foreign key, so
			// sql.ErrNoRows is an impossible state that must remain an internal error.
			return fmt.Errorf("mark email verified: %w", err)
		}

		var err error
		pair, err = c.issueTokenPair(ctx, tok.UserID, tok.UserExternalID)
		if err != nil {
			return fmt.Errorf("issue token pair: %w", err)
		}

		return nil
	}); err != nil {
		return mdl.AuthTokenPair{}, fmt.Errorf("run tx: %w", err)
	}

	return pair, nil
}

// RefreshAccessToken rotates rt and returns a new access/refresh token pair. The old refresh
// token is revoked atomically with the new pair being issued. Returns [mdl.ErrTokenInvalid] if the
// token is expired, revoked, or not found.
// Returns [mdl.ErrValidation] if rt is invalid.
func (c *Core) RefreshAccessToken(ctx context.Context, rt mdl.RefreshToken) (mdl.AuthTokenPair, error) {
	if err := rt.Validate(); err != nil {
		return mdl.AuthTokenPair{}, fmt.Errorf("validate: %w", err)
	}

	hash := hashToken(rt.Token)

	var pair mdl.AuthTokenPair
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		refreshTok, err := c.authStorer.ConsumeRefreshToken(ctx, hash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrTokenInvalid
			}
			return fmt.Errorf("consume refresh token: %w", err)
		}

		pair, err = c.issueTokenPair(ctx, refreshTok.UserID, refreshTok.UserExternalID)
		if err != nil {
			return fmt.Errorf("issue token pair: %w", err)
		}

		return nil
	}); err != nil {
		return mdl.AuthTokenPair{}, fmt.Errorf("run tx: %w", err)
	}

	return pair, nil
}

// RevokeRefreshToken invalidates rt, ending that session.
// Returns [mdl.ErrTokenInvalid] if the token is not found, already revoked, or expired.
// Returns [mdl.ErrValidation] if rt is invalid.
func (c *Core) RevokeRefreshToken(ctx context.Context, rt mdl.RefreshToken) error {
	if err := rt.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	hash := hashToken(rt.Token)

	if _, err := c.authStorer.ConsumeRefreshToken(ctx, hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrTokenInvalid
		}
		return fmt.Errorf("revoke refresh token: %w", err)
	}

	return nil
}

// RevokeAllUserRefreshTokens invalidates all active refresh tokens for userExternalID, ending all sessions.
func (c *Core) RevokeAllUserRefreshTokens(ctx context.Context, userExternalID uuid.UUID) error {
	if err := c.authStorer.RevokeAllUserRefreshTokens(ctx, userExternalID); err != nil {
		return fmt.Errorf("revoke all user refresh tokens: %w", err)
	}
	return nil
}

// AuthSession resolves userID's identity and its permissions. When projectID is non-nil, the
// permissions are resolved from project-, org-, and system-scope role assignments for that
// project, and the returned session includes its project and organization metadata. When projectID
// is nil, permissions are resolved from system-scope role assignments only and Project is nil.
// Returns [mdl.ErrNotFound] if no user with that ID exists, or if projectID is non-nil and does
// not match any project.
// A user with no role assignment relevant to projectID is not an error: the session resolves with
// an empty (or system-scope-only) permission set, which permission-checking code rejects on its own.
func (c *Core) AuthSession(ctx context.Context, userID uuid.UUID, projectID *int) (mdl.AuthSession, error) {
	data, err := c.authStorer.AuthSessionData(ctx, userID, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.AuthSession{}, mdl.ErrNotFound
		}
		return mdl.AuthSession{}, fmt.Errorf("auth session data: %w", err)
	}

	if projectID == nil {
		perms, err := c.permissionStorer.SystemPermissions(ctx, data.UserExternalID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.AuthSession{}, mdl.ErrNotFound
			}
			return mdl.AuthSession{}, fmt.Errorf("system permissions: %w", err)
		}

		return authSessionFromPg(data, perms), nil
	}

	perms, err := c.permissionStorer.ProjectPermissions(ctx, data.UserExternalID, *projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.AuthSession{}, mdl.ErrNotFound
		}
		return mdl.AuthSession{}, fmt.Errorf("project permissions: %w", err)
	}

	return authSessionFromPg(data, perms.PermissionNames), nil
}

// OrganizationAuthSession resolves userID's permissions for projectID's organization from
// organization- and system-scope role assignments.
// Returns [mdl.ErrNotFound] if no user or project with those IDs exists.
func (c *Core) OrganizationAuthSession(ctx context.Context, userID uuid.UUID, projectID int) (mdl.AuthSession, error) {
	data, err := c.authStorer.AuthSessionData(ctx, userID, &projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.AuthSession{}, mdl.ErrNotFound
		}
		return mdl.AuthSession{}, fmt.Errorf("auth session data: %w", err)
	}

	orgPerms, err := c.permissionStorer.OrgPermissionsByProjectID(ctx, data.UserExternalID, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.AuthSession{}, mdl.ErrNotFound
		}
		return mdl.AuthSession{}, fmt.Errorf("organization permissions by project id: %w", err)
	}

	return authSessionFromPg(data, orgPerms.PermissionNames), nil
}

// AuthContext returns authorization context for the authenticated caller and selected project.
// Returns [mdl.ErrNotFound] if the caller or selected project no longer exists.
func (c *Core) AuthContext(ctx context.Context) (mdl.AuthContext, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return mdl.AuthContext{}, errors.New("auth session missing")
	}
	if sess.Project == nil {
		return mdl.AuthContext{}, errors.New("project context missing")
	}

	perms, err := c.permissionStorer.PermissionsByScope(ctx, sess.User.UserID, sess.Project.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.AuthContext{}, mdl.ErrNotFound
		}
		return mdl.AuthContext{}, fmt.Errorf("permissions by scope: %w", err)
	}

	return mdl.AuthContext{
		UserID:                  sess.User.UserID,
		Email:                   sess.User.Email,
		ProjectPermissions:      permissionsFromPg(perms.ProjectPermissionNames),
		OrganizationPermissions: permissionsFromPg(perms.OrgPermissionNames),
		SystemPermissions:       permissionsFromPg(perms.SystemPermissionNames),
	}, nil
}

// issueTokenPair mints a signed JWT access token and a new opaque refresh token, persists the refresh token,
// and returns both.
func (c *Core) issueTokenPair(ctx context.Context, userID int, userExternalID uuid.UUID) (mdl.AuthTokenPair, error) {
	now := time.Now()

	claims := mdl.AuthClaims{
		UserID: userExternalID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.cfg.JWTIssuer,
			Audience:  jwt.ClaimStrings{c.cfg.JWTAudience},
			Subject:   userExternalID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(c.cfg.AccessTokenTTL)),
		},
	}

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.cfg.JWTKey)
	if err != nil {
		return mdl.AuthTokenPair{}, fmt.Errorf("sign access token: %w", err)
	}

	rawRefresh, refreshHash, err := generateToken()
	if err != nil {
		return mdl.AuthTokenPair{}, fmt.Errorf("generate refresh token: %w", err)
	}

	pgCreateRefreshToken := createRefreshTokenToPg(userID, refreshHash, now.Add(c.cfg.RefreshTokenTTL))
	if _, err := c.authStorer.CreateRefreshToken(ctx, pgCreateRefreshToken); err != nil {
		return mdl.AuthTokenPair{}, fmt.Errorf("store refresh token: %w", err)
	}

	return mdl.AuthTokenPair{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		ExpiresIn:    c.cfg.AccessTokenTTL,
	}, nil
}

// generateToken returns a cryptographically random URL-safe token and its SHA-256 hex digest. Only the digest is
// stored; the raw token is sent to the client and never persisted.
func generateToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", "", fmt.Errorf("read random bytes: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
