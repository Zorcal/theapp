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

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestErrorStreamInterceptor_validationEscaped(t *testing.T) {
	interceptor := errorStreamInterceptor(testingx.NewLogger(t))

	handler := func(srv any, ss grpc.ServerStream) error {
		return fmt.Errorf("name required: %w", mdl.ErrValidation)
	}

	err := interceptor(nil, fakeServerStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.UserService/CreateUser"}, handler)
	if err == nil {
		t.Fatal("errorStreamInterceptor() error = nil, want error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("errorStreamInterceptor() error = %q, want a gRPC status error", err)
	}

	if got, want := st.Code(), codes.InvalidArgument; got != want {
		t.Errorf("errorStreamInterceptor() code = %v, want %v", got, want)
	}
	if got, want := st.Message(), "invalid request"; got != want {
		t.Errorf("errorStreamInterceptor() message = %q, want %q", got, want)
	}
}

func TestRecoveryStreamInterceptor(t *testing.T) {
	info := &grpc.StreamServerInfo{FullMethod: "/theapp.v1.UserService/GetUser"}
	panicHandler := func(any, grpc.ServerStream) error {
		panic("boom")
	}
	recovery := recoveryStreamInterceptor()
	errorHandler := func(srv any, ss grpc.ServerStream) error {
		return recovery(srv, ss, info, panicHandler)
	}

	err := errorStreamInterceptor(testingx.NewLogger(t))(nil, fakeServerStream{ctx: t.Context()}, info, errorHandler)
	if got, want := status.Code(err), codes.Internal; got != want {
		t.Errorf("recovered panic code = %v, want %v", got, want)
	}
}

func TestAuthStreamInterceptor(t *testing.T) {
	t.Run("public method bypasses auth", func(t *testing.T) {
		handler := func(_ any, _ grpc.ServerStream) error {
			return nil
		}

		// authCore has no AuthSessionFunc configured, so it panics if the interceptor tries to
		// resolve an auth session instead of going straight to the handler.
		authCore := &MockedAuthCore{
			AuthSessionFunc: nil,
		}
		interceptor := authStreamInterceptor(testJWTKey, testJWTIssuer, testJWTAudience, authCore)

		if err := interceptor(nil, fakeServerStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.AuthService/RequestMagicLink"}, handler); err != nil {
			t.Errorf("authStreamInterceptor() error = %v, want nil", err)
		}
	})

	t.Run("no-project method resolves session with a nil project id", func(t *testing.T) {
		var gotSess mdl.AuthSession
		handler := func(_ any, ss grpc.ServerStream) error {
			gotSess, _ = mdl.AuthSessionFromContext(ss.Context())
			return nil
		}

		want := mdl.AuthSession{User: mdl.AuthUser{UserID: uuid.New()}}

		authCore := &MockedAuthCore{
			AuthSessionFunc: func(_ context.Context, _ uuid.UUID, _ *int) (mdl.AuthSession, error) {
				return want, nil
			},
		}
		interceptor := authStreamInterceptor(testJWTKey, testJWTIssuer, testJWTAudience, authCore)

		if err := interceptor(nil, fakeServerStream{ctx: validStreamAuthCtx(t)}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.AuthService/RevokeAllSessions"}, handler); err != nil {
			t.Fatalf("authStreamInterceptor() error = %v, want nil", err)
		}
		testingx.AssertDiff(t, gotSess, want)
	})

	t.Run("project-scoped method resolves session with the parsed project id", func(t *testing.T) {
		var gotSess mdl.AuthSession
		handler := func(_ any, ss grpc.ServerStream) error {
			gotSess, _ = mdl.AuthSessionFromContext(ss.Context())
			return nil
		}

		projectID := 7
		want := mdl.AuthSession{
			User:    mdl.AuthUser{UserID: uuid.New()},
			Project: &mdl.AuthProject{ID: projectID},
		}

		authCore := &MockedAuthCore{
			AuthSessionFunc: func(_ context.Context, _ uuid.UUID, _ *int) (mdl.AuthSession, error) {
				return want, nil
			},
		}
		interceptor := authStreamInterceptor(testJWTKey, testJWTIssuer, testJWTAudience, authCore)

		ctx := withStreamProjectID(validStreamAuthCtx(t), "7")
		if err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.ExampleService/ExampleMethod"}, handler); err != nil {
			t.Fatalf("authStreamInterceptor() error = %v, want nil", err)
		}
		testingx.AssertDiff(t, gotSess, want)
	})

	t.Run("organization-scoped method resolves an organization session", func(t *testing.T) {
		var gotSess mdl.AuthSession
		handler := func(_ any, ss grpc.ServerStream) error {
			gotSess, _ = mdl.AuthSessionFromContext(ss.Context())
			return nil
		}

		want := mdl.AuthSession{
			User:    mdl.AuthUser{UserID: uuid.New()},
			Project: &mdl.AuthProject{ID: 7, OrgID: 5},
		}

		authCore := &MockedAuthCore{
			OrganizationAuthSessionFunc: func(_ context.Context, _ uuid.UUID, _ int) (mdl.AuthSession, error) {
				return want, nil
			},
		}
		interceptor := authStreamInterceptor(testJWTKey, testJWTIssuer, testJWTAudience, authCore)

		ctx := withStreamProjectID(validStreamAuthCtx(t), "7")
		if err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{
			FullMethod: "/theapp.v1.RoleService/AssignRoleToOrganization",
		}, handler); err != nil {
			t.Fatalf("authStreamInterceptor() error = %v, want nil", err)
		}

		testingx.AssertDiff(t, gotSess, want)
	})
}

