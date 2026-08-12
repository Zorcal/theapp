package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestCore_integration(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)
	core := NewCore(pgauth.NewStore(pool), userStore, rbacStore, pgdb.NewTransactor(pool), testConfig())

	seedUser(t, ctx, userStore, "alice@test.com")

	// MagicLinkToken — a provisioned user receives a token.
	firstToken, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"})
	if err != nil {
		t.Fatalf("MagicLinkToken() error = %v", err)
	}
	if firstToken == "" {
		t.Fatal("MagicLinkToken() = empty, want non-empty token")
	}

	// MagicLinkToken again while first token is still live — first token must be invalidated.
	secondToken, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"})
	if err != nil {
		t.Fatalf("MagicLinkToken() second call error = %v", err)
	}
	if secondToken == firstToken {
		t.Fatal("MagicLinkToken() second call did not issue a new token")
	}
	if _, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: firstToken}); !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("VerifyMagicLink() first token after re-request error = %v, want mdl.ErrTokenInvalid", err)
	}

	// VerifyMagicLink — consumes the second token and returns a token pair.
	pair, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: secondToken})
	if err != nil {
		t.Fatalf("VerifyMagicLink() error = %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("VerifyMagicLink() AccessToken is empty")
	}
	if pair.RefreshToken == "" {
		t.Error("VerifyMagicLink() RefreshToken is empty")
	}

	// VerifyMagicLink marks the email as verified.
	aliceUser, err := userStore.UserByEmail(ctx, "alice@test.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v", err)
	}
	if aliceUser.EmailVerifiedAt == nil {
		t.Error("VerifyMagicLink() EmailVerifiedAt = nil, want non-nil")
	}

	// VerifyMagicLink again — token is consumed, must return ErrTokenInvalid.
	if _, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: secondToken}); !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("VerifyMagicLink() second use error = %v, want mdl.ErrTokenInvalid", err)
	}

	// RefreshAccessToken — old token is revoked and a new pair is issued.
	newPair, err := core.RefreshAccessToken(ctx, mdl.RefreshToken{Token: pair.RefreshToken})
	if err != nil {
		t.Fatalf("RefreshAccessToken() error = %v", err)
	}
	if newPair.RefreshToken == "" {
		t.Error("RefreshAccessToken() RefreshToken is empty")
	}

	// RefreshAccessToken with the old token — must return ErrTokenInvalid after rotation.
	if _, err := core.RefreshAccessToken(ctx, mdl.RefreshToken{Token: pair.RefreshToken}); !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("RefreshAccessToken() old token error = %v, want mdl.ErrTokenInvalid", err)
	}

	// RevokeRefreshToken — invalidates the new token.
	if err := core.RevokeRefreshToken(ctx, mdl.RefreshToken{Token: newPair.RefreshToken}); err != nil {
		t.Fatalf("RevokeRefreshToken() error = %v", err)
	}

	// RefreshAccessToken with revoked token — must return ErrTokenInvalid.
	if _, err := core.RefreshAccessToken(ctx, mdl.RefreshToken{Token: newPair.RefreshToken}); !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("RefreshAccessToken() revoked token error = %v, want mdl.ErrTokenInvalid", err)
	}

	// AuthSession — resolves the permissions granted through a system-scope role assignment.
	seedSystemRoleAssignment(t, ctx, rbacStore, aliceUser.ExternalID, "superadmin")

	sess, err := core.AuthSession(ctx, aliceUser.ExternalID, nil)
	if err != nil {
		t.Fatalf("AuthSession() error = %v", err)
	}

	wantSess := mdl.AuthSession{
		User: mdl.AuthUser{
			UserID:      aliceUser.ExternalID,
			Email:       aliceUser.Email,
			Permissions: mdl.AllPermissions(),
		},
	}

	testingx.AssertDiff(t, sess, wantSess, cmp.Options{
		cmpopts.SortSlices(func(a, b mdl.Permission) bool { return a < b }),
	})
}

