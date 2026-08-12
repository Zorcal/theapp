package validate

import (
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func TestCreateOrganization(t *testing.T) {
	req := &pb.CreateOrganizationRequest{
		Organization: &pb.Organization{Name: "acme"},
		ProjectName:  "widgets",
	}
	if err := CreateOrganization(req); err != nil {
		t.Errorf("CreateOrganization() error = %v, want nil", err)
	}
}

func TestCreateOrganizationUser(t *testing.T) {
	if err := CreateOrganizationUser(&pb.CreateOrganizationUserRequest{Email: "member@test.com"}); err != nil {
		t.Errorf("CreateOrganizationUser() error = %v, want nil", err)
	}
}

func TestCreateOrganizationUser_error(t *testing.T) {
	tests := []validationTest[*pb.CreateOrganizationUserRequest]{
		{
			name: "missing request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "email", description: "required"}),
		},
		{
			name: "missing email",
			in:   &pb.CreateOrganizationUserRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "email", description: "required"}),
		},
		{
			name: "invalid email",
			in:   &pb.CreateOrganizationUserRequest{Email: "not-an-email"},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "email", description: "must be a valid email address"}),
		},
	}
	runValidationErrorTests(t, "CreateOrganizationUser", CreateOrganizationUser, tests)
}

func TestCreateOrganization_error(t *testing.T) {
	tests := []validationTest[*pb.CreateOrganizationRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "organization", description: "required"}),
		},
		{
			name: "missing organization",
			in:   &pb.CreateOrganizationRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "organization", description: "required"}),
		},
		{
			name: "missing fields",
			in:   &pb.CreateOrganizationRequest{Organization: &pb.Organization{}},
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "organization.name", description: "required"},
				violation{field: "project_name", description: "required"},
			),
		},
		{
			name: "surrounding whitespace",
			in: &pb.CreateOrganizationRequest{
				Organization: &pb.Organization{Name: " acme"},
				ProjectName:  "widgets ",
			},
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "organization.name", description: "must not have leading or trailing whitespace"},
				violation{field: "project_name", description: "must not have leading or trailing whitespace"},
			),
		},
	}
	runValidationErrorTests(t, "CreateOrganization", CreateOrganization, tests)
}

func TestListOrganizationUsers(t *testing.T) {
	pageToken, err := conv.EncodePageToken(2, "", &pb.OrganizationUserFilter{ProjectId: 7})
	if err != nil {
		t.Fatalf("EncodePageToken() error = %v", err)
	}

	tests := []struct {
		name string
		in   *pb.ListOrganizationUsersRequest
	}{
		{
			name: "empty request",
			in:   &pb.ListOrganizationUsersRequest{},
		},
		{
			name: "matching page token filter",
			in: &pb.ListOrganizationUsersRequest{
				PageToken: pageToken,
				Filter:    &pb.OrganizationUserFilter{ProjectId: 7},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ListOrganizationUsers(tt.in); err != nil {
				t.Errorf("ListOrganizationUsers() error = %v, want nil", err)
			}
		})
	}
}

func TestListOrganizationUsers_error(t *testing.T) {
	pageToken, err := conv.EncodePageToken(2, "", &pb.OrganizationUserFilter{ProjectId: 7})
	if err != nil {
		t.Fatalf("EncodePageToken() error = %v", err)
	}

	tests := []validationTest[*pb.ListOrganizationUsersRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "request", description: "required"}),
		},
		{
			name: "negative project id",
			in:   &pb.ListOrganizationUsersRequest{Filter: &pb.OrganizationUserFilter{ProjectId: -1}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "filter.project_id", description: "must not be negative"}),
		},
		{
			name: "invalid page token",
			in:   &pb.ListOrganizationUsersRequest{PageToken: "invalid"},
			want: wantInvalidArgument("invalid page_token"),
		},
		{
			name: "page token filter mismatch",
			in: &pb.ListOrganizationUsersRequest{
				PageToken: pageToken,
				Filter:    &pb.OrganizationUserFilter{ProjectId: 8},
			},
			want: wantInvalidArgument("page_token filter mismatch"),
		},
	}
	runValidationErrorTests(t, "ListOrganizationUsers", ListOrganizationUsers, tests)
}
