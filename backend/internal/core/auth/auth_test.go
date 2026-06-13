package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/email"
)

// sendEmailFunc adapts a function to the email.Sender interface.
type sendEmailFunc func(ctx context.Context, m email.Message) error

func (f sendEmailFunc) SendEmail(ctx context.Context, m email.Message) error { return f(ctx, m) }

func testConfig() Config {
	return Config{
		JWTKey:     []byte("test-secret"),
		FromEmail:  "noreply@example.com",
		BaseURL:    "http://localhost:3000/auth/verify",
		MagicTTL:   15 * time.Minute,
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	}
}

func TestCore_RequestMagicLink(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	tests := []struct {
		name        string
		emailAddr   string
		userByEmail func(ctx context.Context, email string) (pguser.User, error)
		createUser  func(ctx context.Context, cu pguser.CreateUser) (pguser.User, error)
		emailSentTo string
	}{
		{
			name:      "existing user gets link",
			emailAddr: "alice@test.com",
			userByEmail: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{ExternalID: userID, Email: "alice@test.com"}, nil
			},
			emailSentTo: "alice@test.com",
		},
		{
			name:      "new user is created and gets link",
			emailAddr: "new@test.com",
			userByEmail: func(_ context.Context, _ string) (pguser.User, error) {
				return pguser.User{}, sql.ErrNoRows
			},
			createUser: func(_ context.Context, cu pguser.CreateUser) (pguser.User, error) {
				return pguser.User{ExternalID: userID, Email: cu.Email}, nil
			},
			emailSentTo: "new@test.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sentTo string
			emailSender := sendEmailFunc(func(_ context.Context, m email.Message) error {
				sentTo = m.To[0]
				return nil
			})

			authStorerMock := &MockedAuthStorer{
				CreateMagicTokenFunc: func(_ context.Context, cm pgauth.CreateMagicToken) (pgauth.MagicToken, error) {
					return pgauth.MagicToken{ID: 1, UserExternalID: cm.UserExternalID, ExpiresAt: cm.ExpiresAt}, nil
				},
			}

			createUser := tt.createUser
			if createUser == nil {
				createUser = func(_ context.Context, _ pguser.CreateUser) (pguser.User, error) {
					t.Error("CreateUser called unexpectedly")
					return pguser.User{}, nil
				}
			}

			userStorerMock := &MockedUserStorer{
				UserByEmailFunc: tt.userByEmail,
				CreateUserFunc:  createUser,
			}

			core := NewCore(authStorerMock, userStorerMock, emailSender, testConfig())

			if err := core.RequestMagicLink(ctx, tt.emailAddr); err != nil {
				t.Fatalf("RequestMagicLink(%q) error = %v", tt.emailAddr, err)
			}

			if sentTo != tt.emailSentTo {
				t.Errorf("RequestMagicLink(%q) email sent to %q, want %q", tt.emailAddr, sentTo, tt.emailSentTo)
			}
		})
	}
}

func TestCore_VerifyMagicLink(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	rawToken, tokenHash, _ := generateToken()

	authStorerMock := &MockedAuthStorer{
		MagicTokenByHashFunc: func(_ context.Context, hash string) (pgauth.MagicToken, error) {
			if hash != tokenHash {
				return pgauth.MagicToken{}, sql.ErrNoRows
			}
			return pgauth.MagicToken{ID: 1, UserExternalID: userID, ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
		},
		ConsumeMagicTokenFunc: func(_ context.Context, _ int64) error {
			return nil
		},
		CreateRefreshTokenFunc: func(_ context.Context, cr pgauth.CreateRefreshToken) (pgauth.RefreshToken, error) {
			return pgauth.RefreshToken{ID: 1, UserExternalID: cr.UserExternalID, ExpiresAt: cr.ExpiresAt, CreatedAt: time.Now()}, nil
		},
	}

	core := NewCore(authStorerMock, &MockedUserStorer{}, sendEmailFunc(func(_ context.Context, _ email.Message) error { return nil }), testConfig())

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
}

func TestCore_VerifyMagicLink_error(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		storeLookupFn func(_ context.Context, hash string) (pgauth.MagicToken, error)
	}{
		{
			name: "token not found",
			storeLookupFn: func(_ context.Context, _ string) (pgauth.MagicToken, error) {
				return pgauth.MagicToken{}, sql.ErrNoRows
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authStorerMock := &MockedAuthStorer{
				MagicTokenByHashFunc: tt.storeLookupFn,
			}

			core := NewCore(authStorerMock, &MockedUserStorer{}, sendEmailFunc(func(_ context.Context, _ email.Message) error { return nil }), testConfig())

			_, err := core.VerifyMagicLink(ctx, "anytoken")
			if !errors.Is(err, mdl.ErrTokenInvalid) {
				t.Errorf("VerifyMagicLink() error = %v, want mdl.ErrTokenInvalid", err)
			}
		})
	}
}

