// Package auth provides magic-link authentication and JWT/refresh-token issuance.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	htmltmpl "html/template"
	texttmpl "text/template"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/email"
)

//go:generate moq -rm -fmt goimports -out auth_storer_moq_test.go . AuthStorer:MockedAuthStorer
//go:generate moq -rm -fmt goimports -out user_storer_moq_test.go . UserStorer:MockedUserStorer

//go:embed templates/magic_link.txt templates/magic_link.html
var emailFS embed.FS

var (
	magicLinkTextTmpl = texttmpl.Must(texttmpl.ParseFS(emailFS, "templates/magic_link.txt"))
	magicLinkHTMLTmpl = htmltmpl.Must(htmltmpl.ParseFS(emailFS, "templates/magic_link.html"))
)

type magicLinkData struct {
	Link string
	TTL  string
}

// AuthStorer defines the auth-token database operations required by Core.
type AuthStorer interface {
	CreateMagicToken(ctx context.Context, cm pgauth.CreateMagicToken) (pgauth.MagicToken, error)
	// MagicTokenByHash returns the valid (unexpired, unconsumed) token with the given hash.
	// Returns [sql.ErrNoRows] if no such token exists.
	MagicTokenByHash(ctx context.Context, hash string) (pgauth.MagicToken, error)
	// ConsumeMagicToken marks the token as used.
	// Returns [sql.ErrNoRows] if the token was already consumed.
	ConsumeMagicToken(ctx context.Context, id int64) error

	CreateRefreshToken(ctx context.Context, cr pgauth.CreateRefreshToken) (pgauth.RefreshToken, error)
	// RefreshTokenByHash returns the valid (unexpired, unrevoked) token with the given hash.
	// Returns [sql.ErrNoRows] if no such token exists.
	RefreshTokenByHash(ctx context.Context, hash string) (pgauth.RefreshToken, error)
	// RevokeRefreshToken marks the token as revoked.
	// Returns [sql.ErrNoRows] if the token was already revoked.
	RevokeRefreshToken(ctx context.Context, id int64) error
}

// UserStorer defines the user database operations required by Core.
type UserStorer interface {
	// UserByEmail returns the user with the given email.
	// Returns [sql.ErrNoRows] if no such user exists.
	UserByEmail(ctx context.Context, email string) (pguser.User, error)
	CreateUser(ctx context.Context, cu pguser.CreateUser) (pguser.User, error)
}

// Config holds tunables for Core.
type Config struct {
	JWTKey    []byte
	FromEmail string // sender address, e.g. "App <noreply@example.com>"
	// BaseURL is prepended to the magic-link token parameter,
	// e.g. "https://app.example.com/auth/verify".
	BaseURL    string
	MagicTTL   time.Duration // magic-link token lifetime
	AccessTTL  time.Duration // JWT access token lifetime
	RefreshTTL time.Duration // refresh token lifetime
}

// Core holds the business logic for authentication.
type Core struct {
	authStorer  AuthStorer
	userStorer  UserStorer
	emailSender email.Sender
	cfg         Config
}

// NewCore constructs a Core with the given dependencies and configuration.
func NewCore(as AuthStorer, us UserStorer, es email.Sender, cfg Config) *Core {
	return &Core{
		authStorer:  as,
		userStorer:  us,
		emailSender: es,
		cfg:         cfg,
	}
}

// RequestMagicLink sends a sign-in link to emailAddr. If no account exists for
// the address, one is created automatically so that registration and first login
// are the same act.
func (c *Core) RequestMagicLink(ctx context.Context, emailAddr string) error {
	pgUser, err := c.userStorer.UserByEmail(ctx, emailAddr)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("look up user by email: %w", err)
		}
		pgUser, err = c.userStorer.CreateUser(ctx, pguser.CreateUser{Email: emailAddr})
		if err != nil {
			return fmt.Errorf("create user for magic link: %w", err)
		}
	}

	rawToken, tokenHash, err := generateToken()
	if err != nil {
		return fmt.Errorf("generate magic link token: %w", err)
	}

	_, err = c.authStorer.CreateMagicToken(ctx, pgauth.CreateMagicToken{
		UserExternalID: pgUser.ExternalID,
		TokenHash:      tokenHash,
		ExpiresAt:      time.Now().Add(c.cfg.MagicTTL),
	})
	if err != nil {
		return fmt.Errorf("store magic link token: %w", err)
	}

	link := c.cfg.BaseURL + "?token=" + rawToken
	data := magicLinkData{Link: link, TTL: c.cfg.MagicTTL.String()}

	var textBuf, htmlBuf strings.Builder
	if err := magicLinkTextTmpl.Execute(&textBuf, data); err != nil {
		return fmt.Errorf("render magic link text email: %w", err)
	}
	if err := magicLinkHTMLTmpl.Execute(&htmlBuf, data); err != nil {
		return fmt.Errorf("render magic link html email: %w", err)
	}

	msg := email.Message{
		From:     c.cfg.FromEmail,
		To:       []string{emailAddr},
		Subject:  "Your sign-in link",
		TextBody: textBuf.String(),
		HTMLBody: htmlBuf.String(),
	}
	if err := c.emailSender.SendEmail(ctx, msg); err != nil {
		return fmt.Errorf("send magic link email: %w", err)
	}

	return nil
}

