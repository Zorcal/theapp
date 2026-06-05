package grpc

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func invalidArgumentStatus(violations []*errdetails.BadRequest_FieldViolation) error {
	st, err := status.New(codes.InvalidArgument, codes.InvalidArgument.String()).WithDetails(
		&errdetails.BadRequest{FieldViolations: violations},
	)
	if err != nil {
		return status.Error(codes.InvalidArgument, codes.InvalidArgument.String())
	}
	return st.Err()
}
