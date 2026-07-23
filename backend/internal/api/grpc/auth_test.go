package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

// TestAuth_MagicLinkIntegration exercises the full authentication lifecycle end-to-end: request link → verify →
// use tokens → rotate → revoke.
func TestAuth_MagicLinkIntegration(t *testing.T) {
	srv := NewServerIntegrationTest(t)
	ctx := t.Context()

	// Request a magic link. The auth core creates the user automatically.
	if _, err := srv.authServiceClient.RequestMagicLink(ctx, &pb.RequestMagicLinkRequest{
		Email: "alice@test.com",
	}); err != nil {
		t.Fatalf("RequestMagicLink() error = %v", err)
	}

	// Extract the token from the captured email and verify it.
	token := srv.emailSender.MagicLinkToken(t)
	pair, err := srv.authServiceClient.VerifyMagicLink(ctx, &pb.VerifyMagicLinkRequest{
		Token: token,
	})
	if err != nil {
		t.Fatalf("VerifyMagicLink() error = %v", err)
	}
	if pair.GetAccessToken() == "" {
		t.Error("VerifyMagicLink() access_token = empty, want non-empty")
	}
	if pair.GetRefreshToken() == "" {
		t.Error("VerifyMagicLink() refresh_token = empty, want non-empty")
	}

	// A freshly created user holds no role yet, so a protected endpoint denies them.
	authedCtx := metadata.AppendToOutgoingContext(authCtxWithToken(ctx, pair.GetAccessToken()), "x-project-id", testProjectID)
	if _, err := srv.userServiceClient.ListUsers(authedCtx, &pb.ListUsersRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("ListUsers() before role assignment code = %v, want %v", status.Code(err), codes.PermissionDenied)
	}

	// Access token grants access to protected endpoints, once granted the permission it requires.
	aliceUser, err := srv.userStore.UserByEmail(ctx, "alice@test.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v", err)
	}
	if err := srv.rbacStore.AssignSystemRole(ctx, aliceUser.ExternalID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}

	if _, err := srv.userServiceClient.ListUsers(authedCtx, &pb.ListUsersRequest{}); err != nil {
		t.Fatalf("ListUsers() with valid access token error = %v", err)
	}

	// Refreshing rotates the refresh token.
	newPair, err := srv.authServiceClient.RefreshAccessToken(ctx, &pb.RefreshAccessTokenRequest{
		RefreshToken: pair.GetRefreshToken(),
	})
	if err != nil {
		t.Fatalf("RefreshAccessToken() error = %v", err)
	}
	if newPair.GetRefreshToken() == pair.GetRefreshToken() {
		t.Error("RefreshAccessToken() refresh_token unchanged, want rotated")
	}

	// Consumed refresh token cannot be reused.
	_, err = srv.authServiceClient.RefreshAccessToken(ctx, &pb.RefreshAccessTokenRequest{
		RefreshToken: pair.GetRefreshToken(),
	})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Errorf("RefreshAccessToken() with consumed token code = %v, want %v", code, codes.Unauthenticated)
	}

	// Revoking the current refresh token ends the session.
	if _, err := srv.authServiceClient.RevokeRefreshToken(ctx, &pb.RevokeRefreshTokenRequest{
		RefreshToken: newPair.GetRefreshToken(),
	}); err != nil {
		t.Fatalf("RevokeRefreshToken() error = %v", err)
	}

	// Revoked token cannot be used to obtain new tokens.
	_, err = srv.authServiceClient.RefreshAccessToken(ctx, &pb.RefreshAccessTokenRequest{
		RefreshToken: newPair.GetRefreshToken(),
	})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Errorf("RefreshAccessToken() with revoked token code = %v, want %v", code, codes.Unauthenticated)
	}
}