func TestAuthStreamInterceptor_error(t *testing.T) {
	dbErr := errors.New("db error")

	handler := func(_ any, _ grpc.ServerStream) error {
		return nil
	}

	t.Run("caller no longer exists", func(t *testing.T) {
		tests := []struct {
			name       string
			ctx        context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
			fullMethod string
		}{
			{
				name:       "no-project method",
				ctx:        validStreamAuthCtx(t),
				fullMethod: "/theapp.v1.AuthService/RevokeAllSessions",
			},
			{
				name:       "project-scoped method",
				ctx:        withStreamProjectID(validStreamAuthCtx(t), "1"),
				fullMethod: "/theapp.v1.ExampleService/ExampleMethod",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				authCore := &MockedAuthCore{
					AuthSessionFunc: func(_ context.Context, _ uuid.UUID, _ *int) (mdl.AuthSession, error) {
						return mdl.AuthSession{}, mdl.ErrNotFound
					},
				}
				interceptor := authStreamInterceptor(testJWTKey, testJWTIssuer, testJWTAudience, authCore)

				err := interceptor(nil, fakeServerStream{ctx: tt.ctx}, &grpc.StreamServerInfo{FullMethod: tt.fullMethod}, handler)
				if got, want := status.Code(err), codes.Unauthenticated; got != want {
					t.Errorf("authStreamInterceptor() code = %v, want %v", got, want)
				}
			})
		}
	})

	t.Run("resolve auth session store error", func(t *testing.T) {
		tests := []struct {
			name       string
			ctx        context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
			fullMethod string
		}{
			{
				name:       "no-project method",
				ctx:        validStreamAuthCtx(t),
				fullMethod: "/theapp.v1.AuthService/RevokeAllSessions",
			},
			{
				name:       "project-scoped method",
				ctx:        withStreamProjectID(validStreamAuthCtx(t), "1"),
				fullMethod: "/theapp.v1.ExampleService/ExampleMethod",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				authCore := &MockedAuthCore{
					AuthSessionFunc: func(_ context.Context, _ uuid.UUID, _ *int) (mdl.AuthSession, error) {
						return mdl.AuthSession{}, dbErr
					},
				}
				interceptor := authStreamInterceptor(testJWTKey, testJWTIssuer, testJWTAudience, authCore)

				if err := interceptor(nil, fakeServerStream{ctx: tt.ctx}, &grpc.StreamServerInfo{FullMethod: tt.fullMethod}, handler); !errors.Is(err, dbErr) {
					t.Errorf("authStreamInterceptor() error = %v, want %v", err, dbErr)
				}
			})
		}
	})

	t.Run("organization session", func(t *testing.T) {
		tests := []struct {
			name     string
			authCore *MockedAuthCore
			want     codes.Code
		}{
			{
				name: "not found",
				authCore: &MockedAuthCore{
					OrganizationAuthSessionFunc: func(_ context.Context, _ uuid.UUID, _ int) (mdl.AuthSession, error) {
						return mdl.AuthSession{}, mdl.ErrNotFound
					},
				},
				want: codes.Unauthenticated,
			},
			{
				name: "store",
				authCore: &MockedAuthCore{
					OrganizationAuthSessionFunc: func(_ context.Context, _ uuid.UUID, _ int) (mdl.AuthSession, error) {
						return mdl.AuthSession{}, dbErr
					},
				},
				want: codes.Unknown,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				interceptor := authStreamInterceptor(testJWTKey, testJWTIssuer, testJWTAudience, tt.authCore)
				ctx := withStreamProjectID(validStreamAuthCtx(t), "1")

				err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{
					FullMethod: "/theapp.v1.RoleService/AssignRoleToOrganization",
				}, handler)

				if got := status.Code(err); got != tt.want {
					t.Errorf("authStreamInterceptor() code = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("malformed project id", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "missing project id header",
				ctx:  validStreamAuthCtx(t),
			},
			{
				name: "empty project id value",
				ctx:  withStreamProjectID(validStreamAuthCtx(t), ""),
			},
			{
				name: "non-numeric project id",
				ctx:  withStreamProjectID(validStreamAuthCtx(t), "abc"),
			},
			{
				name: "zero project id",
				ctx:  withStreamProjectID(validStreamAuthCtx(t), "0"),
			},
			{
				name: "negative project id",
				ctx:  withStreamProjectID(validStreamAuthCtx(t), "-5"),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				interceptor := authStreamInterceptor(testJWTKey, testJWTIssuer, testJWTAudience, &MockedAuthCore{})

				err := interceptor(nil, fakeServerStream{ctx: tt.ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.ExampleService/ExampleMethod"}, handler)
				if got, want := status.Code(err), codes.InvalidArgument; got != want {
					t.Errorf("authStreamInterceptor() code = %v, want %v", got, want)
				}
			})
		}
	})
}

func TestPermissionStreamInterceptor(t *testing.T) {
	t.Run("public method bypasses check", func(t *testing.T) {
		handler := func(_ any, _ grpc.ServerStream) error {
			return nil
		}

		interceptor := permissionStreamInterceptor()

		err := interceptor(nil, fakeServerStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.AuthService/RequestMagicLink"}, handler)
		if err != nil {
			t.Fatalf("permissionStreamInterceptor() error = %v, want nil", err)
		}
	})

	t.Run("permission granted", func(t *testing.T) {
		var gotSess mdl.AuthSession
		var hasSess bool
		handler := func(_ any, ss grpc.ServerStream) error {
			gotSess, hasSess = mdl.AuthSessionFromContext(ss.Context())
			return nil
		}

		want := mdl.AuthSession{
			User: mdl.AuthUser{
				UserID:      uuid.New(),
				Permissions: []mdl.Permission{mdl.PermissionUserRead},
			},
			Project: &mdl.AuthProject{ID: 1},
		}

		interceptor := permissionStreamInterceptor()

		ctx := mdl.ContextWithAuthSession(t.Context(), want)
		if err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.UserService/GetUser"}, handler); err != nil {
			t.Fatalf("permissionStreamInterceptor() error = %v, want nil", err)
		}

		if !hasSess {
			t.Fatal("AuthSessionFromContext() ok = false, want true")
		}

		testingx.AssertDiff(t, gotSess, want)
	})
}

func TestPermissionStreamInterceptor_error(t *testing.T) {
	handler := func(_ any, _ grpc.ServerStream) error {
		return nil
	}

	tests := []struct {
		name   string
		ctx    context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		method string
		want   codes.Code
	}{
		{
			name:   "unauthenticated",
			ctx:    t.Context(),
			method: "/theapp.v1.UserService/GetUser",
			want:   codes.Unauthenticated,
		},
		{
			name:   "missing permission",
			ctx:    mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{User: mdl.AuthUser{UserID: uuid.New()}}),
			method: "/theapp.v1.UserService/GetUser",
			want:   codes.PermissionDenied,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := permissionStreamInterceptor()

			err := interceptor(nil, fakeServerStream{ctx: tt.ctx}, &grpc.StreamServerInfo{FullMethod: tt.method}, handler)
			if got := status.Code(err); got != tt.want {
				t.Errorf("permissionStreamInterceptor() code = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("unregistered method", func(t *testing.T) {
		interceptor := permissionStreamInterceptor()

		ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{User: mdl.AuthUser{UserID: uuid.New()}})
		err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.UserService/NoSuchMethod"}, handler)
		if err == nil {
			t.Fatal("permissionStreamInterceptor() error = nil, want error")
		}

		if _, ok := status.FromError(err); ok {
			t.Errorf("permissionStreamInterceptor() error = %v, want a plain error, not a gRPC status error", err)
		}
	})
}

func TestSystemControlProjectStreamInterceptor(t *testing.T) {
	t.Run("unprotected method", func(t *testing.T) {
		handler := func(_ any, _ grpc.ServerStream) error {
			return nil
		}

		interceptor := systemControlProjectStreamInterceptor()

		if err := interceptor(nil, fakeServerStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.UserService/GetUser"}, handler); err != nil {
			t.Fatalf("systemControlProjectStreamInterceptor() error = %v, want nil", err)
		}
	})

	t.Run("authorized", func(t *testing.T) {
		handler := func(_ any, _ grpc.ServerStream) error {
			return nil
		}
		interceptor := systemControlProjectStreamInterceptor()

		sess := mdl.AuthSession{Project: &mdl.AuthProject{
			OrgName:     mdl.SystemOrgName,
			IsControl:   true,
			IsOrgMember: true,
		}}

		ctx := mdl.ContextWithAuthSession(t.Context(), sess)
		if err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.SystemRoleService/ListSystemRoles"}, handler); err != nil {
			t.Fatalf("systemControlProjectStreamInterceptor() error = %v, want nil", err)
		}
	})
}

