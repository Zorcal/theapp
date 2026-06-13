package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/email"
)

func TestCore_flow(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)

	var capturedToken string
	sender := sendEmailFunc(func(_ context.Context, m email.Message) error {
		parts := strings.SplitN(m.TextBody, "?token=", 2)
		capturedToken = strings.TrimSpace(parts[1])
		return nil
	})

	core := NewCore(
		pgauth.NewStore(pool),
		pguser.NewStore(pool),
		sender,
		pgdb.NewTransactor(pool),
		testConfig(),
	)

	// RequestMagicLink — new user is created and receives a link.
	if err := core.RequestMagicLink(ctx, "alice@test.com"); err != nil {
		t.Fatalf("RequestMagicLink() error = %v", err)
	}
	if capturedToken == "" {
		t.Fatal("RequestMagicLink() did not capture token from email")
	}
	firstToken := capturedToken

	// RequestMagicLink again while first token is still live — first token must be invalidated.
	if err := core.RequestMagicLink(ctx, "alice@test.com"); err != nil {
		t.Fatalf("RequestMagicLink() second call error = %v", err)
	}
	secondToken := capturedToken
	if secondToken == firstToken {
		t.Fatal("RequestMagicLink() second call did not issue a new token")
	}
	if _, err := core.VerifyMagicLink(ctx, firstToken); !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("VerifyMagicLink() first token after re-request error = %v, want mdl.ErrTokenInvalid", err)
	}

	// VerifyMagicLink — consumes the second token and returns a token pair.
	pair, err := core.VerifyMagicLink(ctx, secondToken)
	if err != nil {
		t.Fatalf("VerifyMagicLink() error = %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("VerifyMagicLink() AccessToken is empty")
	}
	if pair.RefreshToken == "" {
		t.Error("VerifyMagicLink() RefreshToken is empty")
	}

	// VerifyMagicLink again — token is consumed, must return ErrTokenInvalid.
	if _, err := core.VerifyMagicLink(ctx, secondToken); !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("VerifyMagicLink() second use error = %v, want mdl.ErrTokenInvalid", err)
	}

	// RefreshAccessToken — old token is revoked and a new pair is issued.
	newPair, err := core.RefreshAccessToken(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshAccessToken() error = %v", err)
	}
	if newPair.RefreshToken == "" {
		t.Error("RefreshAccessToken() RefreshToken is empty")
	}

	// RefreshAccessToken with the old token — must return ErrTokenInvalid after rotation.
	if _, err := core.RefreshAccessToken(ctx, pair.RefreshToken); !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("RefreshAccessToken() old token error = %v, want mdl.ErrTokenInvalid", err)
	}

	// RevokeRefreshToken — invalidates the new token.
	if err := core.RevokeRefreshToken(ctx, newPair.RefreshToken); err != nil {
		t.Fatalf("RevokeRefreshToken() error = %v", err)
	}

	// RefreshAccessToken with revoked token — must return ErrTokenInvalid.
	if _, err := core.RefreshAccessToken(ctx, newPair.RefreshToken); !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("RefreshAccessToken() revoked token error = %v, want mdl.ErrTokenInvalid", err)
	}
}