func TestAuthService_RequestMagicLink(t *testing.T) {
	tests := []struct {
		name             string
		workflowAuthCore WorkflowAuthCore
		in               *pb.RequestMagicLinkRequest
	}{
		{
			name: "sends link",
			workflowAuthCore: &MockedWorkflowAuthCore{
				RequestMagicLinkFunc: func(_ context.Context, _ string) error { return nil },
			},
			in: &pb.RequestMagicLinkRequest{Email: "alice@test.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:              testingx.NewLogger(t),
				WorkflowAuthCore: tt.workflowAuthCore,
			})

			if _, err := srvTest.authServiceClient.RequestMagicLink(t.Context(), tt.in); err != nil {
				t.Fatalf("RequestMagicLink(%q) error = %q, want no error", tt.in.GetEmail(), err)
			}
		})
	}
}

func TestAuthService_RequestMagicLink_error(t *testing.T) {
	tests := []struct {
		name             string
		workflowAuthCore WorkflowAuthCore
		in               *pb.RequestMagicLinkRequest
		want             *status.Status
	}{
		{
			name:             "empty email",
			workflowAuthCore: &MockedWorkflowAuthCore{},
			in:               &pb.RequestMagicLinkRequest{},
			want:             status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:             "malformed email",
			workflowAuthCore: &MockedWorkflowAuthCore{},
			in:               &pb.RequestMagicLinkRequest{Email: "notanemail"},
			want:             status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "core error",
			workflowAuthCore: &MockedWorkflowAuthCore{
				RequestMagicLinkFunc: func(_ context.Context, _ string) error {
					return errors.New("boom")
				},
			},
			in:   &pb.RequestMagicLinkRequest{Email: "alice@test.com"},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:              testingx.NewLogger(t),
				WorkflowAuthCore: tt.workflowAuthCore,
			})

			_, err := srvTest.authServiceClient.RequestMagicLink(t.Context(), tt.in)
			if err == nil {
				t.Fatal("RequestMagicLink() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("RequestMagicLink() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Code(), tt.want.Code(), defaultDiffOpts())
		})
	}
}

func TestAuthService_VerifyMagicLink(t *testing.T) {
	diffOpts := defaultDiffOpts()

	tests := []struct {
		name     string
		authCore AuthCore
		in       *pb.VerifyMagicLinkRequest
		want     *pb.TokenPair
	}{
		{
			name: "returns token pair",
			authCore: &MockedAuthCore{
				VerifyMagicLinkFunc: func(_ context.Context, _ mdl.VerifyMagicLink) (mdl.AuthTokenPair, error) {
					return mdl.AuthTokenPair{
						AccessToken:  "access-token",
						RefreshToken: "refresh-token",
						ExpiresIn:    15 * time.Minute,
					}, nil
				},
			},
			in: &pb.VerifyMagicLinkRequest{Token: "sometoken"},
			want: &pb.TokenPair{
				AccessToken:  "access-token",
				RefreshToken: "refresh-token",
				ExpiresIn:    900,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				AuthCore: tt.authCore,
			})

			got, err := srvTest.authServiceClient.VerifyMagicLink(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("VerifyMagicLink() error = %q, want no error", err)
			}

			testingx.AssertDiff(t, got, tt.want, diffOpts)
		})
	}
}

func TestAuthService_VerifyMagicLink_error(t *testing.T) {
	tests := []struct {
		name     string
		authCore AuthCore
		in       *pb.VerifyMagicLinkRequest
		want     *status.Status
	}{
		{
			name:     "empty token",
			authCore: &MockedAuthCore{},
			in:       &pb.VerifyMagicLinkRequest{},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "token invalid",
			authCore: &MockedAuthCore{
				VerifyMagicLinkFunc: func(_ context.Context, _ mdl.VerifyMagicLink) (mdl.AuthTokenPair, error) {
					return mdl.AuthTokenPair{}, mdl.ErrTokenInvalid
				},
			},
			in:   &pb.VerifyMagicLinkRequest{Token: "expiredtoken"},
			want: status.New(codes.Unauthenticated, "token invalid or expired"),
		},
		{
			name: "core error",
			authCore: &MockedAuthCore{
				VerifyMagicLinkFunc: func(_ context.Context, _ mdl.VerifyMagicLink) (mdl.AuthTokenPair, error) {
					return mdl.AuthTokenPair{}, errors.New("boom")
				},
			},
			in:   &pb.VerifyMagicLinkRequest{Token: "sometoken"},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				AuthCore: tt.authCore,
			})

			_, err := srvTest.authServiceClient.VerifyMagicLink(t.Context(), tt.in)
			if err == nil {
				t.Fatal("VerifyMagicLink() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("VerifyMagicLink() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Code(), tt.want.Code(), defaultDiffOpts())
		})
	}
}

