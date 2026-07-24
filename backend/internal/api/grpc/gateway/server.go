// Package gateway provides the HTTP/JSON gateway that proxies to the gRPC server.
package gateway

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

//go:embed openapi/auth.swagger.json openapi/role.swagger.json openapi/system_role.swagger.json openapi/user.swagger.json
var openapiFiles embed.FS

// ServerConfig contains the dependencies for the HTTP gateway server.
type ServerConfig struct {
	Log      *slog.Logger
	GRPCAddr string
	// GRPCDialOptions are appended to the default dial options. Used in tests to
	// inject an in-memory dialer (bufconn) without opening a real TCP port.
	GRPCDialOptions []grpc.DialOption
}

// NewServer constructs the HTTP gateway handler and a cleanup function that must be called on shutdown.
func NewServer(cfg ServerConfig) (h http.Handler, teardown func(), retErr error) {
	dialOpts := append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, cfg.GRPCDialOptions...)
	conn, err := grpc.NewClient(cfg.GRPCAddr, dialOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("dial grpc: %w", err)
	}

	teardown = func() {
		conn.Close()
	}

	defer func() {
		if retErr != nil {
			teardown()
		}
	}()

	muxOpts := []runtime.ServeMuxOption{
		runtime.WithRoutingErrorHandler(routingErrorHandler(cfg.Log)),
	}
	mux := runtime.NewServeMux(muxOpts...)

	if err := pb.RegisterUserServiceHandler(context.Background(), mux, conn); err != nil {
		return nil, nil, fmt.Errorf("register user service handler: %w", err)
	}
	if err := pb.RegisterAuthServiceHandler(context.Background(), mux, conn); err != nil {
		return nil, nil, fmt.Errorf("register auth service handler: %w", err)
	}
	if err := pb.RegisterSystemRoleServiceHandler(context.Background(), mux, conn); err != nil {
		return nil, nil, fmt.Errorf("register system role service handler: %w", err)
	}
	if err := pb.RegisterRoleServiceHandler(context.Background(), mux, conn); err != nil {
		return nil, nil, fmt.Errorf("register role service handler: %w", err)
	}

	allSpecs := []swaggerUISpec{
		{Name: "Auth API", URL: "/v1/openapi/auth.json"},
		{Name: "Role API", URL: "/v1/openapi/role.json"},
		{Name: "System Role API", URL: "/v1/openapi/system-role.json"},
		{Name: "User API", URL: "/v1/openapi/user.json"},
	}

	httpMux := http.NewServeMux()
	httpMux.Handle("/", mux)
	httpMux.HandleFunc("/v1/openapi/auth.json", openapiHandler("openapi/auth.swagger.json"))
	httpMux.HandleFunc("/v1/openapi/role.json", openapiHandler("openapi/role.swagger.json"))
	httpMux.HandleFunc("/v1/openapi/system-role.json", openapiHandler("openapi/system_role.swagger.json"))
	httpMux.HandleFunc("/v1/openapi/user.json", openapiHandler("openapi/user.swagger.json"))
	httpMux.HandleFunc("/docs", swaggerUIHandler("theapp API", "Auth API", allSpecs))

	return loggingMiddleware(cfg.Log, httpMux), teardown, nil
}

func openapiHandler(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spec, err := openapiFiles.ReadFile(path)
		if err != nil {
			http.Error(w, "spec not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(spec)
	}
}

func routingErrorHandler(log *slog.Logger) runtime.RoutingErrorHandlerFunc {
	return func(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, httpStatus int) {
		log.WarnContext(
			ctx, "HTTP Gateway routing error",
			"method", r.Method,
			"path", r.URL.Path,
			"status", httpStatus,
		)
		runtime.DefaultRoutingErrorHandler(ctx, mux, marshaler, w, r, httpStatus)
	}
}