func TestCore_RequestMagicLink(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	authStorerMock := &MockedAuthStorer{
		LockUserFunc: func(_ context.Context, _ int) error {
			return nil
		},
		LatestMagicLinkTokenCreatedAtFunc: func(_ context.Context, _ int) (time.Time, error) {
			return time.Time{}, sql.ErrNoRows
		},
		InvalidateUserMagicLinkTokensFunc: func(_ context.Context, _ int) error {
			return nil
		},
		CreateMagicLinkTokenFunc: func(_ context.Context, cm pgauth.CreateMagicLinkToken) (pgauth.MagicLinkToken, error) {
			return pgauth.MagicLinkToken{
				ID:        1,
				UserID:    cm.UserID,
				ExpiresAt: cm.ExpiresAt,
			}, nil
		},
	}

	captureEmail := func() (sender email.Sender, sentTo func() string) {
		var to string
		return sendEmailFunc(func(_ context.Context, m email.Message) error {
			to = m.To[0]
			return nil
		}), func() string { return to }
	}

	t.Run("existing user gets link", func(t *testing.T) {
		sender, sentTo := captureEmail()

		userStorerMock := &MockedUserStorer{
			GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      "alice@test.com",
				}, nil
			},
		}

		core := NewCore(authStorerMock, userStorerMock, sender, noopTransactor{}, testConfig())

		if err := core.RequestMagicLink(ctx, "alice@test.com"); err != nil {
			t.Fatalf("RequestMagicLink() error = %v", err)
		}

		if got, want := sentTo(), "alice@test.com"; got != want {
			t.Errorf("RequestMagicLink() email sent to %q, want %q", got, want)
		}
	})

	t.Run("new user is created and gets link", func(t *testing.T) {
		sender, sentTo := captureEmail()

		userStorerMock := &MockedUserStorer{
			GetOrCreateUserByEmailFunc: func(_ context.Context, email string) (pguser.User, error) {
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      email,
				}, nil
			},
		}

		core := NewCore(authStorerMock, userStorerMock, sender, noopTransactor{}, testConfig())

		if err := core.RequestMagicLink(ctx, "new@test.com"); err != nil {
			t.Fatalf("RequestMagicLink() error = %v", err)
		}

		if got, want := sentTo(), "new@test.com"; got != want {
			t.Errorf("RequestMagicLink() email sent to %q, want %q", got, want)
		}
	})

	t.Run("email is normalized to lowercase", func(t *testing.T) {
		var lookedUpEmail string
		userStorerMock := &MockedUserStorer{
			GetOrCreateUserByEmailFunc: func(_ context.Context, e string) (pguser.User, error) {
				lookedUpEmail = e
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      e,
				}, nil
			},
		}

		core := NewCore(authStorerMock, userStorerMock, noopEmail, noopTransactor{}, testConfig())

		if err := core.RequestMagicLink(ctx, "Alice@Test.COM"); err != nil {
			t.Fatalf("RequestMagicLink() error = %v", err)
		}

		if got, want := lookedUpEmail, "alice@test.com"; got != want {
			t.Errorf("RequestMagicLink() looked up email = %q, want %q", got, want)
		}
	})
}

