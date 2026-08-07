package validate

import (
	"fmt"
	"slices"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

func GetUser(req *pb.GetUserRequest) error {
	if req == nil {
		return requiredRequest("id")
	}
	if err := validUUID(req.GetId(), "id"); err != nil {
		return err
	}
	return nil
}

func DeleteUser(req *pb.DeleteUserRequest) error {
	if req == nil {
		return requiredRequest("id")
	}
	if err := validUUID(req.GetId(), "id"); err != nil {
		return err
	}
	return nil
}

func CreateUser(req *pb.CreateUserRequest) error {
	if req == nil {
		return requiredRequest("user")
	}
	if req.GetUser() == nil {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "user", Description: "required",
		})
	}

	var violations []*errdetails.BadRequest_FieldViolation

	if req.GetUser().GetEmail() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "user.email", Description: "required",
		})
	} else if !mdl.IsValidEmail(req.GetUser().GetEmail()) {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "user.email", Description: "must be a valid email address",
		})
	}

	if req.GetUser().GetName() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "user.name", Description: "required",
		})
	}

	if len(violations) > 0 {
		return invalidArgument(violations...)
	}

	return nil
}

func UpdateUser(req *pb.UpdateUserRequest) error {
	if req == nil {
		return requiredRequest("user")
	}
	if req.GetUser() == nil {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "user", Description: "required",
		})
	}
	if err := validUUID(req.GetUser().GetId(), "user.id"); err != nil {
		return err
	}

	maskPaths := req.GetUpdateMask().GetPaths()
	if len(maskPaths) == 0 {
		return status.Error(codes.InvalidArgument, "update_mask is required")
	}

	var violations []*errdetails.BadRequest_FieldViolation

	for _, path := range maskPaths {
		if path != "name" {
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field:       "update_mask",
				Description: fmt.Sprintf("field %q is not updatable", path),
			})
		}
	}

	if slices.Contains(maskPaths, "name") && req.GetUser().GetName() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field: "user.name", Description: "required",
		})
	}

	if len(violations) > 0 {
		return invalidArgument(violations...)
	}

	return nil
}

func ListUsers(req *pb.ListUsersRequest) error {
	if req == nil {
		return requiredRequest("request")
	}

	pageToken, err := conv.DecodePageToken[*pb.UserFilter](req.GetPageToken())
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid page_token")
	}

	if _, err := conv.UserOrderBysFromPB(req.GetOrderBy()); err != nil {
		return status.Error(codes.InvalidArgument, "invalid order_by")
	}

	if req.GetPageToken() != "" {
		if pageToken.OrderBy != req.GetOrderBy() {
			return status.Error(codes.InvalidArgument, "page_token order_by mismatch")
		}
		if !proto.Equal(pageToken.Filter, req.GetFilter()) {
			return status.Error(codes.InvalidArgument, "page_token filter mismatch")
		}
	}

	return nil
}