func TestCore_RefreshAccessToken(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	rawToken, tokenHash, _ := generateToken()

	authStorerMock := &MockedAuthStorer{
		RefreshTokenByHashFunc: func(_ context.Context, hash string) (pgauth.RefreshToken, error) {
			if hash != tokenHash {
				return pgauth.RefreshToken{}, sql.ErrNoRows
			}
			return pgauth.RefreshToken{ID: 1, UserExternalID: userID, ExpiresAt: time.Now().Add(720 * time.Hour), CreatedAt: time.Now()}, nil
		},
		RevokeRefreshTokenFunc: func(_ context.Context, _ int64) error {
			return nil
		},
		CreateRefreshTokenFunc: func(_ context.Context, cr pgauth.CreateRefreshToken) (pgauth.RefreshToken, error) {
			return pgauth.RefreshToken{ID: 2, UserExternalID: cr.UserExternalID, ExpiresAt: cr.ExpiresAt, CreatedAt: time.Now()}, nil
		},
	}

	core := NewCore(authStorerMock, &MockedUserStorer{}, sendEmailFunc(func(_ context.Context, _ email.Message) error { return nil }), testConfig())

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

	authStorerMock := &MockedAuthStorer{
		RefreshTokenByHashFunc: func(_ context.Context, _ string) (pgauth.RefreshToken, error) {
			return pgauth.RefreshToken{}, sql.ErrNoRows
		},
	}

	core := NewCore(authStorerMock, &MockedUserStorer{}, sendEmailFunc(func(_ context.Context, _ email.Message) error { return nil }), testConfig())

	_, err := core.RefreshAccessToken(ctx, "expiredtoken")
	if !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("RefreshAccessToken() error = %v, want mdl.ErrTokenInvalid", err)
	}
}

func TestCore_RevokeRefreshToken(t *testing.T) {
	ctx := context.Background()
	rawToken, tokenHash, _ := generateToken()

	authStorerMock := &MockedAuthStorer{
		RefreshTokenByHashFunc: func(_ context.Context, hash string) (pgauth.RefreshToken, error) {
			if hash != tokenHash {
				return pgauth.RefreshToken{}, sql.ErrNoRows
			}
			return pgauth.RefreshToken{ID: 1, UserExternalID: uuid.New(), ExpiresAt: time.Now().Add(720 * time.Hour), CreatedAt: time.Now()}, nil
		},
		RevokeRefreshTokenFunc: func(_ context.Context, _ int64) error {
			return nil
		},
	}

	core := NewCore(authStorerMock, &MockedUserStorer{}, sendEmailFunc(func(_ context.Context, _ email.Message) error { return nil }), testConfig())

	if err := core.RevokeRefreshToken(ctx, rawToken); err != nil {
		t.Fatalf("RevokeRefreshToken() error = %v", err)
	}
}

func TestCore_RevokeRefreshToken_error(t *testing.T) {
	ctx := context.Background()

	authStorerMock := &MockedAuthStorer{
		RefreshTokenByHashFunc: func(_ context.Context, _ string) (pgauth.RefreshToken, error) {
			return pgauth.RefreshToken{}, sql.ErrNoRows
		},
	}

	core := NewCore(authStorerMock, &MockedUserStorer{}, sendEmailFunc(func(_ context.Context, _ email.Message) error { return nil }), testConfig())

	err := core.RevokeRefreshToken(ctx, "unknown")
	if !errors.Is(err, mdl.ErrTokenInvalid) {
		t.Errorf("RevokeRefreshToken() error = %v, want mdl.ErrTokenInvalid", err)
	}
}
