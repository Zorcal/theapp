package grpc

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func errorStatus(code codes.Code, errCode pb.ErrorCode, msg string) error {
	st, err := status.New(code, msg).WithDetails(&pb.ErrorDetail{Code: errCode})
	if err != nil {
		return status.Error(code, msg)
	}
	return st.Err()
}

func invalidArgumentStatus(violations []*errdetails.BadRequest_FieldViolation) error {
	st, err := status.New(codes.InvalidArgument, codes.InvalidArgument.String()).WithDetails(
		&errdetails.BadRequest{FieldViolations: violations},
	)
	if err != nil {
		return status.Error(codes.InvalidArgument, codes.InvalidArgument.String())
	}
	return st.Err()
}
