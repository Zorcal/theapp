package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestAuthService_RequestMagicLink(t *testing.T) {
	tests := []struct {
		name     string
		authCore AuthCore
		in       *pb.RequestMagicLinkRequest
	}{
		{
			name: "sends link",
			authCore: &MockedAuthCore{
				RequestMagicLinkFunc: func(_ context.Context, _ string) error { return nil },
			},
			in: &pb.RequestMagicLinkRequest{Email: "alice@test.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				AuthCore: tt.authCore,
			})

			_, err := srvTest.authServiceClient.RequestMagicLink(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("RequestMagicLink(%q) error = %q, want no error", tt.in.GetEmail(), err)
			}
		})
	}
}

func TestAuthService_RequestMagicLink_error(t *testing.T) {
	tests := []struct {
		name     string
		authCore AuthCore
		in       *pb.RequestMagicLinkRequest
		want     *status.Status
	}{
		{
			name:     "empty email",
			authCore: &MockedAuthCore{},
			in:       &pb.RequestMagicLinkRequest{},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:     "malformed email",
			authCore: &MockedAuthCore{},
			in:       &pb.RequestMagicLinkRequest{Email: "notanemail"},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "core error",
			authCore: &MockedAuthCore{
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
				Log:      testingx.NewLogger(t),
				AuthCore: tt.authCore,
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
				VerifyMagicLinkFunc: func(_ context.Context, _ string) (mdl.AuthTokenPair, error) {
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
				VerifyMagicLinkFunc: func(_ context.Context, _ string) (mdl.AuthTokenPair, error) {
					return mdl.AuthTokenPair{}, mdl.ErrTokenInvalid
				},
			},
			in:   &pb.VerifyMagicLinkRequest{Token: "expiredtoken"},
			want: status.New(codes.Unauthenticated, "token invalid or expired"),
		},
		{
			name: "core error",
			authCore: &MockedAuthCore{
				VerifyMagicLinkFunc: func(_ context.Context, _ string) (mdl.AuthTokenPair, error) {
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
				RefreshAccessTokenFunc: func(_ context.Context, _ string) (mdl.AuthTokenPair, error) {
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
				RefreshAccessTokenFunc: func(_ context.Context, _ string) (mdl.AuthTokenPair, error) {
					return mdl.AuthTokenPair{}, mdl.ErrTokenInvalid
				},
			},
			in:   &pb.RefreshAccessTokenRequest{RefreshToken: "expired"},
			want: status.New(codes.Unauthenticated, "refresh token invalid, expired, or revoked"),
		},
		{
			name: "core error",
			authCore: &MockedAuthCore{
				RefreshAccessTokenFunc: func(_ context.Context, _ string) (mdl.AuthTokenPair, error) {
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
		},
	})

	if _, err := srvTest.authServiceClient.RevokeAllSessions(srvTest.authCtx(t, t.Context()), nil); err != nil {
		t.Fatalf("RevokeAllSessions() error = %q, want no error", err)
	}
}

func TestAuthService_RevokeAllSessions_error(t *testing.T) {
	tests := []struct {
		name     string
		authCore AuthCore
		ctxFunc  func(*testing.T, ServerTest) context.Context
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
			},
			ctxFunc: func(t *testing.T, s ServerTest) context.Context {
				t.Helper()
				return s.authCtx(t, t.Context())
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
				ctx = tt.ctxFunc(t, srvTest)
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
		ctxFunc func(*testing.T, ServerTest) context.Context
		in      *pb.RevokeRefreshTokenRequest
	}{
		{
			name: "revokes token",
			ctxFunc: func(t *testing.T, s ServerTest) context.Context {
				t.Helper()
				return s.authCtx(t, t.Context())
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
					RevokeRefreshTokenFunc: func(_ context.Context, _ string) error { return nil },
				},
			})

			ctx := t.Context()
			if tt.ctxFunc != nil {
				ctx = tt.ctxFunc(t, srvTest)
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
				RevokeRefreshTokenFunc: func(_ context.Context, _ string) error {
					return mdl.ErrTokenInvalid
				},
			},
			in:   &pb.RevokeRefreshTokenRequest{RefreshToken: "unknown"},
			want: status.New(codes.NotFound, ""),
		},
		{
			name: "core error",
			authCore: &MockedAuthCore{
				RevokeRefreshTokenFunc: func(_ context.Context, _ string) error {
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

			_, err := srvTest.authServiceClient.RevokeRefreshToken(srvTest.authCtx(t, t.Context()), tt.in)
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
