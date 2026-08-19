package gateway

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
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

func TestOpenAPISpecs_documentErrors(t *testing.T) {
	specPaths := []string{
		"openapi/customer/theapp.swagger.json",
		"openapi/internal/theapp.swagger.json",
	}

	for _, specPath := range specPaths {
		specJSON, err := openapiFiles.ReadFile(specPath)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", specPath, err)
		}

		var spec struct {
			Paths map[string]map[string]struct {
				OperationID string `json:"operationId"`
				Responses   map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"responses"`
			} `json:"paths"`
		}
		if err := json.Unmarshal(specJSON, &spec); err != nil {
			t.Fatalf("Unmarshal(%q) error = %v", specPath, err)
		}

		for _, path := range spec.Paths {
			for _, operation := range path {
				documentedErrors := 0
				for code, response := range operation.Responses {
					if code == "200" || code == "default" {
						continue
					}
					documentedErrors++
					if got, want := response.Schema.Ref, "#/definitions/rpcStatus"; got != want {
						t.Errorf("%s response %s schema = %q, want %q", operation.OperationID, code, got, want)
					}
				}
				if documentedErrors == 0 {
					t.Errorf("%s documented error count = 0, want greater than 0", operation.OperationID)
				}
			}
		}
	}
}

func TestOpenAPISpecs_serviceVisibility(t *testing.T) {
	specJSON, err := openapiFiles.ReadFile("openapi/customer/theapp.swagger.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var spec struct {
		Paths map[string]jsontext.Value `json:"paths"`
	}
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	for path := range spec.Paths {
		if strings.HasPrefix(path, "/v1/users") || strings.HasPrefix(path, "/v1/system-role") {
			t.Errorf("customer OpenAPI paths contains internal path %q", path)
		}
	}

	internalSpecJSON, err := openapiFiles.ReadFile("openapi/internal/theapp.swagger.json")
	if err != nil {
		t.Fatalf("ReadFile() internal spec error = %v", err)
	}
	if err := json.Unmarshal(internalSpecJSON, &spec); err != nil {
		t.Fatalf("Unmarshal() internal spec error = %v", err)
	}

	var (
		hasUserService       bool
		hasSystemRoleService bool
	)
	for path := range spec.Paths {
		hasUserService = hasUserService || strings.HasPrefix(path, "/v1/users")
		hasSystemRoleService = hasSystemRoleService || strings.HasPrefix(path, "/v1/system-role")
	}
	if !hasUserService {
		t.Error("internal OpenAPI paths missing UserService")
	}
	if !hasSystemRoleService {
		t.Error("internal OpenAPI paths missing SystemRoleService")
	}
}

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
		username    string
		password    string
	}{
		{
			name:       "customer openapi bundle",
			method:     http.MethodGet,
			path:       "/v1/openapi.json",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name:       "old per-service customer spec omitted",
			method:     http.MethodGet,
			path:       "/v1/openapi/user.json",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "internal spec requires basic auth",
			method:     http.MethodGet,
			path:       "/v1/openapi/internal.json",
			wantStatus: http.StatusUnauthorized,
			wantHeaders: map[string]string{
				"WWW-Authenticate": `Basic realm="internal API docs", charset="UTF-8"`,
			},
		},
		{
			name:       "internal spec rejects invalid basic auth",
			method:     http.MethodGet,
			path:       "/v1/openapi/internal.json",
			wantStatus: http.StatusUnauthorized,
			username:   "test-user",
			password:   "wrong-password",
		},
		{
			name:       "internal spec with basic auth",
			method:     http.MethodGet,
			path:       "/v1/openapi/internal.json",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Type": "application/json",
			},
			username: "test-user",
			password: "test-password",
		},
		{
			name:       "internal API docs with basic auth",
			method:     http.MethodGet,
			path:       "/internal/docs",
			wantStatus: http.StatusOK,
			username:   "test-user",
			password:   "test-password",
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
			if tt.username != "" || tt.password != "" {
				req.SetBasicAuth(tt.username, tt.password)
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

func TestNewServer_error(t *testing.T) {
	tests := []struct {
		name string
		in   ServerConfig
	}{
		{
			name: "internal API docs username missing",
			in:   ServerConfig{InternalAPIDocsPassword: "password"},
		},
		{
			name: "internal API docs password missing",
			in:   ServerConfig{InternalAPIDocsUsername: "username"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := NewServer(tt.in); err == nil {
				t.Errorf("NewServer() error = nil, want error")
			}
		})
	}
}

func newTestGateway(t *testing.T) *httptest.Server {
	t.Helper()

	lis := newTestGRPCBufconn(t)
	handler, teardown, err := NewServer(ServerConfig{
		Log:                     testingx.NewLogger(t),
		GRPCAddr:                "passthrough://bufnet",
		InternalAPIDocsUsername: "test-user",
		InternalAPIDocsPassword: "test-password",
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

func (noopAuthCore) AuthSession(_ context.Context, _ uuid.UUID, _ *int) (mdl.AuthSession, error) {
	return mdl.AuthSession{}, nil
}

func (noopAuthCore) OrganizationAuthSession(_ context.Context, _ uuid.UUID, _ int) (mdl.AuthSession, error) {
	return mdl.AuthSession{}, nil
}

func (noopAuthCore) AuthContext(_ context.Context) (mdl.AuthContext, error) {
	return mdl.AuthContext{}, nil
}

type noopWorkflowAuthCore struct{}

func (noopWorkflowAuthCore) RequestMagicLink(_ context.Context, _ string) error { return nil }
