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
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/validate"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

type customRoleService struct {
	pb.UnimplementedRoleServiceServer

	customRoleCore CustomRoleCore
}

//go:generate moq -rm -fmt goimports -out custom_role_core_moq_test.go . CustomRoleCore:MockedCustomRoleCore

type CustomRoleCore interface {
	// CustomRoles returns a page of custom roles owned by the caller's organization.
	CustomRoles(ctx context.Context, pageSize, pageOffset int) ([]mdl.CustomRole, int, error)
	// CustomRoleByID returns a custom role owned by the caller's organization.
	// Returns [mdl.ErrNotFound] if the role does not exist or is owned by another organization.
	CustomRoleByID(ctx context.Context, customRoleID uuid.UUID) (mdl.CustomRole, error)
	// UserProjectCustomRoles returns a page of custom roles assigned directly to userID in the caller's project.
	// Returns [mdl.ErrNotFound] if the user, project, or organization membership does not exist.
	UserProjectCustomRoles(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.CustomRole, int, error)
	// UserOrgCustomRoles returns a page of custom roles assigned to userID across the caller's organization.
	// Returns [mdl.ErrNotFound] if the user or organization membership does not exist.
	UserOrgCustomRoles(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.CustomRole, int, error)
	// CreateCustomRole creates a custom role in the caller's organization.
	// Returns [mdl.ErrValidation] if the input is invalid.
	// Returns [mdl.ErrAlreadyExists] if the organization already has a role with that name.
	// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission added to the role.
	CreateCustomRole(ctx context.Context, cr mdl.CreateCustomRole) (mdl.CustomRole, error)
	// UpdateCustomRole updates a custom role in the caller's organization.
	// Returns [mdl.ErrNotFound] if the role does not exist or is owned by another organization.
	// Returns [mdl.ErrValidation] if the input is invalid.
	// Returns [mdl.ErrAlreadyExists] if the organization already has a role with that name.
	// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission added to or removed from the role.
	UpdateCustomRole(ctx context.Context, ur mdl.UpdateCustomRole) (mdl.CustomRole, error)
	// ModifyCustomRolePermissions atomically changes permissions on a custom role.
	// Returns [mdl.ErrNotFound] if the role does not exist or is owned by another organization.
	// Returns [mdl.ErrValidation] if the input is invalid.
	// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission added to or removed from the role.
	ModifyCustomRolePermissions(ctx context.Context, mrp mdl.ModifyCustomRolePermissions) (mdl.CustomRole, error)
	// DeleteCustomRole deletes a custom role in the caller's organization.
	// Returns [mdl.ErrNotFound] if the role does not exist or is owned by another organization.
	DeleteCustomRole(ctx context.Context, customRoleID uuid.UUID) error
	// AssignCustomRoleToProject assigns a custom role to a user in the caller's project.
	// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission in the role.
	AssignCustomRoleToProject(ctx context.Context, targetUserID, roleID uuid.UUID) error
	// UnassignCustomRoleFromProject unassigns a custom role from a user in the caller's project.
	// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission in the role.
	UnassignCustomRoleFromProject(ctx context.Context, targetUserID, roleID uuid.UUID) error
	// AssignCustomRoleToOrg assigns a custom role to a user across the caller's organization.
	// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission in the role.
	AssignCustomRoleToOrg(ctx context.Context, targetUserID, roleID uuid.UUID) error
	// UnassignCustomRoleFromOrg unassigns a custom role from a user across the caller's organization.
	// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission in the role.
	UnassignCustomRoleFromOrg(ctx context.Context, targetUserID, roleID uuid.UUID) error
}

func (s *customRoleService) CreateRole(ctx context.Context, req *pb.CreateRoleRequest) (*pb.Role, error) {
	if err := validate.CreateRole(req); err != nil {
		return nil, fmt.Errorf("validate create role request: %w", err)
	}

	role, err := s.customRoleCore.CreateCustomRole(ctx, conv.CreateCustomRoleFromPB(req.GetRole()))
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrValidation):
			return nil, status.Error(codes.InvalidArgument, "invalid role")
		case errors.Is(err, mdl.ErrAlreadyExists):
			return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "role.name", Description: "a role with this name already exists"},
			})
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "organization or permission not found")
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, "caller cannot add role permissions")
		default:
			return nil, fmt.Errorf("create role: %w", err)
		}
	}

	return conv.CustomRoleToPB(role), nil
}