func TestSystemControlProjectStreamInterceptor_error(t *testing.T) {
	handler := func(_ any, _ grpc.ServerStream) error {
		return nil
	}

	t.Run("wrong project", func(t *testing.T) {
		interceptor := systemControlProjectStreamInterceptor()

		sess := mdl.AuthSession{Project: &mdl.AuthProject{OrgName: mdl.SystemOrgName, IsOrgMember: true}}

		ctx := mdl.ContextWithAuthSession(t.Context(), sess)
		err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.SystemRoleService/ListSystemRoles"}, handler)
		if got, want := status.Code(err), codes.PermissionDenied; got != want {
			t.Errorf("systemControlProjectStreamInterceptor() code = %v, want %v", got, want)
		}
	})

	t.Run("not a member", func(t *testing.T) {
		interceptor := systemControlProjectStreamInterceptor()

		sess := mdl.AuthSession{Project: &mdl.AuthProject{OrgName: mdl.SystemOrgName, IsControl: true}}

		ctx := mdl.ContextWithAuthSession(t.Context(), sess)
		err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.SystemRoleService/ListSystemRoles"}, handler)
		if got, want := status.Code(err), codes.PermissionDenied; got != want {
			t.Errorf("systemControlProjectStreamInterceptor() code = %v, want %v", got, want)
		}
	})
}