func TestCore_RequestMagicLink_error(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("get or create user fails", func(t *testing.T) {
		dbErr := errors.New("db error")

		userStorerMock := &MockedUserStorer{
			GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{}, dbErr
			},
		}

		core := NewCore(
			&MockedAuthStorer{},
			userStorerMock,
			noopEmail,
			noopTransactor{},
			testConfig(),
		)

		if err := core.RequestMagicLink(ctx, "alice@test.com"); !errors.Is(err, dbErr) {
			t.Errorf("RequestMagicLink() error = %v, want wrapping %v", err, dbErr)
		}
	})

	t.Run("rate limit check fails", func(t *testing.T) {
		rateLimitErr := errors.New("db error")

		userStorerMock := &MockedUserStorer{
			GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      "alice@test.com",
				}, nil
			},
		}
		authStorerMock := &MockedAuthStorer{
			LockUserFunc: func(_ context.Context, _ int) error {
				return nil
			},
			LatestMagicLinkTokenCreatedAtFunc: func(_ context.Context, _ int) (time.Time, error) {
				return time.Time{}, rateLimitErr
			},
		}

		cfg := testConfig()
		cfg.MagicLinkRateLimit = time.Minute
		core := NewCore(authStorerMock, userStorerMock, noopEmail, noopTransactor{}, cfg)

		if err := core.RequestMagicLink(ctx, "alice@test.com"); !errors.Is(err, rateLimitErr) {
			t.Errorf("RequestMagicLink() error = %v, want wrapping %v", err, rateLimitErr)
		}
	})

	t.Run("rate limited silently succeeds", func(t *testing.T) {
		userStorerMock := &MockedUserStorer{
			GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      "alice@test.com",
				}, nil
			},
		}
		authStorerMock := &MockedAuthStorer{
			LockUserFunc: func(_ context.Context, _ int) error {
				return nil
			},
			LatestMagicLinkTokenCreatedAtFunc: func(_ context.Context, _ int) (time.Time, error) {
				return time.Now(), nil // just sent
			},
		}

		cfg := testConfig()
		cfg.MagicLinkRateLimit = time.Minute
		core := NewCore(authStorerMock, userStorerMock, noopEmail, noopTransactor{}, cfg)

		if err := core.RequestMagicLink(ctx, "alice@test.com"); err != nil {
			t.Errorf("RequestMagicLink() rate-limited error = %v, want nil", err)
		}
	})

	t.Run("invalidation fails", func(t *testing.T) {
		invalidateErr := errors.New("db error")

		userStorerMock := &MockedUserStorer{
			GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      "alice@test.com",
				}, nil
			},
		}

		authStorerMock := &MockedAuthStorer{
			LockUserFunc: func(_ context.Context, _ int) error {
				return nil
			},
			LatestMagicLinkTokenCreatedAtFunc: func(_ context.Context, _ int) (time.Time, error) {
				return time.Time{}, sql.ErrNoRows
			},
			InvalidateUserMagicLinkTokensFunc: func(_ context.Context, _ int) error {
				return invalidateErr
			},
		}

		core := NewCore(authStorerMock, userStorerMock, noopEmail, noopTransactor{}, testConfig())

		if err := core.RequestMagicLink(ctx, "alice@test.com"); !errors.Is(err, invalidateErr) {
			t.Errorf("RequestMagicLink() error = %v, want wrapping %v", err, invalidateErr)
		}
	})

	t.Run("token creation fails", func(t *testing.T) {
		tokenErr := errors.New("db error")

		userStorerMock := &MockedUserStorer{
			GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      "alice@test.com",
				}, nil
			},
		}

		authStorerMock := &MockedAuthStorer{
			LockUserFunc: func(_ context.Context, _ int) error {
				return nil
			},
			LatestMagicLinkTokenCreatedAtFunc: func(_ context.Context, _ int) (time.Time, error) {
				return time.Time{}, sql.ErrNoRows
			},
			InvalidateUserMagicLinkTokensFunc: func(_ context.Context, _ int) error {
				return nil
			},
			CreateMagicLinkTokenFunc: func(_ context.Context, _ pgauth.CreateMagicLinkToken) (pgauth.MagicLinkToken, error) {
				return pgauth.MagicLinkToken{}, tokenErr
			},
		}
		core := NewCore(
			authStorerMock,
			userStorerMock,
			noopEmail,
			noopTransactor{},
			testConfig(),
		)
		if err := core.RequestMagicLink(ctx, "alice@test.com"); !errors.Is(err, tokenErr) {
			t.Errorf("RequestMagicLink() error = %v, want wrapping %v", err, tokenErr)
		}
	})

	t.Run("email send fails", func(t *testing.T) {
		sendErr := errors.New("smtp error")

		userStorerMock := &MockedUserStorer{
			GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      "alice@test.com",
				}, nil
			},
		}

		authStorerMock := &MockedAuthStorer{
			LockUserFunc: func(_ context.Context, _ int) error {
				return nil
			},
			LatestMagicLinkTokenCreatedAtFunc: func(_ context.Context, _ int) (time.Time, error) {
				return time.Time{}, sql.ErrNoRows
			},
			InvalidateUserMagicLinkTokensFunc: func(_ context.Context, _ int) error {
				return nil
			},
			CreateMagicLinkTokenFunc: func(_ context.Context, cm pgauth.CreateMagicLinkToken) (pgauth.MagicLinkToken, error) {
				return pgauth.MagicLinkToken{
					ID:        1,
					UserID:    cm.UserID,
					ExpiresAt: cm.ExpiresAt,
				}, nil
			},
		}

		failingSender := sendEmailFunc(func(_ context.Context, _ email.Message) error {
			return sendErr
		})

		core := NewCore(
			authStorerMock,
			userStorerMock,
			failingSender,
			noopTransactor{},
			testConfig(),
		)

		if err := core.RequestMagicLink(ctx, "alice@test.com"); !errors.Is(err, sendErr) {
			t.Errorf("RequestMagicLink() error = %v, want wrapping %v", err, sendErr)
		}
	})
}

