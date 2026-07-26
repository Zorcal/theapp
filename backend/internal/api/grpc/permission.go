package grpc

import (
	"context"
	"fmt"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/validate"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

type permissionService struct {
	pb.UnimplementedPermissionServiceServer
}

func (s *permissionService) ListPermissions(
	_ context.Context,
	req *pb.ListPermissionsRequest,
) (*pb.ListPermissionsResponse, error) {
	if err := validate.ListPermissions(req); err != nil {
		return nil, fmt.Errorf("validate list permissions request: %w", err)
	}

	return &pb.ListPermissionsResponse{
		Permissions: conv.PermissionsToPB(mdl.PermissionsAssignableToCustomRoles()),
	}, nil
}