func TestOrganizationControlProjectStreamInterceptor(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		authSess mdl.AuthSession
	}{
		{
			name:   "unprotected method",
			method: "/theapp.v1.ProjectService/ListProjects",
		},
		{
			name:     "control project",
			method:   "/theapp.v1.OrgService/CreateOrganizationUser",
			authSess: mdl.AuthSession{Project: &mdl.AuthProject{IsControl: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), tt.authSess)
			interceptor := organizationControlProjectStreamInterceptor()
			handler := func(_ any, _ grpc.ServerStream) error { return nil }

			if err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: tt.method}, handler); err != nil {
				t.Errorf("organizationControlProjectStreamInterceptor() error = %v, want nil", err)
			}
		})
	}
}

func TestOrganizationControlProjectStreamInterceptor_error(t *testing.T) {
	interceptor := organizationControlProjectStreamInterceptor()
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	sess := mdl.AuthSession{Project: &mdl.AuthProject{}}
	ctx := mdl.ContextWithAuthSession(t.Context(), sess)
	err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.OrgService/CreateOrganizationUser"}, handler)
	if got, want := status.Code(err), codes.PermissionDenied; got != want {
		t.Errorf("organizationControlProjectStreamInterceptor() code = %v, want %v", got, want)
	}
}

// validStreamAuthCtx returns a context carrying a validly signed JWT Bearer token in the gRPC
// incoming metadata, for tests that need authStreamInterceptor's bearer check to succeed.
func validStreamAuthCtx(t *testing.T) context.Context {
	t.Helper()
	claims := mdl.AuthClaims{
		UserID: uuid.New(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testJWTIssuer,
			Audience:  jwt.ClaimStrings{testJWTAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testJWTKey)
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+token))
}

// fakeServerStream is a minimal grpc.ServerStream stub carrying only a context, sufficient
// for interceptor tests that never send or receive a message.
type fakeServerStream struct {
	grpc.ServerStream

	ctx context.Context //nolint:containedctx // test double, the whole point is to supply a fixed ctx.
}

func (s fakeServerStream) Context() context.Context {
	return s.ctx
}

// withStreamProjectID returns a copy of ctx with the x-project-id metadata key set to id.
func withStreamProjectID(ctx context.Context, id string) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)
	md = md.Copy()
	md.Set("x-project-id", id)
	return metadata.NewIncomingContext(ctx, md)
}
