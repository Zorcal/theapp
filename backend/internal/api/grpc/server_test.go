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
	"github.com/zorcal/theapp/backend/internal/core/auth"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/core/user"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
	workflowsauth "github.com/zorcal/theapp/backend/internal/workflows/auth"
	"github.com/zorcal/theapp/backend/internal/workflows/dbostest"
)

// testJWTKey is the HMAC key used to sign JWTs in tests. All test servers use
// this key; authCtx mints tokens with the same key so they pass validation.
var (
	testJWTKey      = []byte("test-jwt-key-for-grpc-tests")
	testJWTIssuer   = "theapp-test"
	testJWTAudience = "theapp-api-test"
)

// testProjectID is the fixed project ID authCtxForTestUser sends as x-project-id metadata.
const testProjectID = "1"

// ServerTest is a test harness for the gRPC server using mock cores. Use NewServerTest to construct one.
type ServerTest struct {
	userServiceClient pb.UserServiceClient
	authServiceClient pb.AuthServiceClient
}

// NewServerTest starts a gRPC server with the given config over an in-memory transport and returns
// a harness with pre-wired clients. The JWT config is always overridden with test credentials. AuthCore
// defaults to a mock granting every permission, so tests exercising unrelated RPCs don't each need to
// stub out permission resolution; tests targeting permission enforcement itself set AuthCore explicitly.
func NewServerTest(t *testing.T, cfg ServerConfig) ServerTest {
	t.Helper()

	cfg.JWTKey = testJWTKey
	cfg.JWTIssuer = testJWTIssuer
	cfg.JWTAudience = testJWTAudience

	if cfg.AuthCore == nil {
		cfg.AuthCore = &MockedAuthCore{
			AuthSessionFunc: func(_ context.Context, userID uuid.UUID, projectID *int) (mdl.AuthSession, error) {
				return mdl.AuthSession{
					User: mdl.AuthUser{
						UserID:      userID,
						Permissions: mdl.AllPermissions(),
					},
					ProjectID: projectID,
				}, nil
			},
		}
	}

	conn := newBufconnClientConn(t, cfg)

	return ServerTest{
		userServiceClient: pb.NewUserServiceClient(conn),
		authServiceClient: pb.NewAuthServiceClient(conn),
	}
}

// ServerIntegrationTest is a test harness for the gRPC server using real cores backed by a real
// PostgreSQL database. Use NewServerIntegrationTest to construct one.
type ServerIntegrationTest struct {
	userServiceClient pb.UserServiceClient
	authServiceClient pb.AuthServiceClient
	emailSender       *testingx.CaptureEmailSender
	userStore         *pguser.Store
	rbacStore         *pgrbac.Store
}

// NewServerIntegrationTest starts a gRPC server over an in-memory transport wired to real cores
// and an isolated PostgreSQL database. The database is created from the template and dropped via t.Cleanup.
func NewServerIntegrationTest(t *testing.T) ServerIntegrationTest {
	t.Helper()

	log := testingx.NewLogger(t)

	// context.Background() rather than t.Context(): pgtest registers a Cleanup that drops the database, which runs
	// after the test ends — at which point t.Context() is already canceled.
	pool := pgtest.New(t, context.Background())

	pgUserStore := pguser.NewStore(pool)
	pgAuthStore := pgauth.NewStore(pool)
	pgRBACStore := pgrbac.NewStore(pool)

	emailSender := &testingx.CaptureEmailSender{}

	authCoreCfg := auth.Config{
		JWTKey:             testJWTKey,
		JWTIssuer:          testJWTIssuer,
		JWTAudience:        testJWTAudience,
		MagicLinkFromEmail: "noreply@theapp.test",
		MagicLinkBaseURL:   "http://testhost/auth/verify",
		MagicLinkTTL:       15 * time.Minute,
		MagicLinkRateLimit: 0,
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    720 * time.Hour,
	}

	authCore := auth.NewCore(pgAuthStore, pgUserStore, pgRBACStore, pgdb.NewTransactor(pool), authCoreCfg)
	userCore := user.NewCore(pgUserStore)

	dbosCtx := dbostest.New(t, context.Background(), pool)

	workflowAuthCore := workflowsauth.NewWorkflowCore(authCore, emailSender, authCoreCfg, dbosCtx)
	workflowsauth.RegisterWorkflows(dbosCtx, workflowAuthCore)
	dbostest.Launch(t, dbosCtx)

	conn := newBufconnClientConn(t, ServerConfig{
		Log:              log,
		UserCore:         userCore,
		AuthCore:         authCore,
		WorkflowAuthCore: workflowAuthCore,
		JWTKey:           testJWTKey,
		JWTIssuer:        testJWTIssuer,
		JWTAudience:      testJWTAudience,
	})

	return ServerIntegrationTest{
		userServiceClient: pb.NewUserServiceClient(conn),
		authServiceClient: pb.NewAuthServiceClient(conn),
		emailSender:       emailSender,
		userStore:         pgUserStore,
		rbacStore:         pgRBACStore,
	}
}

// authCtxForTestUser returns ctx with a valid JWT for a fixed test user, and a fixed project ID,
// in the gRPC outgoing metadata. Use it for calls to any protected endpoint in tests.
func authCtxForTestUser(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-project-id", testProjectID)
	return authCtxWithClaims(t, ctx, mdl.AuthClaims{
		UserID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testJWTIssuer,
			Audience:  jwt.ClaimStrings{testJWTAudience},
			Subject:   "00000000-0000-0000-0000-000000000001",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}, testJWTKey)
}

// authCtxWithToken returns ctx with the given JWT as a Bearer token in the gRPC outgoing metadata.
func authCtxWithToken(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

// authCtxWithClaims returns ctx with a Bearer token built from the given claims and key.
func authCtxWithClaims(t *testing.T, ctx context.Context, claims mdl.AuthClaims, jwtKey []byte) context.Context {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtKey)
	if err != nil {
		t.Fatalf("authCtxWithClaims: sign JWT: %v", err)
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

// authCtxWithInvalidToken returns ctx with a syntactically valid but cryptographically
// invalid Bearer token, exercising the token-validation branch of the interceptor.
func authCtxWithInvalidToken(ctx context.Context) context.Context {
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

// newBufconnClientConn starts cfg's gRPC server over an in-memory bufconn listener and returns
// a client connection to it. Both the listener and the connection are closed via t.Cleanup.
func newBufconnClientConn(t *testing.T, cfg ServerConfig) *grpc.ClientConn {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() {
		if err := lis.Close(); err != nil {
			t.Errorf("bufconn close: %v", err)
		}
	})

	srv := NewServer(cfg)
	t.Cleanup(srv.Stop)

	go func() {
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("grpc serve: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("conn close: %v", err)
		}
	})

	return conn
}