func (s *customRoleService) GetRole(ctx context.Context, req *pb.GetRoleRequest) (*pb.Role, error) {
	if err := validate.GetRole(req); err != nil {
		return nil, fmt.Errorf("validate get role request: %w", err)
	}

	roleID := uuid.MustParse(req.GetId())

	role, err := s.customRoleCore.CustomRoleByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "role %q not found", req.GetId())
		}
		return nil, fmt.Errorf("get role: %w", err)
	}

	return conv.CustomRoleToPB(role), nil
}

func (s *customRoleService) ListRoles(ctx context.Context, req *pb.ListRolesRequest) (*pb.ListRolesResponse, error) {
	if err := validate.ListRoles(req); err != nil {
		return nil, fmt.Errorf("validate list roles request: %w", err)
	}

	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50
	}

	pageToken, err := conv.DecodePageToken[*emptypb.Empty](req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid page_token"), err)
	}

	roles, totalCount, err := s.customRoleCore.CustomRoles(ctx, pageSize, pageToken.Offset)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}

	var nextPageToken string
	nextPageOffset := pageToken.Offset + pageSize
	if nextPageOffset < totalCount {
		nextPageToken, err = conv.EncodePageToken(nextPageOffset, "", &emptypb.Empty{})
		if err != nil {
			return nil, fmt.Errorf("encode next_page_token: %w", err)
		}
	}

	return &pb.ListRolesResponse{
		Roles:         conv.CustomRolesToPB(roles),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}

func (s *customRoleService) ListProjectRoleAssignments(ctx context.Context, req *pb.ListProjectRoleAssignmentsRequest) (*pb.ListProjectRoleAssignmentsResponse, error) {
	if err := validate.ListProjectRoleAssignments(req); err != nil {
		return nil, fmt.Errorf("validate list project role assignments request: %w", err)
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

	roles, totalCount, err := s.customRoleCore.UserProjectCustomRoles(ctx, userID, pageSize, pageToken.Offset)
	if err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user, project, or organization membership not found")
		}
		return nil, fmt.Errorf("list project role assignments: %w", err)
	}

	var nextPageToken string
	nextPageOffset := pageToken.Offset + pageSize
	if nextPageOffset < totalCount {
		nextPageToken, err = conv.EncodePageToken(nextPageOffset, "", &emptypb.Empty{})
		if err != nil {
			return nil, fmt.Errorf("encode next_page_token: %w", err)
		}
	}

	return &pb.ListProjectRoleAssignmentsResponse{
		Roles:         conv.CustomRolesToPB(roles),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}

func (s *customRoleService) ListOrganizationRoleAssignments(ctx context.Context, req *pb.ListOrganizationRoleAssignmentsRequest) (*pb.ListOrganizationRoleAssignmentsResponse, error) {
	if err := validate.ListOrganizationRoleAssignments(req); err != nil {
		return nil, fmt.Errorf("validate list organization role assignments request: %w", err)
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

	roles, totalCount, err := s.customRoleCore.UserOrgCustomRoles(ctx, userID, pageSize, pageToken.Offset)
	if err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user or organization membership not found")
		}
		return nil, fmt.Errorf("list organization role assignments: %w", err)
	}

	var nextPageToken string
	nextPageOffset := pageToken.Offset + pageSize
	if nextPageOffset < totalCount {
		nextPageToken, err = conv.EncodePageToken(nextPageOffset, "", &emptypb.Empty{})
		if err != nil {
			return nil, fmt.Errorf("encode next_page_token: %w", err)
		}
	}

	return &pb.ListOrganizationRoleAssignmentsResponse{
		Roles:         conv.CustomRolesToPB(roles),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}

func (s *customRoleService) UpdateRole(ctx context.Context, req *pb.UpdateRoleRequest) (*pb.Role, error) {
	if err := validate.UpdateRole(req); err != nil {
		return nil, fmt.Errorf("validate update role request: %w", err)
	}

	roleID := uuid.MustParse(req.GetRole().GetId())

	role, err := s.customRoleCore.UpdateCustomRole(ctx, conv.UpdateCustomRoleFromPB(req, roleID))
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "role %q not found", req.GetRole().GetId())
		case errors.Is(err, mdl.ErrValidation):
			return nil, status.Error(codes.InvalidArgument, "invalid role")
		case errors.Is(err, mdl.ErrAlreadyExists):
			return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "role.name", Description: "a role with this name already exists"},
			})
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, "caller cannot change role permissions")
		default:
			return nil, fmt.Errorf("update role: %w", err)
		}
	}

	return conv.CustomRoleToPB(role), nil
}

