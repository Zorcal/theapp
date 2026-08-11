package validate

import (
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func TestGetUser(t *testing.T) {
	if err := GetUser(&pb.GetUserRequest{Id: uuid.NewString()}); err != nil {
		t.Errorf("GetUser() error = %v, want nil", err)
	}
}

func TestGetUser_error(t *testing.T) {
	tests := idValidationTests(
		(*pb.GetUserRequest)(nil),
		&pb.GetUserRequest{Id: "bad"},
	)
	runValidationErrorTests(t, "GetUser", GetUser, tests)
}

func TestCreateUser(t *testing.T) {
	err := CreateUser(&pb.CreateUserRequest{User: &pb.User{
		Email: "alice@test.com",
		Name:  "Alice",
	}})
	if err != nil {
		t.Errorf("CreateUser() error = %v, want nil", err)
	}
}

func TestCreateUser_error(t *testing.T) {
	tests := []validationTest[*pb.CreateUserRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user", description: "required"}),
		},
		{
			name: "missing user",
			in:   &pb.CreateUserRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user", description: "required"}),
		},
		{
			name: "missing email",
			in:   &pb.CreateUserRequest{User: &pb.User{Name: "Alice"}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user.email", description: "required"}),
		},
		{
			name: "invalid email",
			in:   &pb.CreateUserRequest{User: &pb.User{Email: "bad", Name: "Alice"}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "user.email",
				description: "must be a valid email address",
			}),
		},
		{
			name: "missing name",
			in:   &pb.CreateUserRequest{User: &pb.User{Email: "alice@test.com"}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user.name", description: "required"}),
		},
	}
	runValidationErrorTests(t, "CreateUser", CreateUser, tests)
}

func TestUpdateUser(t *testing.T) {
	err := UpdateUser(&pb.UpdateUserRequest{
		User:       &pb.User{Id: uuid.NewString(), Name: "Alice", Etag: uuid.NewString()},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	})
	if err != nil {
		t.Errorf("UpdateUser() error = %v, want nil", err)
	}
}

func TestUpdateUser_error(t *testing.T) {
	userID := uuid.NewString()
	etag := uuid.NewString()

	tests := []validationTest[*pb.UpdateUserRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user", description: "required"}),
		},
		{
			name: "missing user",
			in:   &pb.UpdateUserRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user", description: "required"}),
		},
		{
			name: "invalid id",
			in:   &pb.UpdateUserRequest{User: &pb.User{Id: "bad"}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user.id", description: "must be a valid UUID"}),
		},
		{
			name: "missing update mask",
			in:   &pb.UpdateUserRequest{User: &pb.User{Id: userID, Etag: etag}},
			want: wantInvalidArgument("update_mask is required"),
		},
		{
			name: "unknown update field",
			in: &pb.UpdateUserRequest{
				User:       &pb.User{Id: userID, Etag: etag},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"email"}},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "update_mask",
				description: `field "email" is not updatable`,
			}),
		},
		{
			name: "missing updated name",
			in: &pb.UpdateUserRequest{
				User:       &pb.User{Id: userID, Etag: etag},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user.name", description: "required"}),
		},
		{
			name: "missing etag",
			in: &pb.UpdateUserRequest{
				User:       &pb.User{Id: userID, Name: "Alice"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user.etag", description: "must be a valid UUID"}),
		},
	}
	runValidationErrorTests(t, "UpdateUser", UpdateUser, tests)
}

func TestListUsers(t *testing.T) {
	if err := ListUsers(&pb.ListUsersRequest{}); err != nil {
		t.Errorf("ListUsers() error = %v, want nil", err)
	}
}

func TestListUsers_error(t *testing.T) {
	filterToken, err := conv.EncodePageToken(1, "", &pb.UserFilter{Name: "alice"})
	if err != nil {
		t.Fatalf("EncodePageToken() error = %v", err)
	}
	orderToken, err := conv.EncodePageToken(1, "name", &pb.UserFilter{})
	if err != nil {
		t.Fatalf("EncodePageToken() error = %v", err)
	}

	tests := []validationTest[*pb.ListUsersRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "request", description: "required"}),
		},
		{
			name: "invalid page token",
			in:   &pb.ListUsersRequest{PageToken: "bad"},
			want: wantInvalidArgument("invalid page_token"),
		},
		{
			name: "invalid order by",
			in:   &pb.ListUsersRequest{OrderBy: "bad"},
			want: wantInvalidArgument("invalid order_by"),
		},
		{
			name: "page-token order mismatch",
			in:   &pb.ListUsersRequest{PageToken: orderToken},
			want: wantInvalidArgument("page_token order_by mismatch"),
		},
		{
			name: "page-token filter mismatch",
			in:   &pb.ListUsersRequest{PageToken: filterToken},
			want: wantInvalidArgument("page_token filter mismatch"),
		},
	}
	runValidationErrorTests(t, "ListUsers", ListUsers, tests)
}
