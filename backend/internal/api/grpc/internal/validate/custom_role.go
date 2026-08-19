package validate

import (
	"fmt"
	"slices"
	"strings"
	"uuid"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

func CreateRole(req *pb.CreateRoleRequest) error {
	if req == nil {
		return requiredRequest("role")
	}
	if req.GetRole() == nil {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "role", Description: "required",
		})
	}

	var violations []*errdetails.BadRequest_FieldViolation

	name := req.GetRole().GetName()
	trimmedName := strings.TrimSpace(name)
	switch {
	case trimmedName == "":
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "role.name", Description: "required",
		})
	case trimmedName != name:
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "role.name", Description: "must not have leading or trailing whitespace",
		})
	}

	violations = append(violations, permissionViolations(req.GetRole().GetPermissions(), "role.permissions")...)

	if len(violations) > 0 {
		return invalidArgument(violations...)
	}

	return nil
}

func GetRole(req *pb.GetRoleRequest) error {
	if req == nil {
		return requiredRequest("id")
	}
	if err := validUUID(req.GetId(), "id"); err != nil {
		return err
	}
	return nil
}

func ListRoles(req *pb.ListRolesRequest) error {
	if req == nil {
		return requiredRequest("request")
	}
	if _, err := conv.DecodePageToken[*emptypb.Empty](req.GetPageToken()); err != nil {
		return status.Error(codes.InvalidArgument, "invalid page_token")
	}
	return nil
}

func ListProjectRoleAssignments(req *pb.ListProjectRoleAssignmentsRequest) error {
	if req == nil {
		return requiredRequest("user_id")
	}
	if err := roleAssignments(req.GetUserId(), req.GetPageToken()); err != nil {
		return err
	}
	return nil
}

func ListOrganizationRoleAssignments(req *pb.ListOrganizationRoleAssignmentsRequest) error {
	if req == nil {
		return requiredRequest("user_id")
	}
	if err := roleAssignments(req.GetUserId(), req.GetPageToken()); err != nil {
		return err
	}
	return nil
}

func UpdateRole(req *pb.UpdateRoleRequest) error {
	if req == nil {
		return requiredRequest("role")
	}
	if req.GetRole() == nil {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "role", Description: "required",
		})
	}
	if err := validUUID(req.GetRole().GetId(), "role.id"); err != nil {
		return err
	}
	if err := validUUID(req.GetRole().GetEtag(), "role.etag"); err != nil {
		return err
	}

	maskPaths := req.GetUpdateMask().GetPaths()
	if len(maskPaths) == 0 {
		return status.Error(codes.InvalidArgument, "update_mask is required")
	}

	updatableFields := []string{"name", "permissions"}

	var violations []*errdetails.BadRequest_FieldViolation

	for _, path := range maskPaths {
		if !slices.Contains(updatableFields, path) {
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field:       "update_mask",
				Description: fmt.Sprintf("field %q is not updatable", path),
			})
		}
	}

	if slices.Contains(maskPaths, "name") {
		name := req.GetRole().GetName()
		trimmedName := strings.TrimSpace(name)
		switch {
		case trimmedName == "":
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field: "role.name", Description: "required",
			})
		case trimmedName != name:
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field: "role.name", Description: "must not have leading or trailing whitespace",
			})
		}
	}

	if slices.Contains(maskPaths, "permissions") {
		violations = append(violations, permissionViolations(req.GetRole().GetPermissions(), "role.permissions")...)
	}

	if len(violations) > 0 {
		return invalidArgument(violations...)
	}

	return nil
}

func ModifyRolePermissions(req *pb.ModifyRolePermissionsRequest) error {
	if req == nil {
		return requiredRequest("id")
	}
	if err := validUUID(req.GetId(), "id"); err != nil {
		return err
	}
	if err := validUUID(req.GetEtag(), "etag"); err != nil {
		return err
	}

	var violations []*errdetails.BadRequest_FieldViolation

	for i, permName := range req.GetAddPermissions() {
		if slices.Contains(req.GetRemovePermissions(), permName) {
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field:       fmt.Sprintf("add_permissions[%d]", i),
				Description: "must not also appear in remove_permissions",
			})
		}
	}

	violations = append(violations, permissionViolations(req.GetAddPermissions(), "add_permissions")...)
	violations = append(violations, permissionViolations(req.GetRemovePermissions(), "remove_permissions")...)

	if len(violations) > 0 {
		return invalidArgument(violations...)
	}

	return nil
}

func DeleteRole(req *pb.DeleteRoleRequest) error {
	if req == nil {
		return requiredRequest("id")
	}
	if err := validUUID(req.GetId(), "id"); err != nil {
		return err
	}
	return nil
}

func AssignRoleToProject(req *pb.AssignRoleToProjectRequest) error {
	if req == nil {
		return requiredRequest("role_id", "user_id")
	}
	if err := roleAssignment(req.GetRoleId(), req.GetUserId()); err != nil {
		return err
	}
	return nil
}

func UnassignRoleFromProject(req *pb.UnassignRoleFromProjectRequest) error {
	if req == nil {
		return requiredRequest("role_id", "user_id")
	}
	if err := roleAssignment(req.GetRoleId(), req.GetUserId()); err != nil {
		return err
	}
	return nil
}

func AssignRoleToOrganization(req *pb.AssignRoleToOrganizationRequest) error {
	if req == nil {
		return requiredRequest("role_id", "user_id")
	}
	if err := roleAssignment(req.GetRoleId(), req.GetUserId()); err != nil {
		return err
	}
	return nil
}

func UnassignRoleFromOrganization(req *pb.UnassignRoleFromOrganizationRequest) error {
	if req == nil {
		return requiredRequest("role_id", "user_id")
	}
	if err := roleAssignment(req.GetRoleId(), req.GetUserId()); err != nil {
		return err
	}
	return nil
}

func roleAssignment(roleID, userID string) error {
	var violations []*errdetails.BadRequest_FieldViolation

	if _, err := uuid.Parse(roleID); err != nil {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "role_id", Description: "must be a valid UUID",
		})
	}
	if _, err := uuid.Parse(userID); err != nil {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "user_id", Description: "must be a valid UUID",
		})
	}

	if len(violations) > 0 {
		return invalidArgument(violations...)
	}

	return nil
}

func roleAssignments(userID, pageToken string) error {
	if err := validUUID(userID, "user_id"); err != nil {
		return err
	}
	if _, err := conv.DecodePageToken[*emptypb.Empty](pageToken); err != nil {
		return status.Error(codes.InvalidArgument, "invalid page_token")
	}
	return nil
}

func permissionViolations(perms []pb.Permission, field string) []*errdetails.BadRequest_FieldViolation {
	systemOnlyPerms := mdl.SystemOnlyPermissions()

	var violations []*errdetails.BadRequest_FieldViolation

	for i, perm := range perms {
		mdlPerm, ok := conv.PermissionFromPB(perm)
		switch {
		case !ok:
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field:       fmt.Sprintf("%s[%d]", field, i),
				Description: fmt.Sprintf("%q is not a recognized permission", perm.String()),
			})
		case slices.Contains(systemOnlyPerms, mdlPerm):
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field:       fmt.Sprintf("%s[%d]", field, i),
				Description: fmt.Sprintf("%q is system-only", perm.String()),
			})
		}
	}

	return violations
}
