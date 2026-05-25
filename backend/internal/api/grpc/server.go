package grpc

import (
	"log/slog"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// ServerConfig contains all the mandatory systems required by the GRPC server.
type ServerConfig struct {
	Log *slog.Logger
}

// NewServer constructs the GRPC server.
func NewServer(cfg ServerConfig) *grpc.Server {
	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(2*1024*1024), // 2mb
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
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
		log: cfg.Log,
	})

	return srv
}
