package grpc

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"

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

type roleService struct {
	pb.UnimplementedRoleServiceServer

	log      *slog.Logger
	roleCore RoleCore
}

//go:generate moq -rm -fmt goimports -out role_core_moq_test.go . RoleCore:MockedRoleCore

// RoleCore defines the role-related core operations the gRPC layer requires.
type RoleCore interface {
	// OrgRoles returns a page of orgID's own custom roles and the total count across all pages.
	OrgRoles(ctx context.Context, orgID, pageSize, pageOffset int) ([]mdl.RoleCustom, int, error)
	// CreateRole creates a new custom role owned by orgID and returns it.
	// Returns [mdl.ErrValidation] if cr is invalid.
	CreateRole(ctx context.Context, orgID int, cr mdl.CreateRole) (mdl.RoleCustom, error)
	// UpdateRole applies ur.Fields to the custom role identified by ur.ID, owned by orgID, and
	// returns the updated role.
	// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists.
	// Returns [mdl.ErrValidation] if ur is invalid.
	UpdateRole(ctx context.Context, orgID int, ur mdl.UpdateRole) (mdl.RoleCustom, error)
	// ModifyRolePermissions adds and/or removes permissions from the custom role identified by
	// m.ID, owned by orgID, and returns the updated role.
	// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists.
	// Returns [mdl.ErrValidation] if m is invalid.
	ModifyRolePermissions(ctx context.Context, orgID int, m mdl.ModifyRolePermissions) (mdl.RoleCustom, error)
	// DeleteRole deletes the custom role identified by id, owned by orgID.
	// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists.
	DeleteRole(ctx context.Context, orgID int, id uuid.UUID) error
	// AssignRole assigns in.RoleID, owned by orgID, to in.UserID at in.Scope.
	// Returns [mdl.ErrValidation] if in is invalid.
	// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists; if no user with
	// that ID exists; or if in.Scope's project/org doesn't belong to orgID.
	// Returns [mdl.ErrRoleScopeConflict] if in.Scope is project-scoped and in.UserID already holds
	// in.RoleID at the project's org scope.
	// Returns [mdl.ErrNotOrgMember] if in.Scope is org-scoped and in.UserID isn't a member of that
	// org.
	AssignRole(ctx context.Context, orgID int, in mdl.AssignRole) error
	// UnassignRole unassigns in.RoleID, owned by orgID, from in.UserID at in.Scope.
	// Returns [mdl.ErrValidation] if in is invalid.
	// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists; if no user with
	// that ID exists; or if in.Scope's project/org doesn't belong to orgID.
	UnassignRole(ctx context.Context, orgID int, in mdl.UnassignRole) error
	// ListRoleAssignments returns a page of every role userID holds within orgID and the total
	// count across all pages.
	// Returns [mdl.ErrNotFound] if no user with that ID exists.
	ListRoleAssignments(ctx context.Context, orgID int, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.RoleAssignment, int, error)
	// RolePermissions returns a page of the permissions granted to the custom role identified by
	// roleID, owned by orgID, and the total count across all pages.
	// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists.
	RolePermissions(ctx context.Context, orgID int, roleID uuid.UUID, pageSize, pageOffset int) ([]mdl.Permission, int, error)
}

func (s *roleService) CreateRole(ctx context.Context, req *pb.CreateRoleRequest) (*pb.Role, error) {
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if req.GetRole() == nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "role", Description: "required"},
		})
	}

	role, err := s.roleCore.CreateRole(ctx, orgID, conv.CreateRoleFromPB(req.GetRole()))
	if err != nil {
		if errors.Is(err, mdl.ErrValidation) {
			return nil, status.Error(codes.InvalidArgument, "invalid request")
		}
		return nil, fmt.Errorf("create role: %w", err)
	}

	return conv.RoleToPB(role), nil
}

