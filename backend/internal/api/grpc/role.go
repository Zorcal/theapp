package grpc

import "github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"

type roleService struct {
	pb.UnimplementedRoleServiceServer
}