func (s *customRoleService) ModifyRolePermissions(ctx context.Context, req *pb.ModifyRolePermissionsRequest) (*pb.Role, error) {
	if err := validate.ModifyRolePermissions(req); err != nil {
		return nil, fmt.Errorf("validate modify role permissions request: %w", err)
	}

	roleID := uuid.MustParse(req.GetId())

	role, err := s.customRoleCore.ModifyCustomRolePermissions(ctx, conv.ModifyCustomRolePermissionsFromPB(req, roleID))
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "role %q or permission not found", req.GetId())
		case errors.Is(err, mdl.ErrValidation):
			return nil, status.Error(codes.InvalidArgument, "invalid permission changes")
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, "caller cannot change role permissions")
		default:
			return nil, fmt.Errorf("modify role permissions: %w", err)
		}
	}

	return conv.CustomRoleToPB(role), nil
}

func (s *customRoleService) DeleteRole(ctx context.Context, req *pb.DeleteRoleRequest) (*pb.DeleteRoleResponse, error) {
	if err := validate.DeleteRole(req); err != nil {
		return nil, fmt.Errorf("validate delete role request: %w", err)
	}

	roleID := uuid.MustParse(req.GetId())

	if err := s.customRoleCore.DeleteCustomRole(ctx, roleID); err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "role %q not found", req.GetId())
		}
		return nil, fmt.Errorf("delete role: %w", err)
	}

	return &pb.DeleteRoleResponse{}, nil
}

func (s *customRoleService) AssignRoleToProject(ctx context.Context, req *pb.AssignRoleToProjectRequest) (*pb.AssignRoleToProjectResponse, error) {
	if err := validate.AssignRoleToProject(req); err != nil {
		return nil, fmt.Errorf("validate assign role to project request: %w", err)
	}

	roleID := uuid.MustParse(req.GetRoleId())
	userID := uuid.MustParse(req.GetUserId())

	if err := s.customRoleCore.AssignCustomRoleToProject(ctx, userID, roleID); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "user, role, project, or organization membership not found")
		case errors.Is(err, mdl.ErrAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, "user already has role in project")
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, "caller cannot grant role permissions")
		default:
			return nil, fmt.Errorf("assign role to project: %w", err)
		}
	}

	return &pb.AssignRoleToProjectResponse{}, nil
}

func (s *customRoleService) UnassignRoleFromProject(ctx context.Context, req *pb.UnassignRoleFromProjectRequest) (*pb.UnassignRoleFromProjectResponse, error) {
	if err := validate.UnassignRoleFromProject(req); err != nil {
		return nil, fmt.Errorf("validate unassign role from project request: %w", err)
	}

	roleID := uuid.MustParse(req.GetRoleId())
	userID := uuid.MustParse(req.GetUserId())

	if err := s.customRoleCore.UnassignCustomRoleFromProject(ctx, userID, roleID); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "project role assignment not found")
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, "caller cannot revoke role permissions")
		default:
			return nil, fmt.Errorf("unassign role from project: %w", err)
		}
	}

	return &pb.UnassignRoleFromProjectResponse{}, nil
}

func (s *customRoleService) AssignRoleToOrganization(ctx context.Context, req *pb.AssignRoleToOrganizationRequest) (*pb.AssignRoleToOrganizationResponse, error) {
	if err := validate.AssignRoleToOrganization(req); err != nil {
		return nil, fmt.Errorf("validate assign role to organization request: %w", err)
	}

	roleID := uuid.MustParse(req.GetRoleId())
	userID := uuid.MustParse(req.GetUserId())

	if err := s.customRoleCore.AssignCustomRoleToOrg(ctx, userID, roleID); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "user, role, organization, or organization membership not found")
		case errors.Is(err, mdl.ErrAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, "user already has role in organization")
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, "caller cannot grant role permissions")
		default:
			return nil, fmt.Errorf("assign role to organization: %w", err)
		}
	}

	return &pb.AssignRoleToOrganizationResponse{}, nil
}

func (s *customRoleService) UnassignRoleFromOrganization(ctx context.Context, req *pb.UnassignRoleFromOrganizationRequest) (*pb.UnassignRoleFromOrganizationResponse, error) {
	if err := validate.UnassignRoleFromOrganization(req); err != nil {
		return nil, fmt.Errorf("validate unassign role from organization request: %w", err)
	}

	roleID := uuid.MustParse(req.GetRoleId())
	userID := uuid.MustParse(req.GetUserId())

	if err := s.customRoleCore.UnassignCustomRoleFromOrg(ctx, userID, roleID); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "organization role assignment not found")
		case errors.Is(err, mdl.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, "caller cannot revoke role permissions")
		default:
			return nil, fmt.Errorf("unassign role from organization: %w", err)
		}
	}

	return &pb.UnassignRoleFromOrganizationResponse{}, nil
}
