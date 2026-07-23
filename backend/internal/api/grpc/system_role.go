package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

type systemRoleService struct {
	pb.UnimplementedSystemRoleServiceServer

	systemRoleCore             SystemRoleCore
	systemRoleOrganizationCore SystemRoleOrganizationCore
}

//go:generate moq -rm -fmt goimports -out system_role_core_moq_test.go . SystemRoleCore:MockedSystemRoleCore SystemRoleOrganizationCore:MockedSystemRoleOrganizationCore

type SystemRoleCore interface {
	SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]mdl.SystemRole, int, error)
	// UserSystemRoles returns a page of system roles assigned to userID.
	// Returns [mdl.ErrNotFound] if no user with that ID exists.
	UserSystemRoles(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.SystemRole, int, error)
	// AssignSystemRole grants userID the system role named roleName.
	// Returns [mdl.ErrNotFound] if the user or system role does not exist.
	// Returns [mdl.ErrPermissionDenied] if the actor may not assign the role.
	// Returns [mdl.ErrAlreadyExists] if the user already has the role.
	AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error
	// UnassignSystemRole revokes the system role named roleName from userID.
	// Returns [mdl.ErrNotFound] if the user, system role, or assignment does not exist.
	// Returns [mdl.ErrPermissionDenied] if the actor may not unassign the role.
	// Returns [mdl.ErrLastRoleManager] if the change would remove the last role manager.
	UnassignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error
}

type SystemRoleOrganizationCore interface {
	// OrganizationByName returns the organization with the given name.
	// Returns [mdl.ErrNotFound] if no organization with that name exists.
	OrganizationByName(ctx context.Context, name string) (mdl.Organization, error)
}

func (s *systemRoleService) ListSystemRoles(ctx context.Context, req *pb.ListSystemRolesRequest) (*pb.ListSystemRolesResponse, error) {
	if err := s.authorizeProject(ctx); err != nil {
		return nil, fmt.Errorf("authorize project: %w", err)
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
	if err := s.authorizeProject(ctx); err != nil {
		return nil, fmt.Errorf("authorize project: %w", err)
	}

	var violations []*errdetails.BadRequest_FieldViolation
	if req.GetRoleName() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "role_name", Description: "required",
		})
	}

	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "user_id", Description: "must be a valid UUID",
		})
	}

	if len(violations) > 0 {
		return nil, invalidArgumentStatus(violations)
	}

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
	if err := s.authorizeProject(ctx); err != nil {
		return nil, fmt.Errorf("authorize project: %w", err)
	}

	var violations []*errdetails.BadRequest_FieldViolation
	if req.GetRoleName() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "role_name", Description: "required",
		})
	}

	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "user_id", Description: "must be a valid UUID",
		})
	}

	if len(violations) > 0 {
		return nil, invalidArgumentStatus(violations)
	}

	if err := s.systemRoleCore.UnassignSystemRole(ctx, userID, req.GetRoleName()); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "system role assignment not found")
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, codes.PermissionDenied.String())
		case errors.Is(err, mdl.ErrLastRoleManager):
			return nil, status.Error(codes.FailedPrecondition, "cannot remove the last system role manager")
		default:
			return nil, fmt.Errorf("unassign system role: %w", err)
		}
	}

	return &pb.UnassignSystemRoleResponse{}, nil
}

func (s *systemRoleService) ListSystemRoleAssignments(ctx context.Context, req *pb.ListSystemRoleAssignmentsRequest) (*pb.ListSystemRoleAssignmentsResponse, error) {
	if err := s.authorizeProject(ctx); err != nil {
		return nil, fmt.Errorf("authorize project: %w", err)
	}

	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "user_id", Description: "must be a valid UUID"},
		})
	}

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

func (s *systemRoleService) authorizeProject(ctx context.Context) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok || sess.ProjectID == nil || sess.OrgID == nil {
		return errors.New("auth session project or organization missing")
	}

	org, err := s.systemRoleOrganizationCore.OrganizationByName(ctx, mdl.SystemOrgName)
	if err != nil {
		return fmt.Errorf("fetch system organization: %w", err)
	}

	if sess.MustOrgID() != org.ID || sess.MustProjectID() != org.ControlProjectID {
		return status.Error(codes.PermissionDenied, codes.PermissionDenied.String())
	}

	return nil
}
