package grpc

import (
	"context"
	"fmt"
	"testing"

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

// fakeServerStream is a minimal grpc.ServerStream stub carrying only a context, sufficient
// for interceptor tests that never send or receive a message.
type fakeServerStream struct {
	grpc.ServerStream

	ctx context.Context //nolint:containedctx // test double, the whole point is to supply a fixed ctx.
}

func (s fakeServerStream) Context() context.Context {
	return s.ctx
}

func TestProjectStreamInterceptor(t *testing.T) {
	t.Run("public method bypasses check", func(t *testing.T) {
		handler := func(_ any, _ grpc.ServerStream) error {
			return nil
		}

		interceptor := projectStreamInterceptor()

		err := interceptor(nil, fakeServerStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.AuthService/RequestMagicLink"}, handler)
		if err != nil {
			t.Fatalf("projectStreamInterceptor() error = %v, want nil", err)
		}
	})

	t.Run("no-project method bypasses check", func(t *testing.T) {
		handler := func(_ any, _ grpc.ServerStream) error {
			return nil
		}

		interceptor := projectStreamInterceptor()

		err := interceptor(nil, fakeServerStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.AuthService/RevokeAllSessions"}, handler)
		if err != nil {
			t.Fatalf("projectStreamInterceptor() error = %v, want nil", err)
		}
	})

	t.Run("project id attached to context", func(t *testing.T) {
		var gotID int
		var gotOK bool
		handler := func(_ any, ss grpc.ServerStream) error {
			gotID, gotOK = projectIDFromContext(ss.Context())
			return nil
		}

		interceptor := projectStreamInterceptor()

		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("x-project-id", "7"))
		if err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.UserService/GetUser"}, handler); err != nil {
			t.Fatalf("projectStreamInterceptor() error = %v, want nil", err)
		}
		if !gotOK {
			t.Fatal("projectIDFromContext() ok = false, want true")
		}
		testingx.AssertDiff(t, gotID, 7)
	})
}

func TestProjectStreamInterceptor_error(t *testing.T) {
	handler := func(_ any, _ grpc.ServerStream) error {
		return nil
	}

	tests := []struct {
		name string
		ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
	}{
		{
			name: "missing metadata",
			ctx:  t.Context(),
		},
		{
			name: "missing project id header",
			ctx:  metadata.NewIncomingContext(t.Context(), metadata.Pairs()),
		},
		{
			name: "empty project id value",
			ctx:  metadata.NewIncomingContext(t.Context(), metadata.Pairs("x-project-id", "")),
		},
		{
			name: "non-numeric project id",
			ctx:  metadata.NewIncomingContext(t.Context(), metadata.Pairs("x-project-id", "abc")),
		},
		{
			name: "zero project id",
			ctx:  metadata.NewIncomingContext(t.Context(), metadata.Pairs("x-project-id", "0")),
		},
		{
			name: "negative project id",
			ctx:  metadata.NewIncomingContext(t.Context(), metadata.Pairs("x-project-id", "-5")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := projectStreamInterceptor()

			err := interceptor(nil, fakeServerStream{ctx: tt.ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.UserService/GetUser"}, handler)
			if got, want := status.Code(err), codes.InvalidArgument; got != want {
				t.Errorf("projectStreamInterceptor() code = %v, want %v", got, want)
			}
		})
	}
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
		var gotUser mdl.AuthUser
		var gotOK bool
		handler := func(_ any, ss grpc.ServerStream) error {
			gotUser, gotOK = mdl.AuthUserFromContext(ss.Context())
			return nil
		}

		want := mdl.AuthUser{UserID: uuid.New(), Permissions: []mdl.Permission{mdl.PermissionUserRead}}
		interceptor := permissionStreamInterceptor()

		ctx := mdl.ContextWithAuthUser(t.Context(), want)
		if err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.UserService/GetUser"}, handler); err != nil {
			t.Fatalf("permissionStreamInterceptor() error = %v, want nil", err)
		}
		if !gotOK {
			t.Fatal("AuthUserFromContext() ok = false, want true")
		}
		testingx.AssertDiff(t, gotUser, want)
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
			ctx:    mdl.ContextWithAuthUser(t.Context(), mdl.AuthUser{UserID: uuid.New()}),
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

		ctx := mdl.ContextWithAuthUser(t.Context(), mdl.AuthUser{UserID: uuid.New()})
		err := interceptor(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/theapp.v1.UserService/NoSuchMethod"}, handler)
		if err == nil {
			t.Fatal("permissionStreamInterceptor() error = nil, want error")
		}

		if _, ok := status.FromError(err); ok {
			t.Errorf("permissionStreamInterceptor() error = %v, want a plain error, not a gRPC status error", err)
		}
	})
}
