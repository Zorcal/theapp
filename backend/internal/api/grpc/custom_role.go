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
	// CreateCustomRole creates a custom role in the caller's organization.
	// Returns [mdl.ErrValidation] if the input is invalid.
	// Returns [mdl.ErrAlreadyExists] if the organization already has a role with that name.
	CreateCustomRole(ctx context.Context, cr mdl.CreateCustomRole) (mdl.CustomRole, error)
	// UpdateCustomRole updates a custom role in the caller's organization.
	// Returns [mdl.ErrNotFound] if the role does not exist or is owned by another organization.
	// Returns [mdl.ErrValidation] if the input is invalid.
	// Returns [mdl.ErrAlreadyExists] if the organization already has a role with that name.
	UpdateCustomRole(ctx context.Context, ur mdl.UpdateCustomRole) (mdl.CustomRole, error)
	// ModifyCustomRolePermissions atomically changes permissions on a custom role.
	// Returns [mdl.ErrNotFound] if the role does not exist or is owned by another organization.
	// Returns [mdl.ErrValidation] if the input is invalid.
	ModifyCustomRolePermissions(ctx context.Context, mrp mdl.ModifyCustomRolePermissions) (mdl.CustomRole, error)
	// DeleteCustomRole deletes a custom role in the caller's organization.
	// Returns [mdl.ErrNotFound] if the role does not exist or is owned by another organization.
	DeleteCustomRole(ctx context.Context, customRoleID uuid.UUID) error
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
