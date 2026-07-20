package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

// systemRoleService assigns, unassigns, and lists system roles. AssignSystemRole,
// UnassignSystemRole, and ListSystemRoles aren't implemented yet, so those three fall through to
// pb.UnimplementedSystemRoleServiceServer's default codes.Unimplemented response. The service is
// still registered now so its methods have permission-registry entries from the moment they're
// reachable, rather than that being retrofitted once they're implemented.
type systemRoleService struct {
	pb.UnimplementedSystemRoleServiceServer

	systemRoleCore SystemRoleCore
}

//go:generate moq -rm -fmt goimports -out system_role_core_moq_test.go . SystemRoleCore:MockedSystemRoleCore

// SystemRoleCore defines the system-role-related core operations the gRPC layer requires.
type SystemRoleCore interface {
	// StaticRolePermissions returns a page of the permissions granted to the static role named
	// roleName, and the total count across all pages.
	// Returns [mdl.ErrNotFound] if no static role named roleName exists.
	StaticRolePermissions(ctx context.Context, roleName string, pageSize, pageOffset int) ([]mdl.Permission, int, error)
	// SystemRoleAssignments returns a page of the names of every static role userID holds at
	// system scope, and the total count across all pages.
	// Returns [mdl.ErrNotFound] if no user with that ID exists.
	SystemRoleAssignments(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]string, int, error)
}

func (s *systemRoleService) ListSystemRolePermissions(ctx context.Context, req *pb.ListSystemRolePermissionsRequest) (*pb.ListSystemRolePermissionsResponse, error) {
	if req.GetRoleName() == "" {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "role_name", Description: "required"},
		})
	}

	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50 // sensible default/cap
	}

	offset, err := decodeRolePageToken(req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid page_token"), err)
	}

	page, totalCount, err := s.systemRoleCore.StaticRolePermissions(ctx, req.GetRoleName(), pageSize, offset)
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "system role %q not found", req.GetRoleName())
		default:
			return nil, fmt.Errorf("list system role permissions: %w", err)
		}
	}

	var nextPageToken string
	nextOffset := offset + pageSize
	if nextOffset < totalCount {
		nextPageToken = encodeRolePageToken(nextOffset)
	}

	return &pb.ListSystemRolePermissionsResponse{
		Permissions:   conv.PermissionsToPB(page),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}

func (s *systemRoleService) ListSystemRoleAssignments(ctx context.Context, req *pb.ListSystemRoleAssignmentsRequest) (*pb.ListSystemRoleAssignmentsResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "user_id", Description: "must be a valid UUID"},
		})
	}

	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50 // sensible default/cap
	}

	offset, err := decodeRolePageToken(req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid page_token"), err)
	}

	page, totalCount, err := s.systemRoleCore.SystemRoleAssignments(ctx, userID, pageSize, offset)
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "user %q not found", req.GetUserId())
		default:
			return nil, fmt.Errorf("list system role assignments: %w", err)
		}
	}

	var nextPageToken string
	nextOffset := offset + pageSize
	if nextOffset < totalCount {
		nextPageToken = encodeRolePageToken(nextOffset)
	}

	return &pb.ListSystemRoleAssignmentsResponse{
		RoleNames:     page,
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}
