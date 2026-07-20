package mdl

import (
	"testing"

	"github.com/google/uuid"
)

func TestCreateRole_Validate(t *testing.T) {
	in := CreateRole{Name: "viewer", Permissions: []Permission{PermissionRoleRead}}
	if err := in.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestCreateRole_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   CreateRole
	}{
		{
			name: "empty name",
			in:   CreateRole{Name: "", Permissions: []Permission{PermissionRoleRead}},
		},
		{
			name: "no permissions",
			in:   CreateRole{Name: "viewer", Permissions: nil},
		},
		{
			name: "nonexistent permission",
			in:   CreateRole{Name: "viewer", Permissions: []Permission{"not:a-real-permission"}},
		},
		{
			name: "permission that exists but isn't assignable to a custom role",
			in:   CreateRole{Name: "viewer", Permissions: []Permission{PermissionRoleAssignSystem}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err == nil {
				t.Errorf("Validate() error = nil, want error")
			}
		})
	}
}

func TestUpdateRole_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateRole
	}{
		{
			name: "name only",
			in:   UpdateRole{ID: uuid.New(), Fields: RoleUpdateFields{Name: true}, Name: "viewer"},
		},
		{
			name: "permissions only",
			in:   UpdateRole{ID: uuid.New(), Fields: RoleUpdateFields{Permissions: true}, Permissions: []Permission{PermissionRoleRead}},
		},
		{
			name: "name and permissions",
			in: UpdateRole{
				ID:          uuid.New(),
				Fields:      RoleUpdateFields{Name: true, Permissions: true},
				Name:        "viewer",
				Permissions: []Permission{PermissionRoleRead},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestUpdateRole_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateRole
	}{
		{
			name: "zero id",
			in:   UpdateRole{ID: uuid.UUID{}, Fields: RoleUpdateFields{Name: true}, Name: "viewer"},
		},
		{
			name: "no fields set",
			in:   UpdateRole{ID: uuid.New()},
		},
		{
			name: "empty name",
			in:   UpdateRole{ID: uuid.New(), Fields: RoleUpdateFields{Name: true}, Name: ""},
		},
		{
			name: "permissions set but empty",
			in:   UpdateRole{ID: uuid.New(), Fields: RoleUpdateFields{Permissions: true}},
		},
		{
			name: "nonexistent permission",
			in: UpdateRole{
				ID:          uuid.New(),
				Fields:      RoleUpdateFields{Permissions: true},
				Permissions: []Permission{"not:a-real-permission"},
			},
		},
		{
			name: "permission that exists but isn't assignable to a custom role",
			in: UpdateRole{
				ID:          uuid.New(),
				Fields:      RoleUpdateFields{Permissions: true},
				Permissions: []Permission{PermissionRoleAssignSystem},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err == nil {
				t.Errorf("Validate() error = nil, want error")
			}
		})
	}
}

func TestModifyRolePermissions_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   ModifyRolePermissions
	}{
		{
			name: "add only",
			in:   ModifyRolePermissions{ID: uuid.New(), AddPermissions: []Permission{PermissionRoleRead}},
		},
		{
			name: "remove only",
			in:   ModifyRolePermissions{ID: uuid.New(), RemovePermissions: []Permission{PermissionRoleRead}},
		},
		{
			name: "add and remove disjoint permissions",
			in: ModifyRolePermissions{
				ID:                uuid.New(),
				AddPermissions:    []Permission{PermissionRoleRead},
				RemovePermissions: []Permission{PermissionRoleCreate},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestModifyRolePermissions_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   ModifyRolePermissions
	}{
		{
			name: "zero id",
			in:   ModifyRolePermissions{ID: uuid.UUID{}, AddPermissions: []Permission{PermissionRoleRead}},
		},
		{
			name: "neither add nor remove",
			in:   ModifyRolePermissions{ID: uuid.New()},
		},
		{
			name: "nonexistent add permission",
			in:   ModifyRolePermissions{ID: uuid.New(), AddPermissions: []Permission{"not:a-real-permission"}},
		},
		{
			name: "nonexistent remove permission",
			in:   ModifyRolePermissions{ID: uuid.New(), RemovePermissions: []Permission{"not:a-real-permission"}},
		},
		{
			name: "add permission that exists but isn't assignable to a custom role",
			in:   ModifyRolePermissions{ID: uuid.New(), AddPermissions: []Permission{PermissionRoleAssignSystem}},
		},
		{
			name: "same permission in add and remove",
			in: ModifyRolePermissions{
				ID:                uuid.New(),
				AddPermissions:    []Permission{PermissionRoleRead},
				RemovePermissions: []Permission{PermissionRoleRead},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err == nil {
				t.Errorf("Validate() error = nil, want error")
			}
		})
	}
}

func TestRoleScope_Validate(t *testing.T) {
	projectID, orgID := 1, 2

	tests := []struct {
		name string
		in   RoleScope
	}{
		{name: "project only", in: RoleScope{ProjectID: &projectID}},
		{name: "org only", in: RoleScope{OrgID: &orgID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestRoleScope_Validate_error(t *testing.T) {
	projectID, orgID := 1, 2

	tests := []struct {
		name string
		in   RoleScope
	}{
		{name: "neither set", in: RoleScope{}},
		{name: "both set", in: RoleScope{ProjectID: &projectID, OrgID: &orgID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err == nil {
				t.Errorf("Validate() error = nil, want error")
			}
		})
	}
}

func TestAssignRole_Validate(t *testing.T) {
	projectID := 1
	in := AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: RoleScope{ProjectID: &projectID}}
	if err := in.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestAssignRole_Validate_error(t *testing.T) {
	projectID := 1

	tests := []struct {
		name string
		in   AssignRole
	}{
		{
			name: "zero role id",
			in:   AssignRole{RoleID: uuid.UUID{}, UserID: uuid.New(), Scope: RoleScope{ProjectID: &projectID}},
		},
		{
			name: "zero user id",
			in:   AssignRole{RoleID: uuid.New(), UserID: uuid.UUID{}, Scope: RoleScope{ProjectID: &projectID}},
		},
		{
			name: "invalid scope",
			in:   AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: RoleScope{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err == nil {
				t.Errorf("Validate() error = nil, want error")
			}
		})
	}
}

func TestUnassignRole_Validate(t *testing.T) {
	orgID := 1
	in := UnassignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: RoleScope{OrgID: &orgID}}
	if err := in.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestUnassignRole_Validate_error(t *testing.T) {
	orgID := 1

	tests := []struct {
		name string
		in   UnassignRole
	}{
		{
			name: "zero role id",
			in:   UnassignRole{RoleID: uuid.UUID{}, UserID: uuid.New(), Scope: RoleScope{OrgID: &orgID}},
		},
		{
			name: "zero user id",
			in:   UnassignRole{RoleID: uuid.New(), UserID: uuid.UUID{}, Scope: RoleScope{OrgID: &orgID}},
		},
		{
			name: "invalid scope",
			in:   UnassignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: RoleScope{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err == nil {
				t.Errorf("Validate() error = nil, want error")
			}
		})
	}
}
