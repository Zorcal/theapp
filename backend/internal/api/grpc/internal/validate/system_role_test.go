package validate

import (
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func TestListSystemRoles(t *testing.T) {
	if err := ListSystemRoles(&pb.ListSystemRolesRequest{}); err != nil {
		t.Errorf("ListSystemRoles() error = %v, want nil", err)
	}
}

func TestListSystemRoles_error(t *testing.T) {
	tests := []validationTest[*pb.ListSystemRolesRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "request", description: "required"}),
		},
		{
			name: "invalid page token",
			in:   &pb.ListSystemRolesRequest{PageToken: "bad"},
			want: wantInvalidArgument("invalid page_token"),
		},
	}
	runValidationErrorTests(t, "ListSystemRoles", ListSystemRoles, tests)
}

func TestAssignSystemRole(t *testing.T) {
	err := AssignSystemRole(&pb.AssignSystemRoleRequest{
		UserId:   uuid.NewString(),
		RoleName: "admin",
	})
	if err != nil {
		t.Errorf("AssignSystemRole() error = %v, want nil", err)
	}
}

func TestAssignSystemRole_error(t *testing.T) {
	tests := []validationTest[*pb.AssignSystemRoleRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(),
				violation{field: "role_name", description: "required"},
				violation{field: "user_id", description: "required"},
			),
		},
		{
			name: "missing role name",
			in:   &pb.AssignSystemRoleRequest{UserId: uuid.NewString()},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "role_name", description: "required"}),
		},
		{
			name: "invalid user id",
			in:   &pb.AssignSystemRoleRequest{UserId: "bad", RoleName: "admin"},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "user_id",
				description: "must be a valid UUID",
			}),
		},
	}
	runValidationErrorTests(t, "AssignSystemRole", AssignSystemRole, tests)
}

func TestUnassignSystemRole(t *testing.T) {
	err := UnassignSystemRole(&pb.UnassignSystemRoleRequest{
		UserId:   uuid.NewString(),
		RoleName: "admin",
	})
	if err != nil {
		t.Errorf("UnassignSystemRole() error = %v, want nil", err)
	}
}

func TestUnassignSystemRole_error(t *testing.T) {
	tests := []validationTest[*pb.UnassignSystemRoleRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(),
				violation{field: "role_name", description: "required"},
				violation{field: "user_id", description: "required"},
			),
		},
		{
			name: "missing role name",
			in:   &pb.UnassignSystemRoleRequest{UserId: uuid.NewString()},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "role_name", description: "required"}),
		},
		{
			name: "invalid user id",
			in:   &pb.UnassignSystemRoleRequest{UserId: "bad", RoleName: "admin"},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "user_id",
				description: "must be a valid UUID",
			}),
		},
	}
	runValidationErrorTests(t, "UnassignSystemRole", UnassignSystemRole, tests)
}

func TestListSystemRoleAssignments(t *testing.T) {
	err := ListSystemRoleAssignments(&pb.ListSystemRoleAssignmentsRequest{UserId: uuid.NewString()})
	if err != nil {
		t.Errorf("ListSystemRoleAssignments() error = %v, want nil", err)
	}
}

func TestListSystemRoleAssignments_error(t *testing.T) {
	tests := []validationTest[*pb.ListSystemRoleAssignmentsRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user_id", description: "required"}),
		},
		{
			name: "invalid user id",
			in:   &pb.ListSystemRoleAssignmentsRequest{UserId: "bad"},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "user_id", description: "must be a valid UUID"}),
		},
		{
			name: "invalid page token",
			in: &pb.ListSystemRoleAssignmentsRequest{
				UserId:    uuid.NewString(),
				PageToken: "bad",
			},
			want: wantInvalidArgument("invalid page_token"),
		},
	}
	runValidationErrorTests(t, "ListSystemRoleAssignments", ListSystemRoleAssignments, tests)
}
