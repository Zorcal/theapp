// Package grpc provides the gRPC server for the application.
package grpc

import (
	"log/slog"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

// ServerConfig contains all the mandatory systems required by the GRPC server.
type ServerConfig struct {
	Log      *slog.Logger
	UserCore UserCore
	AuthCore AuthCore
	// JWTKey is the HMAC secret used to validate access tokens.
	JWTKey []byte
	// Reflection registers the gRPC reflection service. Enable for local
	// development so ad-hoc clients (grpcurl, Evans, ...) can discover the
	// schema; keep it off elsewhere so the schema isn't exposed publicly.
	Reflection bool
}

// NewServer constructs the GRPC server.
func NewServer(cfg ServerConfig) *grpc.Server {
	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(2*1024*1024), // 2mb
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithFilter(notGRPCInfrastructure))),
		grpc.ChainUnaryInterceptor(
			loggingUnaryInterceptor(cfg.Log),
			authUnaryInterceptor(cfg.JWTKey),
			errorUnaryInterceptor(cfg.Log),
			recoveryUnaryInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			loggingStreamInterceptor(cfg.Log),
			authStreamInterceptor(cfg.JWTKey),
			errorStreamInterceptor(cfg.Log),
			recoveryStreamInterceptor(),
		),
	)

	pb.RegisterUserServiceServer(srv, &userService{
		log:      cfg.Log,
		userCore: cfg.UserCore,
	})

	pb.RegisterAuthServiceServer(srv, &authService{
		log:      cfg.Log,
		authCore: cfg.AuthCore,
	})

	if cfg.Reflection {
		reflection.Register(srv)
	}

	return srv
}

// notGRPCInfrastructure returns false for gRPC's own infrastructure services
// (reflection, health) so they aren't traced. Without this they'd flood
// tracing backends like Tempo with reflection's bidi stream metadata,
// drowning out spans that describe actual application behavior.
func notGRPCInfrastructure(info *stats.RPCTagInfo) bool {
	return !strings.HasPrefix(info.FullMethodName, "/grpc.reflection.")
}
