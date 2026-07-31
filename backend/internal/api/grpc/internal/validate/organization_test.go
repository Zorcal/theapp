package validate

import (
	"testing"

	"google.golang.org/grpc/codes"

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
