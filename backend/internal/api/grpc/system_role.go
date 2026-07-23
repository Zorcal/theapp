package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

type systemRoleService struct {
	pb.UnimplementedSystemRoleServiceServer
}

func (systemRoleService) ListSystemRoles(context.Context, *pb.ListSystemRolesRequest) (*pb.ListSystemRolesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method ListSystemRoles not implemented")
}

func (systemRoleService) AssignSystemRole(context.Context, *pb.AssignSystemRoleRequest) (*pb.AssignSystemRoleResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method AssignSystemRole not implemented")
}

func (systemRoleService) UnassignSystemRole(context.Context, *pb.UnassignSystemRoleRequest) (*pb.UnassignSystemRoleResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method UnassignSystemRole not implemented")
}

func (systemRoleService) ListSystemRoleAssignments(context.Context, *pb.ListSystemRoleAssignmentsRequest) (*pb.ListSystemRoleAssignmentsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method ListSystemRoleAssignments not implemented")
}
