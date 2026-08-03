package validate

import (
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

func CreateOrganization(req *pb.CreateOrganizationRequest) error {
	if req == nil || req.GetOrganization() == nil {
		return requiredRequest("organization")
	}

	var violations []*errdetails.BadRequest_FieldViolation

	name := req.GetOrganization().GetName()
	trimmedName := strings.TrimSpace(name)
	switch {
	case trimmedName == "":
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "organization.name", Description: "required",
		})
	case trimmedName != name:
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "organization.name", Description: "must not have leading or trailing whitespace",
		})
	}

	projectName := req.GetProjectName()
	trimmedProjectName := strings.TrimSpace(projectName)
	switch {
	case trimmedProjectName == "":
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "project_name", Description: "required",
		})
	case trimmedProjectName != projectName:
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "project_name", Description: "must not have leading or trailing whitespace",
		})
	}

	if len(violations) > 0 {
		return invalidArgument(violations...)
	}

	return nil
}

func CreateOrganizationUser(req *pb.CreateOrganizationUserRequest) error {
	if req == nil {
		return requiredRequest("email")
	}
	if req.GetEmail() == "" {
		return requiredRequest("email")
	}
	if !mdl.IsValidEmail(req.GetEmail()) {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "email", Description: "must be a valid email address",
		})
	}
	return nil
}
