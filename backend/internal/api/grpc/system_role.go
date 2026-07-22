package grpc

import (
	"context"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type systemRoleService struct {
	pb.UnimplementedSystemRoleServiceServer
}

func (systemRoleService) ListSystemRoles(context.Context, *pb.ListSystemRolesRequest) (*pb.ListSystemRolesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method ListSystemRoles not implemented")
}

func (systemRoleService) ListSystemRolePermissions(context.Context, *pb.ListSystemRolePermissionsRequest) (*pb.ListSystemRolePermissionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method ListSystemRolePermissions not implemented")
}

func (systemRoleService) AssignSystemRole(context.Context, *pb.AssignSystemRoleRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "method AssignSystemRole not implemented")
}

func (systemRoleService) UnassignSystemRole(context.Context, *pb.UnassignSystemRoleRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "method UnassignSystemRole not implemented")
}

func (systemRoleService) ListSystemRoleAssignments(context.Context, *pb.ListSystemRoleAssignmentsRequest) (*pb.ListSystemRoleAssignmentsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method ListSystemRoleAssignments not implemented")
}