// VerifyMagicLink validates rawToken and returns an access/refresh token pair.
// Returns [mdl.ErrTokenInvalid] if the token is expired, consumed, or not found.
func (c *Core) VerifyMagicLink(ctx context.Context, rawToken string) (mdl.TokenPair, error) {
	hash := hashToken(rawToken)

	magicToken, err := c.authStorer.MagicTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.TokenPair{}, mdl.ErrTokenInvalid
		}
		return mdl.TokenPair{}, fmt.Errorf("look up magic token: %w", err)
	}

	if err := c.authStorer.ConsumeMagicToken(ctx, magicToken.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Token was consumed by a concurrent request.
			return mdl.TokenPair{}, mdl.ErrTokenInvalid
		}
		return mdl.TokenPair{}, fmt.Errorf("consume magic token: %w", err)
	}

	return c.issueTokenPair(ctx, magicToken.UserExternalID)
}

// RefreshAccessToken rotates rawToken and returns a new access/refresh token pair.
// The old refresh token is revoked regardless of whether issuing the new pair succeeds.
// Returns [mdl.ErrTokenInvalid] if the token is expired, revoked, or not found.
func (c *Core) RefreshAccessToken(ctx context.Context, rawToken string) (mdl.TokenPair, error) {
	hash := hashToken(rawToken)

	refreshToken, err := c.authStorer.RefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.TokenPair{}, mdl.ErrTokenInvalid
		}
		return mdl.TokenPair{}, fmt.Errorf("look up refresh token: %w", err)
	}

	if err := c.authStorer.RevokeRefreshToken(ctx, refreshToken.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Token was revoked by a concurrent request — possible token reuse.
			return mdl.TokenPair{}, mdl.ErrTokenInvalid
		}
		return mdl.TokenPair{}, fmt.Errorf("revoke old refresh token: %w", err)
	}

	return c.issueTokenPair(ctx, refreshToken.UserExternalID)
}

// RevokeRefreshToken invalidates rawToken, ending that session.
// Returns [mdl.ErrTokenInvalid] if the token is not found or already revoked.
func (c *Core) RevokeRefreshToken(ctx context.Context, rawToken string) error {
	hash := hashToken(rawToken)

	refreshToken, err := c.authStorer.RefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrTokenInvalid
		}
		return fmt.Errorf("look up refresh token: %w", err)
	}

	if err := c.authStorer.RevokeRefreshToken(ctx, refreshToken.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrTokenInvalid
		}
		return fmt.Errorf("revoke refresh token: %w", err)
	}

	return nil
}

// issueTokenPair mints a signed JWT access token and a new opaque refresh token
// for userID, persists the refresh token, and returns both.
func (c *Core) issueTokenPair(ctx context.Context, userID uuid.UUID) (mdl.TokenPair, error) {
	now := time.Now()

	claims := mdl.AuthClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(c.cfg.AccessTTL)),
		},
	}

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.cfg.JWTKey)
	if err != nil {
		return mdl.TokenPair{}, fmt.Errorf("sign access token: %w", err)
	}

	rawRefresh, refreshHash, err := generateToken()
	if err != nil {
		return mdl.TokenPair{}, fmt.Errorf("generate refresh token: %w", err)
	}

	_, err = c.authStorer.CreateRefreshToken(ctx, pgauth.CreateRefreshToken{
		UserExternalID: userID,
		TokenHash:      refreshHash,
		ExpiresAt:      now.Add(c.cfg.RefreshTTL),
	})
	if err != nil {
		return mdl.TokenPair{}, fmt.Errorf("store refresh token: %w", err)
	}

	return mdl.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		ExpiresIn:    c.cfg.AccessTTL,
	}, nil
}

// generateToken returns a cryptographically random URL-safe token and its
// SHA-256 hex digest. Only the digest is stored; the raw token is sent to the
// client and never persisted.
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
