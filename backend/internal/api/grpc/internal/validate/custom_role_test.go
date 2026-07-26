package validate

import (
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func TestCreateRole(t *testing.T) {
	if err := CreateRole(&pb.CreateRoleRequest{Role: &pb.Role{Name: "role manager"}}); err != nil {
		t.Errorf("CreateRole() error = %v, want nil", err)
	}
}

func TestCreateRole_error(t *testing.T) {
	tests := []validationTest[*pb.CreateRoleRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "role", description: "required"}),
		},
		{
			name: "missing role",
			in:   &pb.CreateRoleRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "role", description: "required"}),
		},
		{
			name: "whitespace-only name",
			in:   &pb.CreateRoleRequest{Role: &pb.Role{Name: " \t"}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "role.name", description: "required"}),
		},
		{
			name: "surrounding whitespace",
			in:   &pb.CreateRoleRequest{Role: &pb.Role{Name: " role manager "}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role.name",
				description: "must not have leading or trailing whitespace",
			}),
		},
		{
			name: "system-only permission",
			in: &pb.CreateRoleRequest{Role: &pb.Role{
				Name:        "role manager",
				Permissions: []pb.Permission{pb.Permission_PERMISSION_USER_READ},
			}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role.permissions[0]",
				description: `"PERMISSION_USER_READ" is system-only`,
			}),
		},
		{
			name: "unspecified permission",
			in: &pb.CreateRoleRequest{Role: &pb.Role{
				Name:        "role manager",
				Permissions: []pb.Permission{pb.Permission_PERMISSION_UNSPECIFIED},
			}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role.permissions[0]",
				description: `"PERMISSION_UNSPECIFIED" is not a recognized permission`,
			}),
		},
		{
			name: "unknown permission",
			in: &pb.CreateRoleRequest{Role: &pb.Role{
				Name:        "role manager",
				Permissions: []pb.Permission{pb.Permission(999)},
			}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role.permissions[0]",
				description: `"999" is not a recognized permission`,
			}),
		},
	}
	runValidationErrorTests(t, "CreateRole", CreateRole, tests)
}

func TestGetRole(t *testing.T) {
	if err := GetRole(&pb.GetRoleRequest{Id: uuid.NewString()}); err != nil {
		t.Errorf("GetRole() error = %v, want nil", err)
	}
}

func TestGetRole_error(t *testing.T) {
	tests := idValidationTests(
		(*pb.GetRoleRequest)(nil),
		&pb.GetRoleRequest{Id: "bad"},
	)
	runValidationErrorTests(t, "GetRole", GetRole, tests)
}

func TestListRoles(t *testing.T) {
	if err := ListRoles(&pb.ListRolesRequest{}); err != nil {
		t.Errorf("ListRoles() error = %v, want nil", err)
	}
}

func TestListRoles_error(t *testing.T) {
	tests := []validationTest[*pb.ListRolesRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "request", description: "required"}),
		},
		{
			name: "invalid page token",
			in:   &pb.ListRolesRequest{PageToken: "bad"},
			want: wantInvalidArgument("invalid page_token"),
		},
	}
	runValidationErrorTests(t, "ListRoles", ListRoles, tests)
}

func TestUpdateRole(t *testing.T) {
	err := UpdateRole(&pb.UpdateRoleRequest{
		Role:       &pb.Role{Id: uuid.NewString(), Name: "role manager"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	})
	if err != nil {
		t.Errorf("UpdateRole() error = %v, want nil", err)
	}
}

func TestUpdateRole_error(t *testing.T) {
	roleID := uuid.NewString()

	tests := []validationTest[*pb.UpdateRoleRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "role", description: "required"}),
		},
		{
			name: "missing role",
			in:   &pb.UpdateRoleRequest{},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "role", description: "required"}),
		},
		{
			name: "invalid id",
			in:   &pb.UpdateRoleRequest{Role: &pb.Role{Id: "bad"}},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "role.id", description: "must be a valid UUID"}),
		},
		{
			name: "missing update mask",
			in:   &pb.UpdateRoleRequest{Role: &pb.Role{Id: roleID}},
			want: wantInvalidArgument("update_mask is required"),
		},
		{
			name: "unknown update field",
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: roleID},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"etag"}},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "update_mask",
				description: `field "etag" is not updatable`,
			}),
		},
		{
			name: "whitespace-only name",
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: roleID, Name: " \t"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "role.name", description: "required"}),
		},
		{
			name: "surrounding whitespace",
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: roleID, Name: " role manager"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role.name",
				description: "must not have leading or trailing whitespace",
			}),
		},
		{
			name: "system-only permission",
			in: &pb.UpdateRoleRequest{
				Role: &pb.Role{
					Id:          roleID,
					Permissions: []pb.Permission{pb.Permission_PERMISSION_USER_READ},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"permissions"}},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role.permissions[0]",
				description: `"PERMISSION_USER_READ" is system-only`,
			}),
		},
	}
	runValidationErrorTests(t, "UpdateRole", UpdateRole, tests)
}

