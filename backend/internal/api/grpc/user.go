package grpc

import (
	"context"
	"log/slog"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userService struct {
	pb.UnimplementedUserServiceServer
	log *slog.Logger
}

func (s *userService) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method ListUsers not implemented")
}