func TestAuthService_RefreshAccessToken(t *testing.T) {
	diffOpts := defaultDiffOpts()

	tests := []struct {
		name     string
		authCore AuthCore
		in       *pb.RefreshAccessTokenRequest
		want     *pb.TokenPair
	}{
		{
			name: "returns new token pair",
			authCore: &MockedAuthCore{
				RefreshAccessTokenFunc: func(_ context.Context, _ mdl.RefreshToken) (mdl.AuthTokenPair, error) {
					return mdl.AuthTokenPair{
						AccessToken:  "new-access",
						RefreshToken: "new-refresh",
						ExpiresIn:    15 * time.Minute,
					}, nil
				},
			},
			in: &pb.RefreshAccessTokenRequest{RefreshToken: "old-refresh"},
			want: &pb.TokenPair{
				AccessToken:  "new-access",
				RefreshToken: "new-refresh",
				ExpiresIn:    900,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				AuthCore: tt.authCore,
			})

			got, err := srvTest.authServiceClient.RefreshAccessToken(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("RefreshAccessToken() error = %q, want no error", err)
			}

			testingx.AssertDiff(t, got, tt.want, diffOpts)
		})
	}
}

func TestAuthService_RefreshAccessToken_error(t *testing.T) {
	tests := []struct {
		name     string
		authCore AuthCore
		in       *pb.RefreshAccessTokenRequest
		want     *status.Status
	}{
		{
			name:     "empty refresh token",
			authCore: &MockedAuthCore{},
			in:       &pb.RefreshAccessTokenRequest{},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "token invalid",
			authCore: &MockedAuthCore{
				RefreshAccessTokenFunc: func(_ context.Context, _ mdl.RefreshToken) (mdl.AuthTokenPair, error) {
					return mdl.AuthTokenPair{}, mdl.ErrTokenInvalid
				},
			},
			in:   &pb.RefreshAccessTokenRequest{RefreshToken: "expired"},
			want: status.New(codes.Unauthenticated, "refresh token invalid, expired, or revoked"),
		},
		{
			name: "core error",
			authCore: &MockedAuthCore{
				RefreshAccessTokenFunc: func(_ context.Context, _ mdl.RefreshToken) (mdl.AuthTokenPair, error) {
					return mdl.AuthTokenPair{}, errors.New("boom")
				},
			},
			in:   &pb.RefreshAccessTokenRequest{RefreshToken: "sometoken"},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				AuthCore: tt.authCore,
			})

			_, err := srvTest.authServiceClient.RefreshAccessToken(t.Context(), tt.in)
			if err == nil {
				t.Fatal("RefreshAccessToken() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("RefreshAccessToken() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Code(), tt.want.Code(), defaultDiffOpts())
		})
	}
}

func TestAuthService_RevokeAllSessions(t *testing.T) {
	srvTest := NewServerTest(t, ServerConfig{
		Log: testingx.NewLogger(t),
		AuthCore: &MockedAuthCore{
			RevokeAllUserRefreshTokensFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
			AuthSessionFunc: func(_ context.Context, userID uuid.UUID, _ *int) (mdl.AuthSession, error) {
				return mdl.AuthSession{User: mdl.AuthUser{UserID: userID}}, nil
			},
		},
	})

	if _, err := srvTest.authServiceClient.RevokeAllSessions(authCtxForTestUser(t, t.Context()), nil); err != nil {
		t.Fatalf("RevokeAllSessions() error = %q, want no error", err)
	}
}

