package validate

import (
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
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

func ListOrganizationUsers(req *pb.ListOrganizationUsersRequest) error {
	if req == nil {
		return requiredRequest("request")
	}
	if req.GetFilter().GetProjectId() < 0 {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "filter.project_id", Description: "must not be negative",
		})
	}

	pageToken, err := conv.DecodePageToken[*pb.OrganizationUserFilter](req.GetPageToken())
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid page_token")
	}
	if req.GetPageToken() != "" && !proto.Equal(pageToken.Filter, req.GetFilter()) {
		return status.Error(codes.InvalidArgument, "page_token filter mismatch")
	}

	return nil
}
