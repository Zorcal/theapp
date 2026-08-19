// Package validate validates protobuf requests before gRPC handlers use them.
package validate

import (
	"uuid"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
)

func invalidArgument(violations ...*errdetails.BadRequest_FieldViolation) error {
	st, err := status.New(codes.InvalidArgument, codes.InvalidArgument.String()).WithDetails(
		&errdetails.BadRequest{FieldViolations: violations},
	)
	if err != nil {
		return status.Error(codes.InvalidArgument, codes.InvalidArgument.String())
	}

	return st.Err()
}

func requiredRequest(fields ...string) error {
	violations := make([]*errdetails.BadRequest_FieldViolation, 0, len(fields))
	for _, field := range fields {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       field,
			Description: "required",
		})
	}

	return invalidArgument(violations...)
}

func validUUID(value, field string) error {
	if _, err := uuid.Parse(value); err != nil {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: field, Description: "must be a valid UUID",
		})
	}

	return nil
}

func validEmptyPageToken(value string) error {
	if _, err := conv.DecodePageToken[*emptypb.Empty](value); err != nil {
		return status.Error(codes.InvalidArgument, "invalid page_token")
	}
	return nil
}
