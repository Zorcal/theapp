package grpc

import (
	"log/slog"
	"strings"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"
)

// ServerConfig contains all the mandatory systems required by the GRPC server.
type ServerConfig struct {
	Log      *slog.Logger
	UserCore UserCore
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
			errorUnaryInterceptor(cfg.Log),
			recoveryUnaryInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			loggingStreamInterceptor(cfg.Log),
			errorStreamInterceptor(cfg.Log),
			recoveryStreamInterceptor(),
		),
	)

	pb.RegisterUserServiceServer(srv, &userService{
		log:      cfg.Log,
		userCore: cfg.UserCore,
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
