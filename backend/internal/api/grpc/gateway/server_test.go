package gateway

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	grpcapi "github.com/zorcal/theapp/backend/internal/api/grpc"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestNewServer(t *testing.T) {
	ts := newTestGateway(t)
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		wantStatus  int
		wantHeaders map[string]string
	}{
		{
			name:       "auth openapi spec",
			method:     http.MethodGet,
			path:       "/v1/openapi/auth.json",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name:       "user openapi spec",
			method:     http.MethodGet,
			path:       "/v1/openapi/user.json",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name:       "unknown route",
			method:     http.MethodGet,
			path:       "/unknown",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "protected route without token reaches grpc and is rejected",
			method:     http.MethodGet,
			path:       "/v1/users/00000000-0000-0000-0000-000000000001",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:        "public route proxies to grpc",
			method:      http.MethodPost,
			path:        "/v1/auth/magic-link",
			body:        `{"email":"test@example.com"}`,
			contentType: "application/json",
			wantStatus:  http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			req, err := http.NewRequestWithContext(t.Context(), tt.method, ts.URL+tt.path, body)
			if err != nil {
				t.Fatalf("NewRequestWithContext: %v", err)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			for k, want := range tt.wantHeaders {
				if got := resp.Header.Get(k); got != want {
					t.Errorf("header %q = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func newTestGateway(t *testing.T) *httptest.Server {
	t.Helper()

	lis := newTestGRPCBufconn(t)
	handler, teardown, err := NewServer(ServerConfig{
		Log:      testingx.NewLogger(t),
		GRPCAddr: "passthrough://bufnet",
		GRPCDialOptions: []grpc.DialOption{
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(teardown)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return ts
}

func newTestGRPCBufconn(t *testing.T) *bufconn.Listener {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() {
		if err := lis.Close(); err != nil {
			t.Errorf("bufconn close: %v", err)
		}
	})

	srv := grpcapi.NewServer(grpcapi.ServerConfig{
		Log:              testingx.NewLogger(t),
		UserCore:         &noopUserCore{},
		AuthCore:         &noopAuthCore{},
		WorkflowAuthCore: &noopWorkflowAuthCore{},
		JWTKey:           []byte("test-key"),
		JWTIssuer:        "test",
		JWTAudience:      "test",
	})
	t.Cleanup(srv.Stop)

	go func() {
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("grpc serve: %v", err)
		}
	}()

	return lis
}

type noopUserCore struct{}

func (noopUserCore) UserByID(_ context.Context, _ uuid.UUID) (mdl.User, error) {
	return mdl.User{}, nil
}

func (noopUserCore) Users(_ context.Context, _ mdl.UserFilter, _ []order.By[mdl.UserOrderByField], _, _ int) ([]mdl.User, int, error) {
	return nil, 0, nil
}

func (noopUserCore) CreateUser(_ context.Context, _ mdl.CreateUser) (mdl.User, error) {
	return mdl.User{}, nil
}

func (noopUserCore) UpdateUser(_ context.Context, _ mdl.UpdateUser) (mdl.User, error) {
	return mdl.User{}, nil
}

type noopAuthCore struct{}

func (noopAuthCore) VerifyMagicLink(_ context.Context, _ mdl.VerifyMagicLink) (mdl.AuthTokenPair, error) {
	return mdl.AuthTokenPair{}, nil
}

func (noopAuthCore) RefreshAccessToken(_ context.Context, _ mdl.RefreshToken) (mdl.AuthTokenPair, error) {
	return mdl.AuthTokenPair{}, nil
}
func (noopAuthCore) RevokeRefreshToken(_ context.Context, _ mdl.RefreshToken) error  { return nil }
func (noopAuthCore) RevokeAllUserRefreshTokens(_ context.Context, _ uuid.UUID) error { return nil }

func (noopAuthCore) AuthUser(_ context.Context, userID uuid.UUID) (mdl.AuthUser, error) {
	return mdl.AuthUser{UserID: userID, Permissions: mdl.AllPermissions}, nil
}

type noopWorkflowAuthCore struct{}

func (noopWorkflowAuthCore) RequestMagicLink(_ context.Context, _ string) error { return nil }