func (s *roleService) UpdateRole(ctx context.Context, req *pb.UpdateRoleRequest) (*pb.Role, error) {
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if req.GetRole() == nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "role", Description: "required"},
		})
	}

	id, err := uuid.Parse(req.GetRole().GetId())
	if err != nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "role.id", Description: "must be a valid UUID"},
		})
	}

	if err := validateUpdateRoleRequest(req); err != nil {
		return nil, fmt.Errorf("validate update role request: %w", err)
	}

	role, err := s.roleCore.UpdateRole(ctx, orgID, conv.UpdateRoleFromPB(req, id))
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "role %q not found", req.GetRole().GetId())
		case errors.Is(err, mdl.ErrValidation):
			return nil, status.Error(codes.InvalidArgument, "invalid request")
		default:
			return nil, fmt.Errorf("update role: %w", err)
		}
	}

	return conv.RoleToPB(role), nil
}

func (s *roleService) ModifyRolePermissions(ctx context.Context, req *pb.ModifyRolePermissionsRequest) (*pb.Role, error) {
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "id", Description: "must be a valid UUID"},
		})
	}

	role, err := s.roleCore.ModifyRolePermissions(ctx, orgID, conv.ModifyRolePermissionsFromPB(req, id))
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "role %q not found", req.GetId())
		case errors.Is(err, mdl.ErrValidation):
			return nil, status.Error(codes.InvalidArgument, "invalid request")
		default:
			return nil, fmt.Errorf("modify role permissions: %w", err)
		}
	}

	return conv.RoleToPB(role), nil
}

func (s *roleService) DeleteRole(ctx context.Context, req *pb.DeleteRoleRequest) (*emptypb.Empty, error) {
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "id", Description: "must be a valid UUID"},
		})
	}

	if err := s.roleCore.DeleteRole(ctx, orgID, id); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "role %q not found", req.GetId())
		default:
			return nil, fmt.Errorf("delete role: %w", err)
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *roleService) AssignRole(ctx context.Context, req *pb.AssignRoleRequest) (*emptypb.Empty, error) {
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	roleID, userID, err := parseRoleAndUserID(req.GetRoleId(), req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err := s.roleCore.AssignRole(ctx, orgID, conv.AssignRoleFromPB(req, roleID, userID)); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "role or user not found")
		case errors.Is(err, mdl.ErrRoleScopeConflict):
			return nil, status.Error(codes.FailedPrecondition, "role already assigned at org scope")
		case errors.Is(err, mdl.ErrNotOrgMember):
			return nil, status.Error(codes.FailedPrecondition, "user is not a member of the organization")
		case errors.Is(err, mdl.ErrValidation):
			return nil, status.Error(codes.InvalidArgument, "invalid request")
		default:
			return nil, fmt.Errorf("assign role: %w", err)
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *roleService) UnassignRole(ctx context.Context, req *pb.UnassignRoleRequest) (*emptypb.Empty, error) {
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	roleID, userID, err := parseRoleAndUserID(req.GetRoleId(), req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err := s.roleCore.UnassignRole(ctx, orgID, conv.UnassignRoleFromPB(req, roleID, userID)); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "role or user not found")
		case errors.Is(err, mdl.ErrValidation):
			return nil, status.Error(codes.InvalidArgument, "invalid request")
		default:
			return nil, fmt.Errorf("unassign role: %w", err)
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *roleService) ListRoleAssignments(ctx context.Context, req *pb.ListRoleAssignmentsRequest) (*pb.ListRoleAssignmentsResponse, error) {
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

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

	page, totalCount, err := s.roleCore.ListRoleAssignments(ctx, orgID, userID, pageSize, offset)
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "user %q not found", req.GetUserId())
		default:
			return nil, fmt.Errorf("list role assignments: %w", err)
		}
	}

	var nextPageToken string
	nextOffset := offset + pageSize
	if nextOffset < totalCount {
		nextPageToken = encodeRolePageToken(nextOffset)
	}

	return &pb.ListRoleAssignmentsResponse{
		Assignments:   conv.RoleAssignmentsToPB(page),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}

