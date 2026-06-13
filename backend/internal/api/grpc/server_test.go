package grpc

import (
	"context"
	"errors"
	"math"
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

// testJWTKey is the HMAC key used to sign JWTs in tests. All test servers use
// this key; authCtx mints tokens with the same key so they pass validation.
var (
	testJWTKey      = []byte("test-jwt-key-for-grpc-tests")
	testJWTIssuer   = "theapp-test"
	testJWTAudience = "theapp-api-test"
)

type ServerTest struct {
	userServiceClient pb.UserServiceClient
	authServiceClient pb.AuthServiceClient
	jwtKey            []byte
}

func NewServerTest(t *testing.T, cfg ServerConfig) ServerTest {
	t.Helper()

	cfg.JWTKey = testJWTKey
	cfg.JWTIssuer = testJWTIssuer
	cfg.JWTAudience = testJWTAudience

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
		authServiceClient: pb.NewAuthServiceClient(conn),
		jwtKey:            cfg.JWTKey,
	}
}

// authCtx returns ctx with a valid JWT Bearer token in the gRPC outgoing
// metadata. Use it for calls to any protected endpoint in tests.
func (s ServerTest) authCtx(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	claims := mdl.AuthClaims{
		UserID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testJWTIssuer,
			Audience:  jwt.ClaimStrings{testJWTAudience},
			Subject:   "00000000-0000-0000-0000-000000000001",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtKey)
	if err != nil {
		t.Fatalf("authCtx: sign JWT: %v", err)
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

// bearerCtxWithClaims returns a context carrying a Bearer token signed with testJWTKey
// and the given claims.
func bearerCtxWithClaims(t *testing.T, claims mdl.AuthClaims) context.Context {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testJWTKey)
	if err != nil {
		t.Fatalf("bearerCtxWithClaims: sign JWT: %v", err)
	}
	return metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer "+token)
}

// invalidBearerCtx returns ctx with a syntactically valid but cryptographically
// invalid Bearer token, exercising the token-validation branch of the interceptor.
func invalidBearerCtx(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer not.a.valid.jwt")
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
