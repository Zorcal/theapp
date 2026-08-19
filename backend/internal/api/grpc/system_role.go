package grpc

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/validate"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

type systemRoleService struct {
	pb.UnimplementedSystemRoleServiceServer

	systemRoleCore SystemRoleCore
}

//go:generate go tool moq -rm -fmt goimports -out system_role_core_moq_test.go . SystemRoleCore:MockedSystemRoleCore

type SystemRoleCore interface {
	// SystemRoles returns a page of system roles and the total count.
	SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]mdl.SystemRole, int, error)
	// UserSystemRoles returns a page of system roles assigned to userID.
	// Returns [mdl.ErrNotFound] if no user with that ID exists.
	UserSystemRoles(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.SystemRole, int, error)
	// AssignSystemRole grants userID the system role named roleName.
	// Returns [mdl.ErrNotFound] if the actor, user, or system role does not exist.
	// Returns [mdl.ErrPermissionDenied] if the actor may not assign the role.
	// Returns [mdl.ErrAlreadyExists] if the user already has the role.
	AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error
	// UnassignSystemRole revokes the system role named roleName from userID.
	// Returns [mdl.ErrNotFound] if the actor, user, system role, or assignment does not exist.
	// Returns [mdl.ErrPermissionDenied] if the actor may not unassign the role.
	// Returns [mdl.ErrLastFullyPrivilegedSystemAdmin] if the change would leave no fully privileged
	// system administrator.
	UnassignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error
}

func (s *systemRoleService) ListSystemRoles(ctx context.Context, req *pb.ListSystemRolesRequest) (*pb.ListSystemRolesResponse, error) {
	if err := validate.ListSystemRoles(req); err != nil {
		return nil, fmt.Errorf("validate list system roles request: %w", err)
	}

	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50
	}

	pageToken, err := conv.DecodePageToken[*emptypb.Empty](req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid page_token"), err)
	}

	roles, totalCount, err := s.systemRoleCore.SystemRoles(ctx, pageSize, pageToken.Offset)
	if err != nil {
		return nil, fmt.Errorf("list system roles: %w", err)
	}

	var nextPageToken string
	nextPageOffset := pageToken.Offset + pageSize
	if nextPageOffset < totalCount {
		nextPageToken, err = conv.EncodePageToken(nextPageOffset, "", &emptypb.Empty{})
		if err != nil {
			return nil, fmt.Errorf("encode next_page_token: %w", err)
		}
	}

	return &pb.ListSystemRolesResponse{
		Roles:         conv.SystemRolesToPB(roles),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}

func (s *systemRoleService) AssignSystemRole(ctx context.Context, req *pb.AssignSystemRoleRequest) (*pb.AssignSystemRoleResponse, error) {
	if err := validate.AssignSystemRole(req); err != nil {
		return nil, fmt.Errorf("validate assign system role request: %w", err)
	}

	userID := uuid.MustParse(req.GetUserId())

	if err := s.systemRoleCore.AssignSystemRole(ctx, userID, req.GetRoleName()); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "user or system role not found")
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, codes.PermissionDenied.String())
		case errors.Is(err, mdl.ErrAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, "user already has system role")
		default:
			return nil, fmt.Errorf("assign system role: %w", err)
		}
	}

	return &pb.AssignSystemRoleResponse{}, nil
}

func (s *systemRoleService) UnassignSystemRole(ctx context.Context, req *pb.UnassignSystemRoleRequest) (*pb.UnassignSystemRoleResponse, error) {
	if err := validate.UnassignSystemRole(req); err != nil {
		return nil, fmt.Errorf("validate unassign system role request: %w", err)
	}

	userID := uuid.MustParse(req.GetUserId())

	if err := s.systemRoleCore.UnassignSystemRole(ctx, userID, req.GetRoleName()); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "system role assignment not found")
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, codes.PermissionDenied.String())
		case errors.Is(err, mdl.ErrLastFullyPrivilegedSystemAdmin):
			return nil, errorStatus(codes.FailedPrecondition, pb.ErrorCode_ERROR_CODE_LAST_FULLY_PRIVILEGED_SYSTEM_ADMIN,
				"cannot remove the last fully privileged system administrator")
		default:
			return nil, fmt.Errorf("unassign system role: %w", err)
		}
	}

	return &pb.UnassignSystemRoleResponse{}, nil
}

func (s *systemRoleService) ListSystemRoleAssignments(ctx context.Context, req *pb.ListSystemRoleAssignmentsRequest) (*pb.ListSystemRoleAssignmentsResponse, error) {
	if err := validate.ListSystemRoleAssignments(req); err != nil {
		return nil, fmt.Errorf("validate list system role assignments request: %w", err)
	}

	userID := uuid.MustParse(req.GetUserId())

	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50
	}

	pageToken, err := conv.DecodePageToken[*emptypb.Empty](req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid page_token"), err)
	}

	roles, totalCount, err := s.systemRoleCore.UserSystemRoles(ctx, userID, pageSize, pageToken.Offset)
	if err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "user %q not found", req.GetUserId())
		}
		return nil, fmt.Errorf("list system role assignments: %w", err)
	}

	var nextPageToken string
	nextPageOffset := pageToken.Offset + pageSize
	if nextPageOffset < totalCount {
		nextPageToken, err = conv.EncodePageToken(nextPageOffset, "", &emptypb.Empty{})
		if err != nil {
			return nil, fmt.Errorf("encode next_page_token: %w", err)
		}
	}

	return &pb.ListSystemRoleAssignmentsResponse{
		Roles:         conv.SystemRolesToPB(roles),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}
