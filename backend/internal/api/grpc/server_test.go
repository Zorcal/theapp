package grpc

import (
	"context"
	"errors"
	"math"
	"net"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

type ServerTest struct {
	userServiceClient pb.UserServiceClient
}

func NewServerTest(t *testing.T, cfg ServerConfig) ServerTest {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() {
		if err := lis.Close(); err != nil {
			t.Fatalf("failed to close listener: %s", err)
		}
	})

	srv := NewServer(cfg)
	t.Cleanup(srv.Stop)

	go func() {
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("server error: %s", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect to client: %s", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Fatalf("failed to close client connection: %s", err)
		}
	})

	return ServerTest{
		userServiceClient: pb.NewUserServiceClient(conn),
	}
}

func defaultDiffOpts() cmp.Options {
	return cmp.Options{
		cmp.Comparer(proto.Equal),
		equateApproxTimestamppb(),
	}
}

func equateApproxTimestamppb() cmp.Option {
	tolerance := time.Millisecond * 100
	return cmp.Comparer(func(a, b *timestamppb.Timestamp) bool {
		d := a.AsTime().Sub(b.AsTime())
		return math.Abs(float64(d)) <= float64(tolerance)
	})
}
