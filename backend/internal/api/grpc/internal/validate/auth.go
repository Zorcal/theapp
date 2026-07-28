package validate

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

func RequestMagicLink(req *pb.RequestMagicLinkRequest) error {
	if req == nil {
		return requiredRequest("email")
	}
	if req.GetEmail() == "" {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "email", Description: "required",
		})
	}
	if !mdl.IsValidEmail(req.GetEmail()) {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "email", Description: "invalid format",
		})
	}
	return nil
}

func VerifyMagicLink(req *pb.VerifyMagicLinkRequest) error {
	if req == nil {
		return requiredRequest("token")
	}
	if req.GetToken() == "" {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "token", Description: "required",
		})
	}
	return nil
}

func RefreshAccessToken(req *pb.RefreshAccessTokenRequest) error {
	if req == nil {
		return requiredRequest("refresh_token")
	}
	if req.GetRefreshToken() == "" {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "refresh_token", Description: "required",
		})
	}
	return nil
}

func RevokeRefreshToken(req *pb.RevokeRefreshTokenRequest) error {
	if req == nil {
		return requiredRequest("refresh_token")
	}
	if req.GetRefreshToken() == "" {
		return invalidArgument(&errdetails.BadRequest_FieldViolation{
			Field: "refresh_token", Description: "required",
		})
	}
	return nil
}

func RevokeAllSessions(req *emptypb.Empty) error {
	if req == nil {
		return requiredRequest("request")
	}
	return nil
}

func GetAuthContext(req *pb.GetAuthContextRequest) error {
	if req == nil {
		return requiredRequest("request")
	}
	return nil
}
