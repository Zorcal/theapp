package mdl

import (
	"errors"
	"testing"
)

func TestCreateCustomRole_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   CreateCustomRole
	}{
		{
			name: "name without permissions",
			in:   CreateCustomRole{Name: "project manager"},
		},
		{
			name: "with permissions",
			in: CreateCustomRole{
				Name:        "project manager",
				Permissions: []Permission{PermissionCustomRoleRead},
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

func TestCreateCustomRole_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   CreateCustomRole
	}{
		{
			name: "empty name",
			in:   CreateCustomRole{Name: ""},
		},
		{
			name: "whitespace-only name",
			in:   CreateCustomRole{Name: "  "},
		},
		{
			name: "leading whitespace",
			in:   CreateCustomRole{Name: " project manager"},
		},
		{
			name: "trailing whitespace",
			in:   CreateCustomRole{Name: "project manager "},
		},
		{
			name: "system-only permission",
			in: CreateCustomRole{
				Name:        "project manager",
				Permissions: []Permission{PermissionUserRead},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestUpdateCustomRole_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateCustomRole
	}{
		{
			name: "name not selected",
			in:   UpdateCustomRole{Fields: CustomRoleUpdateFields{}},
		},
		{
			name: "name selected",
			in: UpdateCustomRole{
				Fields: CustomRoleUpdateFields{Name: true},
				Name:   "project lead",
			},
		},
		{
			name: "permission selected",
			in: UpdateCustomRole{
				Fields:      CustomRoleUpdateFields{Permissions: true},
				Permissions: []Permission{PermissionCustomRoleUpdate},
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

func TestUpdateCustomRole_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateCustomRole
	}{
		{
			name: "empty selected name",
			in: UpdateCustomRole{
				Fields: CustomRoleUpdateFields{Name: true},
			},
		},
		{
			name: "leading whitespace in selected name",
			in: UpdateCustomRole{
				Fields: CustomRoleUpdateFields{Name: true},
				Name:   " project lead",
			},
		},
		{
			name: "system-only permission",
			in: UpdateCustomRole{
				Fields:      CustomRoleUpdateFields{Permissions: true},
				Permissions: []Permission{PermissionSystemRoleRead},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestModifyCustomRolePermissions_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   ModifyCustomRolePermissions
	}{
		{
			name: "disjoint permissions",
			in: ModifyCustomRolePermissions{
				AddPermissions:    []Permission{PermissionCustomRoleRead},
				RemovePermissions: []Permission{PermissionCustomRoleUpdate},
			},
		},
		{
			name: "empty changes",
			in:   ModifyCustomRolePermissions{},
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

func TestModifyCustomRolePermissions_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   ModifyCustomRolePermissions
	}{
		{
			name: "permission in both sets",
			in: ModifyCustomRolePermissions{
				AddPermissions:    []Permission{PermissionCustomRoleRead},
				RemovePermissions: []Permission{PermissionCustomRoleRead},
			},
		},
		{
			name: "system-only permission to add",
			in: ModifyCustomRolePermissions{
				AddPermissions: []Permission{PermissionUserCreate},
			},
		},
		{
			name: "system-only permission to remove",
			in: ModifyCustomRolePermissions{
				RemovePermissions: []Permission{PermissionSystemRoleUnassign},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestCustomRole_MinimumAssignmentScope(t *testing.T) {
	tests := []struct {
		name string
		in   CustomRole
		want AssignmentScope
	}{
		{
			name: "project custom role",
			in: CustomRole{
				Kind:        RoleKindCustom,
				Permissions: []Permission{PermissionCustomRoleAssignProject},
			},
			want: AssignmentScopeProject,
		},
		{
			name: "custom role",
			in: CustomRole{
				Kind:        RoleKindCustom,
				Permissions: []Permission{PermissionCustomRoleAssignOrg},
			},
			want: AssignmentScopeOrganization,
		},
		{
			name: "organization admin",
			in: CustomRole{
				Kind:        RoleKindOrganizationAdmin,
				Permissions: []Permission{PermissionCustomRoleAssignProject},
			},
			want: AssignmentScopeOrganization,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.MinimumAssignmentScope(); got != tt.want {
				t.Errorf("MinimumAssignmentScope() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCustomRole_MinimumAssignmentScope_panicsForUnknownKind(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MinimumAssignmentScope() did not panic")
		}
	}()

	CustomRole{Kind: RoleKind(999)}.MinimumAssignmentScope()
}
