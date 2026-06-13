package grpc

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestAuthInterceptor_unauthenticated(t *testing.T) {
	tests := []struct {
		name string
		call func(srvTest ServerTest) error
	}{
		{
			name: "missing authorization header",
			call: func(s ServerTest) error {
				_, err := s.userServiceClient.GetUser(t.Context(), &pb.GetUserRequest{Id: "any"})
				return err
			},
		},
		{
			name: "invalid bearer token",
			call: func(s ServerTest) error {
				_, err := s.userServiceClient.GetUser(invalidBearerCtx(t.Context()), &pb.GetUserRequest{Id: "any"})
				return err
			},
		},
		{
			name: "wrong issuer",
			call: func(s ServerTest) error {
				ctx := bearerCtxWithClaims(t, mdl.AuthClaims{
					RegisteredClaims: jwt.RegisteredClaims{
						Issuer:    "wrong-issuer",
						Audience:  jwt.ClaimStrings{testJWTAudience},
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
					},
				})
				_, err := s.userServiceClient.GetUser(ctx, &pb.GetUserRequest{Id: "any"})
				return err
			},
		},
		{
			name: "wrong audience",
			call: func(s ServerTest) error {
				ctx := bearerCtxWithClaims(t, mdl.AuthClaims{
					RegisteredClaims: jwt.RegisteredClaims{
						Issuer:    testJWTIssuer,
						Audience:  jwt.ClaimStrings{"wrong-audience"},
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
					},
				})
				_, err := s.userServiceClient.GetUser(ctx, &pb.GetUserRequest{Id: "any"})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				UserCore: &MockedUserCore{},
			})

			err := tt.call(srvTest)
			if err == nil {
				t.Fatal("call() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("call() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Code(), codes.Unauthenticated, defaultDiffOpts())
		})
	}
}