func TestCore_MagicLinkToken(t *testing.T) {
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

	t.Run("existing user gets token", func(t *testing.T) {
		userStorerMock := &MockedUserStorer{
			UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      "alice@test.com",
				}, nil
			},
		}

		core := NewCore(authStorerMock, userStorerMock, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

		tok, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"})
		if err != nil {
			t.Fatalf("MagicLinkToken() error = %v", err)
		}
		if tok == "" {
			t.Error("MagicLinkToken() = empty, want non-empty token")
		}
	})

	t.Run("email is normalized to lowercase", func(t *testing.T) {
		var lookedUpEmail string
		userStorerMock := &MockedUserStorer{
			UserByEmailFunc: func(_ context.Context, e string) (pguser.User, error) {
				lookedUpEmail = e
				return pguser.User{
					ID:         1,
					ExternalID: userID,
					Email:      e,
				}, nil
			},
		}

		core := NewCore(authStorerMock, userStorerMock, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

		if _, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "Alice@Test.COM"}); err != nil {
			t.Fatalf("MagicLinkToken() error = %v", err)
		}

		if got, want := lookedUpEmail, "alice@test.com"; got != want {
			t.Errorf("MagicLinkToken() looked up email = %q, want %q", got, want)
		}
	})
}

func TestCore_MagicLinkToken_error(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("invalid input", func(t *testing.T) {
		core := NewCore(&MockedAuthStorer{}, &MockedUserStorer{}, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

		if _, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: ""}); !errors.Is(err, mdl.ErrValidation) {
			t.Errorf("MagicLinkToken() error = %v, want mdl.ErrValidation", err)
		}
	})

	t.Run("user lookup", func(t *testing.T) {
		dbErr := errors.New("db error")

		userStorerMock := &MockedUserStorer{
			UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{}, dbErr
			},
		}

		core := NewCore(
			&MockedAuthStorer{},
			userStorerMock,
			&MockedPermissionStorer{},
			immediateTransactor{},
			testConfig(),
		)

		if _, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"}); !errors.Is(err, dbErr) {
			t.Errorf("MagicLinkToken() error = %v, want wrapping %v", err, dbErr)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		userStorer := &MockedUserStorer{
			UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{}, sql.ErrNoRows
			},
		}
		core := NewCore(&MockedAuthStorer{}, userStorer, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

		if _, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "unknown@test.com"}); !errors.Is(err, mdl.ErrNotFound) {
			t.Errorf("MagicLinkToken() error = %v, want %v", err, mdl.ErrNotFound)
		}
	})

	t.Run("user lock", func(t *testing.T) {
		dbErr := errors.New("db error")
		authStorer := &MockedAuthStorer{
			LockUserFunc: func(_ context.Context, _ int) error {
				return dbErr
			},
		}
		userStorer := &MockedUserStorer{
			UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{ID: 1}, nil
			},
		}
		core := NewCore(authStorer, userStorer, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

		if _, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"}); !errors.Is(err, dbErr) {
			t.Errorf("MagicLinkToken() error = %v, want wrapping %v", err, dbErr)
		}
	})

	t.Run("rate limit check", func(t *testing.T) {
		rateLimitErr := errors.New("db error")

		userStorerMock := &MockedUserStorer{
			UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
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
		core := NewCore(authStorerMock, userStorerMock, &MockedPermissionStorer{}, immediateTransactor{}, cfg)

		if _, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"}); !errors.Is(err, rateLimitErr) {
			t.Errorf("MagicLinkToken() error = %v, want wrapping %v", err, rateLimitErr)
		}
	})

	t.Run("rate limited", func(t *testing.T) {
		userStorerMock := &MockedUserStorer{
			UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
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
		core := NewCore(authStorerMock, userStorerMock, &MockedPermissionStorer{}, immediateTransactor{}, cfg)

		tok, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"})
		if !errors.Is(err, mdl.ErrRateLimited) {
			t.Errorf("MagicLinkToken() error = %v, want mdl.ErrRateLimited", err)
		}
		if tok != "" {
			t.Errorf("MagicLinkToken() = %q, want empty", tok)
		}
	})

	t.Run("invalidation", func(t *testing.T) {
		invalidateErr := errors.New("db error")

		userStorerMock := &MockedUserStorer{
			UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
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

		core := NewCore(authStorerMock, userStorerMock, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

		if _, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"}); !errors.Is(err, invalidateErr) {
			t.Errorf("MagicLinkToken() error = %v, want wrapping %v", err, invalidateErr)
		}
	})

	t.Run("token creation", func(t *testing.T) {
		tokenErr := errors.New("db error")

		userStorerMock := &MockedUserStorer{
			UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
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
			&MockedPermissionStorer{},
			immediateTransactor{},
			testConfig(),
		)
		if _, err := core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"}); !errors.Is(err, tokenErr) {
			t.Errorf("MagicLinkToken() error = %v, want wrapping %v", err, tokenErr)
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
		&MockedUserStorer{
			MarkEmailVerifiedFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
		},
		&MockedPermissionStorer{},
		immediateTransactor{},
		testConfig(),
	)

	pair, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: rawToken})
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

	t.Run("invalid input", func(t *testing.T) {
		core := NewCore(&MockedAuthStorer{}, &MockedUserStorer{}, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

		if _, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: ""}); !errors.Is(err, mdl.ErrValidation) {
			t.Errorf("VerifyMagicLink() error = %v, want mdl.ErrValidation", err)
		}
	})

	tests := []struct {
		name       string
		authStorer *MockedAuthStorer
		want       error
	}{
		{
			name: "token not found",
			authStorer: &MockedAuthStorer{
				MagicLinkTokenByHashFunc: func(_ context.Context, _ string) (pgauth.MagicLinkToken, error) {
					return pgauth.MagicLinkToken{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrTokenInvalid,
		},
		{
			name: "lookup",
			authStorer: &MockedAuthStorer{
				MagicLinkTokenByHashFunc: func(_ context.Context, _ string) (pgauth.MagicLinkToken, error) {
					return pgauth.MagicLinkToken{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(
				tt.authStorer,
				&MockedUserStorer{},
				&MockedPermissionStorer{},
				immediateTransactor{},
				testConfig(),
			)
			if _, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: "anytoken"}); !errors.Is(err, tt.want) {
				t.Errorf("VerifyMagicLink() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("concurrent consume", func(t *testing.T) {
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
			&MockedPermissionStorer{},
			immediateTransactor{},
			testConfig(),
		)

		if _, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: rawToken}); !errors.Is(err, mdl.ErrTokenInvalid) {
			t.Errorf("VerifyMagicLink() error = %v, want mdl.ErrTokenInvalid", err)
		}
	})

	t.Run("consume error", func(t *testing.T) {
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
			&MockedPermissionStorer{},
			immediateTransactor{},
			testConfig(),
		)

		if _, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: rawToken}); !errors.Is(err, consumeErr) {
			t.Errorf("VerifyMagicLink() error = %v, want wrapping %v", err, consumeErr)
		}
	})

	t.Run("mark email verified", func(t *testing.T) {
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
				return nil
			},
		}
		userStorerMock := &MockedUserStorer{
			MarkEmailVerifiedFunc: func(_ context.Context, _ uuid.UUID) error {
				return sql.ErrNoRows
			},
		}
		core := NewCore(
			authStorerMock,
			userStorerMock,
			&MockedPermissionStorer{},
			immediateTransactor{},
			testConfig(),
		)

		// The persisted token still references the user through a foreign key, so sql.ErrNoRows
		// must remain an internal error.
		if _, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: rawToken}); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("VerifyMagicLink() error = %v, want wrapping %v", err, sql.ErrNoRows)
		}
	})

	t.Run("refresh token creation", func(t *testing.T) {
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
			&MockedUserStorer{
				MarkEmailVerifiedFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
			},
			&MockedPermissionStorer{},
			immediateTransactor{},
			testConfig(),
		)

		if _, err := core.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: rawToken}); !errors.Is(err, createErr) {
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
		&MockedPermissionStorer{},
		immediateTransactor{},
		testConfig(),
	)

	pair, err := core.RefreshAccessToken(ctx, mdl.RefreshToken{Token: rawToken})
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

	t.Run("invalid input", func(t *testing.T) {
		core := NewCore(&MockedAuthStorer{}, &MockedUserStorer{}, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

		if _, err := core.RefreshAccessToken(ctx, mdl.RefreshToken{Token: ""}); !errors.Is(err, mdl.ErrValidation) {
			t.Errorf("RefreshAccessToken() error = %v, want mdl.ErrValidation", err)
		}
	})

	tests := []struct {
		name       string
		authStorer *MockedAuthStorer
		want       error
	}{
		{
			name: "token not found",
			authStorer: &MockedAuthStorer{
				ConsumeRefreshTokenFunc: func(_ context.Context, _ string) (pgauth.RefreshToken, error) {
					return pgauth.RefreshToken{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrTokenInvalid,
		},
		{
			name: "consume",
			authStorer: &MockedAuthStorer{
				ConsumeRefreshTokenFunc: func(_ context.Context, _ string) (pgauth.RefreshToken, error) {
					return pgauth.RefreshToken{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(
				tt.authStorer,
				&MockedUserStorer{},
				&MockedPermissionStorer{},
				immediateTransactor{},
				testConfig(),
			)
			if _, err := core.RefreshAccessToken(ctx, mdl.RefreshToken{Token: "anytoken"}); !errors.Is(err, tt.want) {
				t.Errorf("RefreshAccessToken() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("refresh token creation", func(t *testing.T) {
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
			&MockedPermissionStorer{},
			immediateTransactor{},
			testConfig(),
		)

		if _, err := core.RefreshAccessToken(ctx, mdl.RefreshToken{Token: rawToken}); !errors.Is(err, createErr) {
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
		&MockedPermissionStorer{},
		immediateTransactor{},
		testConfig(),
	)

	if err := core.RevokeRefreshToken(ctx, mdl.RefreshToken{Token: rawToken}); err != nil {
		t.Fatalf("RevokeRefreshToken() error = %v", err)
	}
}

func TestCore_RevokeRefreshToken_error(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("db error")

	t.Run("invalid input", func(t *testing.T) {
		core := NewCore(&MockedAuthStorer{}, &MockedUserStorer{}, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

		if err := core.RevokeRefreshToken(ctx, mdl.RefreshToken{Token: ""}); !errors.Is(err, mdl.ErrValidation) {
			t.Errorf("RevokeRefreshToken() error = %v, want mdl.ErrValidation", err)
		}
	})

	tests := []struct {
		name       string
		authStorer *MockedAuthStorer
		want       error
	}{
		{
			name: "token not found or already revoked",
			authStorer: &MockedAuthStorer{
				ConsumeRefreshTokenFunc: func(_ context.Context, _ string) (pgauth.RefreshToken, error) {
					return pgauth.RefreshToken{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrTokenInvalid,
		},
		{
			name: "store",
			authStorer: &MockedAuthStorer{
				ConsumeRefreshTokenFunc: func(_ context.Context, _ string) (pgauth.RefreshToken, error) {
					return pgauth.RefreshToken{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(
				tt.authStorer,
				&MockedUserStorer{},
				&MockedPermissionStorer{},
				immediateTransactor{},
				testConfig(),
			)
			if err := core.RevokeRefreshToken(ctx, mdl.RefreshToken{Token: "anytoken"}); !errors.Is(err, tt.want) {
				t.Errorf("RevokeRefreshToken() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_RevokeAllUserRefreshTokens(t *testing.T) {
	ctx := context.Background()
	authStorerMock := &MockedAuthStorer{
		RevokeAllUserRefreshTokensFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
	}

	core := NewCore(authStorerMock, &MockedUserStorer{}, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

	if err := core.RevokeAllUserRefreshTokens(ctx, uuid.New()); err != nil {
		t.Fatalf("RevokeAllUserRefreshTokens() error = %v", err)
	}
}

func TestCore_RevokeAllUserRefreshTokens_error(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("db error")

	authStorerMock := &MockedAuthStorer{
		RevokeAllUserRefreshTokensFunc: func(_ context.Context, _ uuid.UUID) error {
			return dbErr
		},
	}

	core := NewCore(authStorerMock, &MockedUserStorer{}, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

	if err := core.RevokeAllUserRefreshTokens(ctx, uuid.New()); !errors.Is(err, dbErr) {
		t.Errorf("RevokeAllUserRefreshTokens() error = %v, want wrapping %v", err, dbErr)
	}
}

func TestCore_AuthSession(t *testing.T) {
	userID := uuid.New()

	t.Run("system scope", func(t *testing.T) {
		authStorer := &MockedAuthStorer{
			AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
				return pgauth.AuthSessionData{UserExternalID: userID, Email: "alice@test.com"}, nil
			},
		}
		permissionStorer := &MockedPermissionStorer{
			SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
				return []string{"user:create", "user:read"}, nil
			},
		}
		core := NewCore(authStorer, &MockedUserStorer{}, permissionStorer, immediateTransactor{}, testConfig())

		got, err := core.AuthSession(t.Context(), userID, nil)
		if err != nil {
			t.Fatalf("AuthSession() error = %v", err)
		}

		want := mdl.AuthSession{
			User: mdl.AuthUser{
				UserID:      userID,
				Email:       "alice@test.com",
				Permissions: []mdl.Permission{mdl.PermissionUserCreate, mdl.PermissionUserRead},
			},
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project scope", func(t *testing.T) {
		authStorer := &MockedAuthStorer{
			AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
				return pgauth.AuthSessionData{
					UserExternalID:   userID,
					Email:            "alice@test.com",
					ProjectID:        new(42),
					OrgID:            new(5),
					OrgName:          new("acme"),
					IsControlProject: new(true),
					IsOrgMember:      new(true),
				}, nil
			},
		}
		permissionStorer := &MockedPermissionStorer{
			ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
				return pgrbac.ProjectPermissions{
					OrgID:           5,
					PermissionNames: []string{"user:create", "user:read"},
				}, nil
			},
		}
		core := NewCore(authStorer, &MockedUserStorer{}, permissionStorer, immediateTransactor{}, testConfig())

		got, err := core.AuthSession(t.Context(), userID, new(42))
		if err != nil {
			t.Fatalf("AuthSession() error = %v", err)
		}

		want := mdl.AuthSession{
			User: mdl.AuthUser{
				UserID:      userID,
				Email:       "alice@test.com",
				Permissions: []mdl.Permission{mdl.PermissionUserCreate, mdl.PermissionUserRead},
			},
			Project: &mdl.AuthProject{
				ID:          42,
				OrgID:       5,
				OrgName:     "acme",
				IsControl:   true,
				IsOrgMember: true,
			},
		}

		testingx.AssertDiff(t, got, want)
	})
}

func TestCore_AuthSession_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name             string
		authStorer       *MockedAuthStorer
		permissionStorer *MockedPermissionStorer
		projectID        *int
		want             error
	}{
		{
			name: "session data not found",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{}, sql.ErrNoRows
				},
			},
			permissionStorer: &MockedPermissionStorer{},
			want:             mdl.ErrNotFound,
		},
		{
			name: "system permissions not found",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{UserExternalID: uuid.New()}, nil
				},
			},
			permissionStorer: &MockedPermissionStorer{
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "project permissions not found",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{
						UserExternalID:   uuid.New(),
						ProjectID:        new(42),
						OrgID:            new(5),
						OrgName:          new("acme"),
						IsControlProject: new(false),
						IsOrgMember:      new(true),
					}, nil
				},
			},
			permissionStorer: &MockedPermissionStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{}, sql.ErrNoRows
				},
			},
			projectID: new(42),
			want:      mdl.ErrNotFound,
		},
		{
			name: "session data store",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{}, dbErr
				},
			},
			permissionStorer: &MockedPermissionStorer{},
			want:             dbErr,
		},
		{
			name: "system permissions store",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{UserExternalID: uuid.New()}, nil
				},
			},
			permissionStorer: &MockedPermissionStorer{
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "project permissions store",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{
						UserExternalID:   uuid.New(),
						ProjectID:        new(42),
						OrgID:            new(5),
						OrgName:          new("acme"),
						IsControlProject: new(false),
						IsOrgMember:      new(true),
					}, nil
				},
			},
			permissionStorer: &MockedPermissionStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{}, dbErr
				},
			},
			projectID: new(42),
			want:      dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.authStorer, &MockedUserStorer{}, tt.permissionStorer, immediateTransactor{}, testConfig())

			if _, err := core.AuthSession(t.Context(), uuid.New(), tt.projectID); !errors.Is(err, tt.want) {
				t.Errorf("AuthSession() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_OrganizationAuthSession(t *testing.T) {
	userID := uuid.New()
	authStorer := &MockedAuthStorer{
		AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
			return pgauth.AuthSessionData{
				UserExternalID:   userID,
				Email:            "alice@test.com",
				ProjectID:        new(42),
				OrgID:            new(5),
				OrgName:          new("acme"),
				IsControlProject: new(true),
				IsOrgMember:      new(true),
			}, nil
		},
	}
	permissionStorer := &MockedPermissionStorer{
		OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
			return pgrbac.OrgPermissions{
				OrgID:           5,
				PermissionNames: []string{"custom-role:assign-org", "custom-role:read-org-assignments"},
			}, nil
		},
	}
	core := NewCore(authStorer, &MockedUserStorer{}, permissionStorer, immediateTransactor{}, testConfig())

	got, err := core.OrganizationAuthSession(t.Context(), userID, 42)
	if err != nil {
		t.Fatalf("OrganizationAuthSession() error = %v", err)
	}

	want := mdl.AuthSession{
		User: mdl.AuthUser{
			UserID: userID,
			Email:  "alice@test.com",
			Permissions: []mdl.Permission{
				mdl.PermissionCustomRoleAssignOrg,
				mdl.PermissionCustomRoleReadOrgAssignments,
			},
		},
		Project: &mdl.AuthProject{
			ID:          42,
			OrgID:       5,
			OrgName:     "acme",
			IsControl:   true,
			IsOrgMember: true,
		},
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_OrganizationAuthSession_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name             string
		authStorer       *MockedAuthStorer
		permissionStorer *MockedPermissionStorer
		want             error
	}{
		{
			name: "session data not found",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{}, sql.ErrNoRows
				},
			},
			permissionStorer: &MockedPermissionStorer{},
			want:             mdl.ErrNotFound,
		},
		{
			name: "organization permissions not found",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{
						UserExternalID:   uuid.New(),
						ProjectID:        new(42),
						OrgID:            new(5),
						OrgName:          new("acme"),
						IsControlProject: new(false),
						IsOrgMember:      new(true),
					}, nil
				},
			},
			permissionStorer: &MockedPermissionStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "session data store",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{}, dbErr
				},
			},
			permissionStorer: &MockedPermissionStorer{},
			want:             dbErr,
		},
		{
			name: "organization permissions store",
			authStorer: &MockedAuthStorer{
				AuthSessionDataFunc: func(_ context.Context, _ uuid.UUID, _ *int) (pgauth.AuthSessionData, error) {
					return pgauth.AuthSessionData{
						UserExternalID:   uuid.New(),
						ProjectID:        new(42),
						OrgID:            new(5),
						OrgName:          new("acme"),
						IsControlProject: new(false),
						IsOrgMember:      new(true),
					}, nil
				},
			},
			permissionStorer: &MockedPermissionStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.authStorer, &MockedUserStorer{}, tt.permissionStorer, immediateTransactor{}, testConfig())

			if _, err := core.OrganizationAuthSession(t.Context(), uuid.New(), 42); !errors.Is(err, tt.want) {
				t.Errorf("OrganizationAuthSession() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_AuthContext(t *testing.T) {
	userID := uuid.New()
	mockedOutput := pgrbac.PermissionsByScope{
		ProjectPermissionNames: []string{"custom-role:read", "custom-role:update"},
		OrgPermissionNames:     []string{"custom-role:read"},
		SystemPermissionNames:  []string{"user:read"},
	}
	permissionStorer := &MockedPermissionStorer{
		PermissionsByScopeFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.PermissionsByScope, error) {
			return mockedOutput, nil
		},
	}
	core := NewCore(&MockedAuthStorer{}, &MockedUserStorer{}, permissionStorer, immediateTransactor{}, testConfig())
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User: mdl.AuthUser{
			UserID:      userID,
			Email:       "alice@test.com",
			Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
		},
		Project: &mdl.AuthProject{ID: 42, OrgID: 5},
	})

	got, err := core.AuthContext(ctx)
	if err != nil {
		t.Fatalf("AuthContext() error = %v", err)
	}

	want := mdl.AuthContext{
		UserID:                  userID,
		Email:                   "alice@test.com",
		ProjectPermissions:      []mdl.Permission{mdl.PermissionCustomRoleRead, mdl.PermissionCustomRoleUpdate},
		OrganizationPermissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
		SystemPermissions:       []mdl.Permission{mdl.PermissionUserRead},
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_AuthContext_error(t *testing.T) {
	dbErr := errors.New("db error")
	userID := uuid.New()
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User:    mdl.AuthUser{UserID: userID},
		Project: &mdl.AuthProject{ID: 42},
	})

	tests := []struct {
		name             string
		permissionStorer *MockedPermissionStorer
		want             error
	}{
		{
			name: "not found",
			permissionStorer: &MockedPermissionStorer{
				PermissionsByScopeFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.PermissionsByScope, error) {
					return pgrbac.PermissionsByScope{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "store error",
			permissionStorer: &MockedPermissionStorer{
				PermissionsByScopeFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.PermissionsByScope, error) {
					return pgrbac.PermissionsByScope{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(&MockedAuthStorer{}, &MockedUserStorer{}, tt.permissionStorer, immediateTransactor{}, testConfig())

			if _, err := core.AuthContext(ctx); !errors.Is(err, tt.want) {
				t.Errorf("AuthContext() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  t.Context(),
			},
			{
				name: "project context missing",
				ctx: mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
					User: mdl.AuthUser{UserID: userID},
				}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedAuthStorer{}, &MockedUserStorer{}, &MockedPermissionStorer{}, immediateTransactor{}, testConfig())

				if _, err := core.AuthContext(tt.ctx); err == nil {
					t.Error("AuthContext() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_txRollback(t *testing.T) {
	// Verifies that a failure inside the transaction rolls back the preceding
	// write, leaving the credential reusable on retry.

	t.Run("VerifyMagicLink leaves magic link reusable on CreateRefreshToken failure", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)

		createErr := errors.New("db error")
		realAuthStore := pgauth.NewStore(pool)
		realUserStore := pguser.NewStore(pool)
		realRBACStore := pgrbac.NewStore(pool)
		seedUser(t, ctx, realUserStore, "alice@test.com")

		// Get a raw token using the real store.
		coreSetup := NewCore(realAuthStore, realUserStore, realRBACStore, pgdb.NewTransactor(pool), testConfig())
		magicTok, err := coreSetup.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "alice@test.com"})
		if err != nil {
			t.Fatalf("MagicLinkToken() error = %v", err)
		}

		coreFailStore := NewCore(
			&failingRefreshTokenStorer{
				Store: realAuthStore,
				err:   createErr,
			},
			realUserStore,
			realRBACStore,
			pgdb.NewTransactor(pool),
			testConfig(),
		)

		// CreateRefreshToken fails → tx rolls back → ConsumeMagicLinkToken is undone.
		if _, err := coreFailStore.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: magicTok}); !errors.Is(err, createErr) {
			t.Fatalf("VerifyMagicLink() error = %v, want wrapping %v", err, createErr)
		}

		// Same token must still be consumable.
		coreReal := NewCore(realAuthStore, realUserStore, realRBACStore, pgdb.NewTransactor(pool), testConfig())
		if _, err := coreReal.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: magicTok}); err != nil {
			t.Errorf("VerifyMagicLink() after rollback error = %v, want nil", err)
		}
	})

	t.Run("RefreshAccessToken leaves refresh token valid on CreateRefreshToken failure", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)

		realAuthStore := pgauth.NewStore(pool)
		realUserStore := pguser.NewStore(pool)
		realRBACStore := pgrbac.NewStore(pool)
		seedUser(t, ctx, realUserStore, "bob@test.com")
		coreReal := NewCore(realAuthStore, realUserStore, realRBACStore, pgdb.NewTransactor(pool), testConfig())

		magicTok, err := coreReal.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: "bob@test.com"})
		if err != nil {
			t.Fatalf("MagicLinkToken() error = %v", err)
		}
		pair, err := coreReal.VerifyMagicLink(ctx, mdl.VerifyMagicLink{Token: magicTok})
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
			realRBACStore,
			pgdb.NewTransactor(pool),
			testConfig(),
		)

		// CreateRefreshToken fails → tx rolls back → ConsumeRefreshToken is undone.
		if _, err := coreFailStore.RefreshAccessToken(ctx, mdl.RefreshToken{Token: refreshTok}); !errors.Is(err, createErr) {
			t.Fatalf("RefreshAccessToken() error = %v, want wrapping %v", err, createErr)
		}

		// Old refresh token must still be valid.
		if _, err := coreReal.RefreshAccessToken(ctx, mdl.RefreshToken{Token: refreshTok}); err != nil {
			t.Errorf("RefreshAccessToken() after rollback error = %v, want nil", err)
		}
	})
}

type immediateTransactor struct{}

func (immediateTransactor) RunTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

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

func seedSystemRoleAssignment(t *testing.T, ctx context.Context, s *pgrbac.Store, userID uuid.UUID, roleName string) {
	t.Helper()

	if err := s.AssignSystemRole(ctx, userID, roleName); err != nil {
		t.Fatalf("seed system role assignment: %v", err)
	}
}

func seedUser(t *testing.T, ctx context.Context, store *pguser.Store, email string) pguser.User {
	t.Helper()

	user, err := store.CreateUser(ctx, pguser.CreateUser{Email: email})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return user
}

// failingRefreshTokenStorer wraps a real Store but always returns err from CreateRefreshToken.
type failingRefreshTokenStorer struct {
	*pgauth.Store

	err error
}

func (s *failingRefreshTokenStorer) CreateRefreshToken(_ context.Context, _ pgauth.CreateRefreshToken) (pgauth.RefreshToken, error) {
	return pgauth.RefreshToken{}, s.err
}