func TestModifyRolePermissions(t *testing.T) {
	if err := ModifyRolePermissions(&pb.ModifyRolePermissionsRequest{Id: uuid.NewString()}); err != nil {
		t.Errorf("ModifyRolePermissions() error = %v, want nil", err)
	}
}

func TestModifyRolePermissions_error(t *testing.T) {
	roleID := uuid.NewString()

	tests := []validationTest[*pb.ModifyRolePermissionsRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "id", description: "required"}),
		},
		{
			name: "invalid id",
			in:   &pb.ModifyRolePermissionsRequest{Id: "bad"},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{field: "id", description: "must be a valid UUID"}),
		},
		{
			name: "overlapping permission",
			in: &pb.ModifyRolePermissionsRequest{
				Id:                roleID,
				AddPermissions:    []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ},
				RemovePermissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "add_permissions[0]",
				description: "must not also appear in remove_permissions",
			}),
		},
		{
			name: "system-only permission addition",
			in: &pb.ModifyRolePermissionsRequest{
				Id:             roleID,
				AddPermissions: []pb.Permission{pb.Permission_PERMISSION_USER_READ},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "add_permissions[0]",
				description: `"PERMISSION_USER_READ" is system-only`,
			}),
		},
		{
			name: "system-only permission removal",
			in: &pb.ModifyRolePermissionsRequest{
				Id:                roleID,
				RemovePermissions: []pb.Permission{pb.Permission_PERMISSION_USER_READ},
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "remove_permissions[0]",
				description: `"PERMISSION_USER_READ" is system-only`,
			}),
		},
	}
	runValidationErrorTests(t, "ModifyRolePermissions", ModifyRolePermissions, tests)
}

func TestDeleteRole(t *testing.T) {
	if err := DeleteRole(&pb.DeleteRoleRequest{Id: uuid.NewString()}); err != nil {
		t.Errorf("DeleteRole() error = %v, want nil", err)
	}
}

func TestDeleteRole_error(t *testing.T) {
	tests := idValidationTests(
		(*pb.DeleteRoleRequest)(nil),
		&pb.DeleteRoleRequest{Id: "bad"},
	)
	runValidationErrorTests(t, "DeleteRole", DeleteRole, tests)
}

func TestAssignRoleToProject(t *testing.T) {
	if err := AssignRoleToProject(&pb.AssignRoleToProjectRequest{
		RoleId: uuid.NewString(),
		UserId: uuid.NewString(),
	}); err != nil {
		t.Errorf("AssignRoleToProject() error = %v, want nil", err)
	}
}

func TestAssignRoleToProject_error(t *testing.T) {
	tests := []validationTest[*pb.AssignRoleToProjectRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "role_id", description: "required"},
				violation{field: "user_id", description: "required"},
			),
		},
		{
			name: "invalid role id",
			in: &pb.AssignRoleToProjectRequest{
				RoleId: "bad",
				UserId: uuid.NewString(),
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role_id",
				description: "must be a valid UUID",
			}),
		},
		{
			name: "invalid user id",
			in: &pb.AssignRoleToProjectRequest{
				RoleId: uuid.NewString(),
				UserId: "bad",
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "user_id",
				description: "must be a valid UUID",
			}),
		},
		{
			name: "invalid ids",
			in: &pb.AssignRoleToProjectRequest{
				RoleId: "bad",
				UserId: "also-bad",
			},
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "role_id", description: "must be a valid UUID"},
				violation{field: "user_id", description: "must be a valid UUID"},
			),
		},
	}
	runValidationErrorTests(t, "AssignRoleToProject", AssignRoleToProject, tests)
}

