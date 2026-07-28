package validate

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func TestRequestMagicLink(t *testing.T) {
	if err := RequestMagicLink(&pb.RequestMagicLinkRequest{Email: "alice@test.com"}); err != nil {
		t.Errorf("RequestMagicLink() error = %v, want nil", err)
	}
}

func TestRequestMagicLink_error(t *testing.T) {
	tests := []validationTest[*pb.RequestMagicLinkRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "email",
				description: "required",
			}),
		},
		{
			name: "missing email",
			in:   &pb.RequestMagicLinkRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "email",
				description: "required",
			}),
		},
		{
			name: "invalid email",
			in:   &pb.RequestMagicLinkRequest{Email: "bad"},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "email",
				description: "invalid format",
			}),
		},
	}
	runValidationErrorTests(t, "RequestMagicLink", RequestMagicLink, tests)
}

func TestVerifyMagicLink(t *testing.T) {
	if err := VerifyMagicLink(&pb.VerifyMagicLinkRequest{Token: "token"}); err != nil {
		t.Errorf("VerifyMagicLink() error = %v, want nil", err)
	}
}

func TestVerifyMagicLink_error(t *testing.T) {
	tests := []validationTest[*pb.VerifyMagicLinkRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "token",
				description: "required",
			}),
		},
		{
			name: "missing token",
			in:   &pb.VerifyMagicLinkRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "token",
				description: "required",
			}),
		},
	}
	runValidationErrorTests(t, "VerifyMagicLink", VerifyMagicLink, tests)
}

func TestRefreshAccessToken(t *testing.T) {
	if err := RefreshAccessToken(&pb.RefreshAccessTokenRequest{RefreshToken: "token"}); err != nil {
		t.Errorf("RefreshAccessToken() error = %v, want nil", err)
	}
}

func TestRefreshAccessToken_error(t *testing.T) {
	tests := []validationTest[*pb.RefreshAccessTokenRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "refresh_token",
				description: "required",
			}),
		},
		{
			name: "missing refresh token",
			in:   &pb.RefreshAccessTokenRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "refresh_token",
				description: "required",
			}),
		},
	}
	runValidationErrorTests(t, "RefreshAccessToken", RefreshAccessToken, tests)
}

func TestRevokeRefreshToken(t *testing.T) {
	if err := RevokeRefreshToken(&pb.RevokeRefreshTokenRequest{RefreshToken: "token"}); err != nil {
		t.Errorf("RevokeRefreshToken() error = %v, want nil", err)
	}
}

func TestRevokeRefreshToken_error(t *testing.T) {
	tests := []validationTest[*pb.RevokeRefreshTokenRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "refresh_token",
				description: "required",
			}),
		},
		{
			name: "missing refresh token",
			in:   &pb.RevokeRefreshTokenRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "refresh_token",
				description: "required",
			}),
		},
	}
	runValidationErrorTests(t, "RevokeRefreshToken", RevokeRefreshToken, tests)
}

func TestRevokeAllSessions(t *testing.T) {
	if err := RevokeAllSessions(&emptypb.Empty{}); err != nil {
		t.Errorf("RevokeAllSessions() error = %v, want nil", err)
	}
}

func TestRevokeAllSessions_error(t *testing.T) {
	tests := []validationTest[*emptypb.Empty]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "request",
				description: "required",
			}),
		},
	}
	runValidationErrorTests(t, "RevokeAllSessions", RevokeAllSessions, tests)
}

func TestGetAuthContext(t *testing.T) {
	if err := GetAuthContext(&pb.GetAuthContextRequest{}); err != nil {
		t.Errorf("GetAuthContext() error = %v, want nil", err)
	}
}

func TestGetAuthContext_error(t *testing.T) {
	tests := []validationTest[*pb.GetAuthContextRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "request",
				description: "required",
			}),
		},
	}
	runValidationErrorTests(t, "GetAuthContext", GetAuthContext, tests)
}