func (s *roleService) ListRolePermissions(ctx context.Context, req *pb.ListRolePermissionsRequest) (*pb.ListRolePermissionsResponse, error) {
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	roleID, err := uuid.Parse(req.GetRoleId())
	if err != nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "role_id", Description: "must be a valid UUID"},
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

	// A roleID that identifies a static role, rather than a custom one, surfaces as mdl.ErrNotFound
	// here, not a distinct "that's a static role" error: static roles live in their own table,
	// entirely separate from the custom roles this RPC operates on, so RolePermissions simply
	// can't find one by ID.
	page, totalCount, err := s.roleCore.RolePermissions(ctx, orgID, roleID, pageSize, offset)
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "role %q not found", req.GetRoleId())
		default:
			return nil, fmt.Errorf("list role permissions: %w", err)
		}
	}

	var nextPageToken string
	nextOffset := offset + pageSize
	if nextOffset < totalCount {
		nextPageToken = encodeRolePageToken(nextOffset)
	}

	return &pb.ListRolePermissionsResponse{
		Permissions:   conv.PermissionsToPB(page),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}

func (s *roleService) ListRoles(ctx context.Context, req *pb.ListRolesRequest) (*pb.ListRolesResponse, error) {
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50 // sensible default/cap
	}

	offset, err := decodeRolePageToken(req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid page_token"), err)
	}

	page, totalCount, err := s.roleCore.OrgRoles(ctx, orgID, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}

	var nextPageToken string
	nextOffset := offset + pageSize
	if nextOffset < totalCount {
		nextPageToken = encodeRolePageToken(nextOffset)
	}

	return &pb.ListRolesResponse{
		Roles:         conv.RolesToPB(page),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}

func (s *roleService) ListAssignablePermissions(ctx context.Context, _ *pb.ListAssignablePermissionsRequest) (*pb.ListAssignablePermissionsResponse, error) {
	if _, err := orgIDFromContext(ctx); err != nil {
		return nil, err
	}

	return &pb.ListAssignablePermissionsResponse{
		Permissions: conv.PermissionsToPB(mdl.AssignablePermissions),
	}, nil
}

// orgIDFromContext extracts the organization ID resolved for the caller's current project from
// ctx. Every RoleService RPC requires x-project-id metadata, so OrgID is always set once
// authUnaryInterceptor has run; a missing session or OrgID indicates the interceptor chain
// wasn't applied to this method, not a caller error.
func orgIDFromContext(ctx context.Context) (int, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return 0, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if sess.OrgID == nil {
		return 0, status.Error(codes.Internal, "missing project scope")
	}
	return *sess.OrgID, nil
}

// parseRoleAndUserID parses roleID and userID, returning an InvalidArgument status naming
// whichever field first fails to parse as a UUID.
func parseRoleAndUserID(roleID, userID string) (uuid.UUID, uuid.UUID, error) {
	parsedRoleID, err := uuid.Parse(roleID)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "role_id", Description: "must be a valid UUID"},
		})
	}
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "user_id", Description: "must be a valid UUID"},
		})
	}
	return parsedRoleID, parsedUserID, nil
}

func validateUpdateRoleRequest(req *pb.UpdateRoleRequest) error {
	maskPaths := req.GetUpdateMask().GetPaths()

	if len(maskPaths) == 0 {
		return status.Error(codes.InvalidArgument, "update_mask is required")
	}

	var violations []*errdetails.BadRequest_FieldViolation

	updatableFields := []string{"name", "permissions"}
	for _, path := range maskPaths {
		if !slices.Contains(updatableFields, path) {
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field:       "update_mask",
				Description: fmt.Sprintf("field %q is not updatable", path),
			})
		}
	}

	if slices.Contains(maskPaths, "name") && req.GetRole().GetName() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       "role.name",
			Description: "required",
		})
	}

	if slices.Contains(maskPaths, "permissions") && len(req.GetRole().GetPermissions()) == 0 {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       "role.permissions",
			Description: "required",
		})
	}

	if len(violations) > 0 {
		return invalidArgumentStatus(violations)
	}
	return nil
}

// encodeRolePageToken/decodeRolePageToken implement ListRoles' opaque pagination cursor as a
// base64-encoded offset. ListRolesRequest has no order_by or filter to pin alongside it (unlike
// conv.PageToken), since role listing has neither.
func encodeRolePageToken(offset int) string {
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func decodeRolePageToken(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	b, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("decode base64: %w", err)
	}
	offset, err := strconv.Atoi(string(b))
	if err != nil {
		return 0, fmt.Errorf("parse offset: %w", err)
	}
	return offset, nil
}
