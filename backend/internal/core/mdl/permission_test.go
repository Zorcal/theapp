package mdl

import "testing"

func TestIsPermissionSuperset(t *testing.T) {
	tests := []struct {
		name     string
		held     []Permission
		required []Permission
		want     bool
	}{
		{
			name:     "equal sets",
			held:     []Permission{PermissionUserRead, PermissionUserUpdate},
			required: []Permission{PermissionUserRead, PermissionUserUpdate},
			want:     true,
		},
		{
			name:     "strict superset",
			held:     []Permission{PermissionUserRead, PermissionUserCreate, PermissionUserUpdate},
			required: []Permission{PermissionUserRead, PermissionUserUpdate},
			want:     true,
		},
		{
			name:     "missing permission",
			held:     []Permission{PermissionUserRead},
			required: []Permission{PermissionUserRead, PermissionUserUpdate},
			want:     false,
		},
		{
			name:     "nothing required",
			held:     []Permission{PermissionUserRead},
			required: []Permission{},
			want:     true,
		},
		{
			name:     "nothing held",
			held:     []Permission{},
			required: []Permission{PermissionUserRead},
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPermissionSuperset(tt.held, tt.required); got != tt.want {
				t.Errorf("IsPermissionSuperset(%v, %v) = %t, want %t", tt.held, tt.required, got, tt.want)
			}
		})
	}
}

func TestPermissionAssignmentScope(t *testing.T) {
	tests := []struct {
		name string
		in   Permission
		want AssignmentScope
	}{
		{
			name: "project",
			in:   PermissionCustomRoleAssignProject,
			want: AssignmentScopeProject,
		},
		{
			name: "organization",
			in:   PermissionProjectCreate,
			want: AssignmentScopeOrganization,
		},
		{
			name: "system",
			in:   PermissionSystemRoleAssign,
			want: AssignmentScopeSystem,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PermissionAssignmentScope(tt.in); got != tt.want {
				t.Errorf("PermissionAssignmentScope(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestPermissionAssignmentScope_panicsForUnknownPermission(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("PermissionAssignmentScope() did not panic")
		}
	}()

	PermissionAssignmentScope("unknown")
}

func TestPermissionAssignmentScope_exhaustive(t *testing.T) {
	for _, permission := range AllPermissions() {
		t.Run(string(permission), func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Errorf("PermissionAssignmentScope(%q) panicked: %v", permission, recovered)
				}
			}()

			PermissionAssignmentScope(permission)
		})
	}
}
