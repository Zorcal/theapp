package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestErrorUnaryInterceptor_validationEscaped(t *testing.T) {
	interceptor := errorUnaryInterceptor(testingx.NewLogger(t))

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, fmt.Errorf("name required: %w", mdl.ErrValidation)
	}

	_, err := interceptor(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/theapp.v1.UserService/CreateUser"}, handler)
	if err == nil {
		t.Fatal("errorUnaryInterceptor() error = nil, want error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("errorUnaryInterceptor() error = %q, want a gRPC status error", err)
	}

	if got, want := st.Code(), codes.InvalidArgument; got != want {
		t.Errorf("errorUnaryInterceptor() code = %v, want %v", got, want)
	}
	if got, want := st.Message(), "invalid request"; got != want {
		t.Errorf("errorUnaryInterceptor() message = %q, want %q", got, want)
	}
}

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
				_, err := s.userServiceClient.GetUser(authCtxWithInvalidToken(t.Context()), &pb.GetUserRequest{Id: "any"})
				return err
			},
		},
		{
			name: "wrong issuer",
			call: func(s ServerTest) error {
				ctx := authCtxWithClaims(t, t.Context(),
					mdl.AuthClaims{
						RegisteredClaims: jwt.RegisteredClaims{
							Issuer:    "wrong-issuer",
							Audience:  jwt.ClaimStrings{testJWTAudience},
							ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
						},
					},
					testJWTKey,
				)
				_, err := s.userServiceClient.GetUser(ctx, &pb.GetUserRequest{Id: "any"})
				return err
			},
		},
		{
			name: "wrong audience",
			call: func(s ServerTest) error {
				ctx := authCtxWithClaims(t, t.Context(),
					mdl.AuthClaims{
						RegisteredClaims: jwt.RegisteredClaims{
							Issuer:    testJWTIssuer,
							Audience:  jwt.ClaimStrings{"wrong-audience"},
							ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
						},
					},
					testJWTKey,
				)
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

func TestScopedWorkflowID_deterministic(t *testing.T) {
	ctx := contextWithUserID(t.Context(), uuid.New())
	method := "/theapp.v1.AuthService/RequestMagicLink"
	payload := []byte("payload")
	key := uuid.NewString()

	got1, err := scopedWorkflowID(ctx, method, payload, key)
	if err != nil {
		t.Fatalf("scopedWorkflowID() error = %v, want nil", err)
	}
	got2, err := scopedWorkflowID(ctx, method, payload, key)
	if err != nil {
		t.Fatalf("scopedWorkflowID() error = %v, want nil", err)
	}

	testingx.AssertDiff(t, got1, got2, defaultDiffOpts())
}

func TestScopedWorkflowID_scoping(t *testing.T) {
	baseUserID := uuid.New()
	baseMethod := "/theapp.v1.AuthService/RequestMagicLink"
	basePayload := []byte("payload-a")
	baseKey := uuid.NewString()

	base, err := scopedWorkflowID(contextWithUserID(t.Context(), baseUserID), baseMethod, basePayload, baseKey)
	if err != nil {
		t.Fatalf("scopedWorkflowID() error = %v, want nil", err)
	}

	tests := []struct {
		name     string
		userID   uuid.UUID
		unauthed bool
		method   string
		payload  []byte
		key      string
	}{
		{
			name:    "different user",
			userID:  uuid.New(),
			method:  baseMethod,
			payload: basePayload,
			key:     baseKey,
		},
		{
			name:     "unauthenticated caller",
			unauthed: true,
			method:   baseMethod,
			payload:  basePayload,
			key:      baseKey,
		},
		{
			name:    "different method",
			userID:  baseUserID,
			method:  "/theapp.v1.AuthService/VerifyMagicLink",
			payload: basePayload,
			key:     baseKey,
		},
		{
			name:    "different payload",
			userID:  baseUserID,
			method:  baseMethod,
			payload: []byte("payload-b"),
			key:     baseKey,
		},
		{
			name:    "different key",
			userID:  baseUserID,
			method:  baseMethod,
			payload: basePayload,
			key:     uuid.NewString(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if !tt.unauthed {
				ctx = contextWithUserID(ctx, tt.userID)
			}

			got, err := scopedWorkflowID(ctx, tt.method, tt.payload, tt.key)
			if err != nil {
				t.Fatalf("scopedWorkflowID() error = %v, want nil", err)
			}
			if got == base {
				t.Errorf("scopedWorkflowID() = %q, want different from base %q", got, base)
			}
		})
	}
}

func TestIdempotencyInterceptor_error(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "non-uuid key",
			key:  "not-a-uuid",
		},
		{
			name: "whitespace key",
			key:  " ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:              testingx.NewLogger(t),
				WorkflowAuthCore: &MockedWorkflowAuthCore{},
			})

			ctx := metadata.AppendToOutgoingContext(t.Context(), "x-idempotency-key", tt.key)
			_, err := srvTest.authServiceClient.RequestMagicLink(ctx, &pb.RequestMagicLinkRequest{Email: "alice@test.com"})
			if err == nil {
				t.Fatal("RequestMagicLink() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("RequestMagicLink() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Code(), codes.InvalidArgument, defaultDiffOpts())
		})
	}
}

func TestScopedWorkflowID_error(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "empty key",
			key:  "",
		},
		{
			name: "non-uuid key",
			key:  "not-a-uuid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := scopedWorkflowID(t.Context(), "/theapp.v1.AuthService/RequestMagicLink", nil, tt.key); err == nil {
				t.Errorf("scopedWorkflowID(%q) error = nil, want error", tt.key)
			}
		})
	}
}
