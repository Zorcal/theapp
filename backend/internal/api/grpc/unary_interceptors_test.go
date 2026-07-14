package grpc

import (
	"context"
	"errors"
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

// TestPermissionRegistry_exhaustiveness asserts every method registered on the gRPC server is
// either public (see publicMethods) or has a permissionRegistry entry, so a new RPC added without
// registering one fails the build instead of silently defaulting to either extreme.
func TestPermissionRegistry_exhaustiveness(t *testing.T) {
	srv := NewServer(ServerConfig{Log: testingx.NewLogger(t)})
	defer srv.Stop()

	for serviceName, info := range srv.GetServiceInfo() {
		for _, m := range info.Methods {
			method := fmt.Sprintf("/%s/%s", serviceName, m.Name)

			_, public := publicMethods[method]
			_, registered := permissionRegistry[method]

			if !public && !registered {
				t.Errorf("method %q has no permissionRegistry entry and is not public", method)
			}
			if public && registered {
				t.Errorf("method %q is both public and has a permissionRegistry entry", method)
			}
		}
	}
}

func TestPermissionUnaryInterceptor(t *testing.T) {
	handler := func(ctx context.Context, _ any) (any, error) {
		s, ok := AuthSessionFromContext(ctx)
		if !ok {
			return nil, errors.New("no auth session in context")
		}
		return s, nil
	}

	t.Run("public method bypasses check", func(t *testing.T) {
		interceptor := permissionUnaryInterceptor(&MockedAuthCore{})

		_, err := interceptor(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/theapp.v1.AuthService/RequestMagicLink"}, handler)
		if err == nil {
			t.Fatal("permissionUnaryInterceptor() error = nil, want error")
		}
		// The handler ran (and failed for lack of an auth session), proving the permission check itself was skipped.
		testingx.AssertErrContains(t, err, "no auth session in context")
	})

	t.Run("permission granted", func(t *testing.T) {
		userID := uuid.New()
		authCore := &MockedAuthCore{
			AuthUserFunc: func(_ context.Context, id uuid.UUID) (mdl.AuthUser, error) {
				return mdl.AuthUser{UserID: id, Permissions: []mdl.Permission{mdl.PermissionUserRead}}, nil
			},
		}
		interceptor := permissionUnaryInterceptor(authCore)

		ctx := contextWithUserID(t.Context(), userID)
		resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/theapp.v1.UserService/GetUser"}, handler)
		if err != nil {
			t.Fatalf("permissionUnaryInterceptor() error = %v, want nil", err)
		}
		got, ok := resp.(mdl.AuthSession)
		if !ok {
			t.Fatalf("permissionUnaryInterceptor() response = %T, want mdl.AuthSession", resp)
		}

		want := mdl.AuthSession{User: mdl.AuthUser{UserID: userID, Permissions: []mdl.Permission{mdl.PermissionUserRead}}}
		testingx.AssertDiff(t, got, want)
	})
}

func TestPermissionUnaryInterceptor_error(t *testing.T) {
	handler := func(ctx context.Context, _ any) (any, error) {
		return "handler ran", nil
	}

	tests := []struct {
		name     string
		authCore AuthCore
		ctx      context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		method   string
		want     codes.Code
	}{
		{
			name:     "unregistered method",
			authCore: &MockedAuthCore{},
			ctx:      contextWithUserID(t.Context(), uuid.New()),
			method:   "/theapp.v1.UserService/NoSuchMethod",
			want:     codes.PermissionDenied,
		},
		{
			name:     "unauthenticated",
			authCore: &MockedAuthCore{},
			ctx:      t.Context(),
			method:   "/theapp.v1.UserService/GetUser",
			want:     codes.Unauthenticated,
		},
		{
			name: "caller no longer exists",
			authCore: &MockedAuthCore{
				AuthUserFunc: func(_ context.Context, _ uuid.UUID) (mdl.AuthUser, error) {
					return mdl.AuthUser{}, mdl.ErrNotFound
				},
			},
			ctx:    contextWithUserID(t.Context(), uuid.New()),
			method: "/theapp.v1.UserService/GetUser",
			want:   codes.Unauthenticated,
		},
		{
			name: "missing permission",
			authCore: &MockedAuthCore{
				AuthUserFunc: func(_ context.Context, id uuid.UUID) (mdl.AuthUser, error) {
					return mdl.AuthUser{UserID: id}, nil
				},
			},
			ctx:    contextWithUserID(t.Context(), uuid.New()),
			method: "/theapp.v1.UserService/GetUser",
			want:   codes.PermissionDenied,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := permissionUnaryInterceptor(tt.authCore)

			_, err := interceptor(tt.ctx, nil, &grpc.UnaryServerInfo{FullMethod: tt.method}, handler)
			if got := status.Code(err); got != tt.want {
				t.Errorf("permissionUnaryInterceptor() code = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("resolve auth user store error", func(t *testing.T) {
		authCore := &MockedAuthCore{
			AuthUserFunc: func(_ context.Context, _ uuid.UUID) (mdl.AuthUser, error) {
				return mdl.AuthUser{}, errors.New("db down")
			},
		}
		interceptor := permissionUnaryInterceptor(authCore)

		ctx := contextWithUserID(t.Context(), uuid.New())
		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/theapp.v1.UserService/GetUser"}, handler)
		if err == nil {
			t.Fatal("permissionUnaryInterceptor() error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "db down")
	})
}