func TestAuthService_RevokeAllSessions_error(t *testing.T) {
	tests := []struct {
		name     string
		authCore AuthCore
		ctxFunc  func(*testing.T) context.Context
		want     *status.Status
	}{
		{
			name:     "no JWT",
			authCore: &MockedAuthCore{},
			want:     status.New(codes.Unauthenticated, ""),
		},
		{
			name: "core error",
			authCore: &MockedAuthCore{
				RevokeAllUserRefreshTokensFunc: func(_ context.Context, _ uuid.UUID) error {
					return errors.New("boom")
				},
				AuthSessionFunc: func(_ context.Context, userID uuid.UUID, _ *int) (mdl.AuthSession, error) {
					return mdl.AuthSession{User: mdl.AuthUser{UserID: userID}}, nil
				},
			},
			ctxFunc: func(t *testing.T) context.Context {
				t.Helper()
				return authCtxForTestUser(t, t.Context())
			},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				AuthCore: tt.authCore,
			})

			ctx := t.Context()
			if tt.ctxFunc != nil {
				ctx = tt.ctxFunc(t)
			}

			_, err := srvTest.authServiceClient.RevokeAllSessions(ctx, nil)
			if err == nil {
				t.Fatal("RevokeAllSessions() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("RevokeAllSessions() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Code(), tt.want.Code(), defaultDiffOpts())
		})
	}
}

func TestAuthService_RevokeRefreshToken(t *testing.T) {
	tests := []struct {
		name    string
		ctxFunc func(*testing.T) context.Context
		in      *pb.RevokeRefreshTokenRequest
	}{
		{
			name: "revokes token",
			ctxFunc: func(t *testing.T) context.Context {
				t.Helper()
				return authCtxForTestUser(t, t.Context())
			},
			in: &pb.RevokeRefreshTokenRequest{RefreshToken: "some-refresh"},
		},
		{
			name: "no JWT required",
			in:   &pb.RevokeRefreshTokenRequest{RefreshToken: "some-refresh"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log: testingx.NewLogger(t),
				AuthCore: &MockedAuthCore{
					RevokeRefreshTokenFunc: func(_ context.Context, _ mdl.RefreshToken) error { return nil },
				},
			})

			ctx := t.Context()
			if tt.ctxFunc != nil {
				ctx = tt.ctxFunc(t)
			}

			if _, err := srvTest.authServiceClient.RevokeRefreshToken(ctx, tt.in); err != nil {
				t.Fatalf("RevokeRefreshToken() error = %q, want no error", err)
			}
		})
	}
}

func TestAuthService_RevokeRefreshToken_error(t *testing.T) {
	tests := []struct {
		name     string
		authCore AuthCore
		in       *pb.RevokeRefreshTokenRequest
		want     *status.Status
	}{
		{
			name:     "empty refresh token",
			authCore: &MockedAuthCore{},
			in:       &pb.RevokeRefreshTokenRequest{},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "token not found",
			authCore: &MockedAuthCore{
				RevokeRefreshTokenFunc: func(_ context.Context, _ mdl.RefreshToken) error {
					return mdl.ErrTokenInvalid
				},
			},
			in:   &pb.RevokeRefreshTokenRequest{RefreshToken: "unknown"},
			want: status.New(codes.NotFound, ""),
		},
		{
			name: "core error",
			authCore: &MockedAuthCore{
				RevokeRefreshTokenFunc: func(_ context.Context, _ mdl.RefreshToken) error {
					return errors.New("boom")
				},
			},
			in:   &pb.RevokeRefreshTokenRequest{RefreshToken: "sometoken"},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				AuthCore: tt.authCore,
			})

			_, err := srvTest.authServiceClient.RevokeRefreshToken(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("RevokeRefreshToken() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("RevokeRefreshToken() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Code(), tt.want.Code(), defaultDiffOpts())
		})
	}
}