func TestUnassignRoleFromProject(t *testing.T) {
	if err := UnassignRoleFromProject(&pb.UnassignRoleFromProjectRequest{
		RoleId: uuid.NewString(),
		UserId: uuid.NewString(),
	}); err != nil {
		t.Errorf("UnassignRoleFromProject() error = %v, want nil", err)
	}
}

func TestUnassignRoleFromProject_error(t *testing.T) {
	tests := []validationTest[*pb.UnassignRoleFromProjectRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "role_id", description: "required"},
				violation{field: "user_id", description: "required"},
			),
		},
		{
			name: "invalid role id",
			in: &pb.UnassignRoleFromProjectRequest{
				RoleId: "bad",
				UserId: uuid.NewString(),
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role_id",
				description: "must be a valid UUID",
			}),
		},
		{
			name: "invalid user id",
			in: &pb.UnassignRoleFromProjectRequest{
				RoleId: uuid.NewString(),
				UserId: "bad",
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "user_id",
				description: "must be a valid UUID",
			}),
		},
		{
			name: "invalid ids",
			in: &pb.UnassignRoleFromProjectRequest{
				RoleId: "bad",
				UserId: "also-bad",
			},
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "role_id", description: "must be a valid UUID"},
				violation{field: "user_id", description: "must be a valid UUID"},
			),
		},
	}
	runValidationErrorTests(t, "UnassignRoleFromProject", UnassignRoleFromProject, tests)
}

func TestAssignRoleToOrganization(t *testing.T) {
	if err := AssignRoleToOrganization(&pb.AssignRoleToOrganizationRequest{
		RoleId: uuid.NewString(),
		UserId: uuid.NewString(),
	}); err != nil {
		t.Errorf("AssignRoleToOrganization() error = %v, want nil", err)
	}
}

func TestAssignRoleToOrganization_error(t *testing.T) {
	tests := []validationTest[*pb.AssignRoleToOrganizationRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "role_id", description: "required"},
				violation{field: "user_id", description: "required"},
			),
		},
		{
			name: "invalid role id",
			in: &pb.AssignRoleToOrganizationRequest{
				RoleId: "bad",
				UserId: uuid.NewString(),
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role_id",
				description: "must be a valid UUID",
			}),
		},
		{
			name: "invalid user id",
			in: &pb.AssignRoleToOrganizationRequest{
				RoleId: uuid.NewString(),
				UserId: "bad",
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "user_id",
				description: "must be a valid UUID",
			}),
		},
		{
			name: "invalid ids",
			in: &pb.AssignRoleToOrganizationRequest{
				RoleId: "bad",
				UserId: "also-bad",
			},
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "role_id", description: "must be a valid UUID"},
				violation{field: "user_id", description: "must be a valid UUID"},
			),
		},
	}
	runValidationErrorTests(t, "AssignRoleToOrganization", AssignRoleToOrganization, tests)
}

func TestUnassignRoleFromOrganization(t *testing.T) {
	if err := UnassignRoleFromOrganization(&pb.UnassignRoleFromOrganizationRequest{
		RoleId: uuid.NewString(),
		UserId: uuid.NewString(),
	}); err != nil {
		t.Errorf("UnassignRoleFromOrganization() error = %v, want nil", err)
	}
}

func TestUnassignRoleFromOrganization_error(t *testing.T) {
	tests := []validationTest[*pb.UnassignRoleFromOrganizationRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "role_id", description: "required"},
				violation{field: "user_id", description: "required"},
			),
		},
		{
			name: "invalid role id",
			in: &pb.UnassignRoleFromOrganizationRequest{
				RoleId: "bad",
				UserId: uuid.NewString(),
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "role_id",
				description: "must be a valid UUID",
			}),
		},
		{
			name: "invalid user id",
			in: &pb.UnassignRoleFromOrganizationRequest{
				RoleId: uuid.NewString(),
				UserId: "bad",
			},
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "user_id",
				description: "must be a valid UUID",
			}),
		},
		{
			name: "invalid ids",
			in: &pb.UnassignRoleFromOrganizationRequest{
				RoleId: "bad",
				UserId: "also-bad",
			},
			want: wantInvalidArgument(
				codes.InvalidArgument.String(),
				violation{field: "role_id", description: "must be a valid UUID"},
				violation{field: "user_id", description: "must be a valid UUID"},
			),
		},
	}
	runValidationErrorTests(t, "UnassignRoleFromOrganization", UnassignRoleFromOrganization, tests)
}