func TestCore_VerifyMagicLink(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	rawToken, tokenHash := mustGenerateToken(t)

	authStorerMock := &MockedAuthStorer{
		MagicLinkTokenByHashFunc: func(_ context.Context, hash string) (pgauth.MagicLinkToken, error) {
			if hash != tokenHash {
				return pgauth.MagicLinkToken{}, sql.ErrNoRows
			}
			return pgauth.MagicLinkToken{
				ID:             1,
				UserID:         1,
				UserExternalID: userID,
				ExpiresAt:      time.Now().Add(15 * time.Minute),
			}, nil
		},
		ConsumeMagicLinkTokenFunc: func(_ context.Context, _ int) error {
			return nil
		},
		CreateRefreshTokenFunc: func(_ context.Context, cr pgauth.CreateRefreshToken) (pgauth.RefreshToken, error) {
			return pgauth.RefreshToken{
				ID:        1,
				UserID:    cr.UserID,
				ExpiresAt: cr.ExpiresAt,
				CreatedAt: time.Now(),
			}, nil
		},
	}

	core := NewCore(
		authStorerMock,
		&MockedUserStorer{},
		noopEmail,
		noopTransactor{},
		testConfig(),
	)

	pair, err := core.VerifyMagicLink(ctx, rawToken)
	if err != nil {
		t.Fatalf("VerifyMagicLink() error = %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("VerifyMagicLink() AccessToken is empty")
	}
	if pair.RefreshToken == "" {
		t.Error("VerifyMagicLink() RefreshToken is empty")
	}
	if pair.ExpiresIn <= 0 {
		t.Errorf("VerifyMagicLink() ExpiresIn = %v, want positive", pair.ExpiresIn)
	}

	cfg := testConfig()
	parsed, err := jwt.ParseWithClaims(pair.AccessToken, &mdl.AuthClaims{}, func(t *jwt.Token) (any, error) {
		return cfg.JWTKey, nil
	}, jwt.WithIssuer(cfg.JWTIssuer), jwt.WithAudience(cfg.JWTAudience))
	if err != nil {
		t.Fatalf("VerifyMagicLink() access token parse error = %v", err)
	}

	claims, ok := parsed.Claims.(*mdl.AuthClaims)
	if !ok || !parsed.Valid {
		t.Fatal("VerifyMagicLink() access token invalid")
	}
	if got, want := claims.UserID, userID; got != want {
		t.Errorf("VerifyMagicLink() token UserID = %v, want %v", got, want)
	}
}

func TestCore_VerifyMagicLink_error(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("db error")

	tests := []struct {
		name    string
		mockErr error
		wantErr error
	}{
		{name: "token not found", mockErr: sql.ErrNoRows, wantErr: mdl.ErrTokenInvalid},
		{name: "lookup error propagates", mockErr: dbErr, wantErr: dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authStorerMock := &MockedAuthStorer{
				MagicLinkTokenByHashFunc: func(_ context.Context, _ string) (pgauth.MagicLinkToken, error) {
					return pgauth.MagicLinkToken{}, tt.mockErr
				},
			}
			core := NewCore(
				authStorerMock,
				&MockedUserStorer{},
				noopEmail,
				noopTransactor{},
				testConfig(),
			)
			if _, err := core.VerifyMagicLink(ctx, "anytoken"); !errors.Is(err, tt.wantErr) {
				t.Errorf("VerifyMagicLink() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	t.Run("concurrent consume returns ErrTokenInvalid", func(t *testing.T) {
		rawToken, tokenHash := mustGenerateToken(t)

		authStorerMock := &MockedAuthStorer{
			MagicLinkTokenByHashFunc: func(_ context.Context, hash string) (pgauth.MagicLinkToken, error) {
				if hash != tokenHash {
					return pgauth.MagicLinkToken{}, sql.ErrNoRows
				}
				return pgauth.MagicLinkToken{
					ID:             1,
					UserID:         1,
					UserExternalID: uuid.New(),
					ExpiresAt:      time.Now().Add(15 * time.Minute),
				}, nil
			},
			ConsumeMagicLinkTokenFunc: func(_ context.Context, _ int) error {
				return sql.ErrNoRows
			},
		}

		core := NewCore(
			authStorerMock,
			&MockedUserStorer{},
			noopEmail,
			noopTransactor{},
			testConfig(),
		)

		if _, err := core.VerifyMagicLink(ctx, rawToken); !errors.Is(err, mdl.ErrTokenInvalid) {
			t.Errorf("VerifyMagicLink() error = %v, want mdl.ErrTokenInvalid", err)
		}
	})

	t.Run("consume error propagates", func(t *testing.T) {
		rawToken, tokenHash := mustGenerateToken(t)
		consumeErr := errors.New("db error")

		authStorerMock := &MockedAuthStorer{
			MagicLinkTokenByHashFunc: func(_ context.Context, hash string) (pgauth.MagicLinkToken, error) {
				if hash != tokenHash {
					return pgauth.MagicLinkToken{}, sql.ErrNoRows
				}
				return pgauth.MagicLinkToken{
					ID:             1,
					UserID:         1,
					UserExternalID: uuid.New(),
					ExpiresAt:      time.Now().Add(15 * time.Minute),
				}, nil
			},
			ConsumeMagicLinkTokenFunc: func(_ context.Context, _ int) error {
				return consumeErr
			},
		}

		core := NewCore(
			authStorerMock,
			&MockedUserStorer{},
			noopEmail,
			noopTransactor{},
			testConfig(),
		)

		if _, err := core.VerifyMagicLink(ctx, rawToken); !errors.Is(err, consumeErr) {
			t.Errorf("VerifyMagicLink() error = %v, want wrapping %v", err, consumeErr)
		}
	})

	t.Run("refresh token creation fails", func(t *testing.T) {
		rawToken, tokenHash := mustGenerateToken(t)
		createErr := errors.New("db error")

		authStorerMock := &MockedAuthStorer{
			MagicLinkTokenByHashFunc: func(_ context.Context, hash string) (pgauth.MagicLinkToken, error) {
				if hash != tokenHash {
					return pgauth.MagicLinkToken{}, sql.ErrNoRows
				}
				return pgauth.MagicLinkToken{
					ID:             1,
					UserID:         1,
					UserExternalID: uuid.New(),
					ExpiresAt:      time.Now().Add(15 * time.Minute),
				}, nil
			},
			ConsumeMagicLinkTokenFunc: func(_ context.Context, _ int) error {
				return nil
			},
			CreateRefreshTokenFunc: func(_ context.Context, _ pgauth.CreateRefreshToken) (pgauth.RefreshToken, error) {
				return pgauth.RefreshToken{}, createErr
			},
		}

		core := NewCore(
			authStorerMock,
			&MockedUserStorer{},
			noopEmail,
			noopTransactor{},
			testConfig(),
		)

		if _, err := core.VerifyMagicLink(ctx, rawToken); !errors.Is(err, createErr) {
			t.Errorf("VerifyMagicLink() error = %v, want wrapping %v", err, createErr)
		}
	})
}

func TestCore_RefreshAccessToken(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	rawToken, tokenHash := mustGenerateToken(t)

	authStorerMock := &MockedAuthStorer{
		ConsumeRefreshTokenFunc: func(_ context.Context, hash string) (pgauth.RefreshToken, error) {
			if hash != tokenHash {
				return pgauth.RefreshToken{}, sql.ErrNoRows
			}
			return pgauth.RefreshToken{
				ID:             1,
				UserID:         1,
				UserExternalID: userID,
				ExpiresAt:      time.Now().Add(720 * time.Hour),
				CreatedAt:      time.Now(),
			}, nil
		},
		CreateRefreshTokenFunc: func(_ context.Context, cr pgauth.CreateRefreshToken) (pgauth.RefreshToken, error) {
			return pgauth.RefreshToken{
				ID:        2,
				UserID:    cr.UserID,
				ExpiresAt: cr.ExpiresAt,
				CreatedAt: time.Now(),
			}, nil
		},
	}

	core := NewCore(
		authStorerMock,
		&MockedUserStorer{},
		noopEmail,
		noopTransactor{},
		testConfig(),
	)

	pair, err := core.RefreshAccessToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("RefreshAccessToken() error = %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("RefreshAccessToken() AccessToken is empty")
	}
	if pair.RefreshToken == "" {
		t.Error("RefreshAccessToken() RefreshToken is empty")
	}
}

func TestCore_RefreshAccessToken_error(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("db error")

	tests := []struct {
		name    string
		mockErr error
		wantErr error
	}{
		{name: "token not found", mockErr: sql.ErrNoRows, wantErr: mdl.ErrTokenInvalid},
		{name: "consume error propagates", mockErr: dbErr, wantErr: dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authStorerMock := &MockedAuthStorer{
				ConsumeRefreshTokenFunc: func(_ context.Context, _ string) (pgauth.RefreshToken, error) {
					return pgauth.RefreshToken{}, tt.mockErr
				},
			}
			core := NewCore(
				authStorerMock,
				&MockedUserStorer{},
				noopEmail,
				noopTransactor{},
				testConfig(),
			)
			if _, err := core.RefreshAccessToken(ctx, "anytoken"); !errors.Is(err, tt.wantErr) {
				t.Errorf("RefreshAccessToken() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	t.Run("refresh token creation fails", func(t *testing.T) {
		rawToken, tokenHash := mustGenerateToken(t)
		createErr := errors.New("db error")

		authStorerMock := &MockedAuthStorer{
			ConsumeRefreshTokenFunc: func(_ context.Context, hash string) (pgauth.RefreshToken, error) {
				if hash != tokenHash {
					return pgauth.RefreshToken{}, sql.ErrNoRows
				}
				return pgauth.RefreshToken{
					ID:             1,
					UserID:         1,
					UserExternalID: uuid.New(),
					ExpiresAt:      time.Now().Add(720 * time.Hour),
					CreatedAt:      time.Now(),
				}, nil
			},
			CreateRefreshTokenFunc: func(_ context.Context, _ pgauth.CreateRefreshToken) (pgauth.RefreshToken, error) {
				return pgauth.RefreshToken{}, createErr
			},
		}

		core := NewCore(
			authStorerMock,
			&MockedUserStorer{},
			noopEmail,
			noopTransactor{},
			testConfig(),
		)

		if _, err := core.RefreshAccessToken(ctx, rawToken); !errors.Is(err, createErr) {
			t.Errorf("RefreshAccessToken() error = %v, want wrapping %v", err, createErr)
		}
	})
}

func TestCore_RevokeRefreshToken(t *testing.T) {
	ctx := context.Background()
	rawToken, tokenHash := mustGenerateToken(t)

	authStorerMock := &MockedAuthStorer{
		ConsumeRefreshTokenFunc: func(_ context.Context, hash string) (pgauth.RefreshToken, error) {
			if hash != tokenHash {
				return pgauth.RefreshToken{}, sql.ErrNoRows
			}
			return pgauth.RefreshToken{
				ID:             1,
				UserID:         1,
				UserExternalID: uuid.New(),
				ExpiresAt:      time.Now().Add(720 * time.Hour),
				CreatedAt:      time.Now(),
			}, nil
		},
	}

	core := NewCore(
		authStorerMock,
		&MockedUserStorer{},
		noopEmail,
		noopTransactor{},
		testConfig(),
	)

	if err := core.RevokeRefreshToken(ctx, rawToken); err != nil {
		t.Fatalf("RevokeRefreshToken() error = %v", err)
	}
}

func TestCore_RevokeRefreshToken_error(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("db error")

	tests := []struct {
		name    string
		mockErr error
		wantErr error
	}{
		{name: "token not found or already revoked", mockErr: sql.ErrNoRows, wantErr: mdl.ErrTokenInvalid},
		{name: "consume error propagates", mockErr: dbErr, wantErr: dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authStorerMock := &MockedAuthStorer{
				ConsumeRefreshTokenFunc: func(_ context.Context, _ string) (pgauth.RefreshToken, error) {
					return pgauth.RefreshToken{}, tt.mockErr
				},
			}
			core := NewCore(
				authStorerMock,
				&MockedUserStorer{},
				noopEmail,
				noopTransactor{},
				testConfig(),
			)
			if err := core.RevokeRefreshToken(ctx, "anytoken"); !errors.Is(err, tt.wantErr) {
				t.Errorf("RevokeRefreshToken() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCore_txRollback(t *testing.T) {
	// Verifies that a failure inside the transaction rolls back the preceding
	// write, leaving the credential reusable on retry.

	captureToken := func(t *testing.T) (sender email.Sender, token func() string) {
		t.Helper()
		var captured string
		return sendEmailFunc(func(_ context.Context, m email.Message) error {
			parts := strings.SplitN(m.TextBody, "?token=", 2)
			captured = strings.TrimSpace(parts[1])
			return nil
		}), func() string { return captured }
	}

	t.Run("VerifyMagicLink leaves magic link reusable on CreateRefreshToken failure", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		sender, tok := captureToken(t)

		createErr := errors.New("db error")
		realAuthStore := pgauth.NewStore(pool)

		coreFailStore := NewCore(
			&failingRefreshTokenStorer{
				Store: realAuthStore,
				err:   createErr,
			},
			pguser.NewStore(pool),
			sender,
			pgdb.NewTransactor(pool),
			testConfig(),
		)
		if err := coreFailStore.RequestMagicLink(ctx, "alice@test.com"); err != nil {
			t.Fatalf("RequestMagicLink() error = %v", err)
		}
		magicTok := tok()

		// CreateRefreshToken fails → tx rolls back → ConsumeMagicLinkToken is undone.
		if _, err := coreFailStore.VerifyMagicLink(ctx, magicTok); !errors.Is(err, createErr) {
			t.Fatalf("VerifyMagicLink() error = %v, want wrapping %v", err, createErr)
		}

		// Same token must still be consumable.
		coreReal := NewCore(realAuthStore, pguser.NewStore(pool), sender, pgdb.NewTransactor(pool), testConfig())
		if _, err := coreReal.VerifyMagicLink(ctx, magicTok); err != nil {
			t.Errorf("VerifyMagicLink() after rollback error = %v, want nil", err)
		}
	})

	t.Run("RefreshAccessToken leaves refresh token valid on CreateRefreshToken failure", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		sender, tok := captureToken(t)

		realAuthStore := pgauth.NewStore(pool)
		realUserStore := pguser.NewStore(pool)
		coreReal := NewCore(realAuthStore, realUserStore, sender, pgdb.NewTransactor(pool), testConfig())

		if err := coreReal.RequestMagicLink(ctx, "bob@test.com"); err != nil {
			t.Fatalf("RequestMagicLink() error = %v", err)
		}
		pair, err := coreReal.VerifyMagicLink(ctx, tok())
		if err != nil {
			t.Fatalf("VerifyMagicLink() error = %v", err)
		}
		refreshTok := pair.RefreshToken

		createErr := errors.New("db error")
		coreFailStore := NewCore(
			&failingRefreshTokenStorer{
				Store: realAuthStore,
				err:   createErr,
			},
			realUserStore,
			sender,
			pgdb.NewTransactor(pool),
			testConfig(),
		)

		// CreateRefreshToken fails → tx rolls back → ConsumeRefreshToken is undone.
		if _, err := coreFailStore.RefreshAccessToken(ctx, refreshTok); !errors.Is(err, createErr) {
			t.Fatalf("RefreshAccessToken() error = %v, want wrapping %v", err, createErr)
		}

		// Old refresh token must still be valid.
		if _, err := coreReal.RefreshAccessToken(ctx, refreshTok); err != nil {
			t.Errorf("RefreshAccessToken() after rollback error = %v, want nil", err)
		}
	})
}

type noopTransactor struct{}

func (noopTransactor) RunTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// sendEmailFunc adapts a function to the email.Sender interface.
type sendEmailFunc func(ctx context.Context, m email.Message) error

func (f sendEmailFunc) SendEmail(ctx context.Context, m email.Message) error { return f(ctx, m) }

var noopEmail sendEmailFunc = func(_ context.Context, _ email.Message) error { return nil }

func testConfig() Config {
	return Config{
		JWTKey:             []byte("test-secret"),
		JWTIssuer:          "theapp-test",
		JWTAudience:        "theapp-api-test",
		MagicLinkFromEmail: "noreply@test.com",
		MagicLinkBaseURL:   "http://localhost:3000/auth/verify",
		MagicLinkTTL:       15 * time.Minute,
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    720 * time.Hour,
	}
}

func mustGenerateToken(t *testing.T) (raw, hash string) {
	t.Helper()
	raw, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}
	return raw, hash
}

// failingRefreshTokenStorer wraps a real Store but always returns err from CreateRefreshToken.
type failingRefreshTokenStorer struct {
	*pgauth.Store

	err error
}

func (s *failingRefreshTokenStorer) CreateRefreshToken(_ context.Context, _ pgauth.CreateRefreshToken) (pgauth.RefreshToken, error) {
	return pgauth.RefreshToken{}, s.err
}
