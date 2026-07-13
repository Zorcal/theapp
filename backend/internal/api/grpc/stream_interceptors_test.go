package grpc

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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
