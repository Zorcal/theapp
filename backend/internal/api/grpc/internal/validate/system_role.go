package validate

import (
	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func ListSystemRoles(req *pb.ListSystemRolesRequest) error {
	if req == nil {
		return requiredRequest("request")
	}
	if err := validEmptyPageToken(req.GetPageToken()); err != nil {
		return err
	}
	return nil
}

func AssignSystemRole(req *pb.AssignSystemRoleRequest) error {
	if req == nil {
		return requiredRequest("role_name", "user_id")
	}

	var violations []*errdetails.BadRequest_FieldViolation
	if req.GetRoleName() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "role_name", Description: "required",
		})
	}
	if _, err := uuid.Parse(req.GetUserId()); err != nil {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "user_id", Description: "must be a valid UUID",
		})
	}

	if len(violations) > 0 {
		return invalidArgument(violations...)
	}

	return nil
}

func UnassignSystemRole(req *pb.UnassignSystemRoleRequest) error {
	if req == nil {
		return requiredRequest("role_name", "user_id")
	}

	var violations []*errdetails.BadRequest_FieldViolation
	if req.GetRoleName() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "role_name", Description: "required",
		})
	}
	if _, err := uuid.Parse(req.GetUserId()); err != nil {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "user_id", Description: "must be a valid UUID",
		})
	}

	if len(violations) > 0 {
		return invalidArgument(violations...)
	}

	return nil
}

func ListSystemRoleAssignments(req *pb.ListSystemRoleAssignmentsRequest) error {
	if req == nil {
		return requiredRequest("user_id")
	}
	if err := validUUID(req.GetUserId(), "user_id"); err != nil {
		return err
	}
	if err := validEmptyPageToken(req.GetPageToken()); err != nil {
		return err
	}
	return nil
}
