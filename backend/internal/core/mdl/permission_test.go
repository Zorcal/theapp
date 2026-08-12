package mdl

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/zorcal/theapp/backend/internal/testingx"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func TestPermissionRegistries(t *testing.T) {
	systemOnly := SystemOnlyPermissions()
	customRole := PermissionsAssignableToCustomRoles()

	for _, permission := range systemOnly {
		if slices.Contains(customRole, permission) {
			t.Errorf("permission %q is both system-only and custom-role assignable", permission)
		}
	}

	testingx.AssertDiff(
		t,
		slices.Concat(systemOnly, customRole),
		AllPermissions(),
		cmp.Options{
			cmpopts.SortSlices(func(a, b Permission) bool { return a < b }),
		},
	)
}

func TestCustomRolePermissionDescriptors(t *testing.T) {
	descriptors := CustomRolePermissionDescriptors()
	permissions := PermissionsAssignableToCustomRoles()

	if got, want := len(descriptors), len(permissions); got != want {
		t.Fatalf("CustomRolePermissionDescriptors() len = %d, want %d", got, want)
	}

	for _, descriptor := range descriptors {
		if !slices.Contains(permissions, descriptor.Permission) {
			t.Errorf("CustomRolePermissionDescriptors() contains unavailable permission %q", descriptor.Permission)
		}
		if got, want := descriptor.MinimumAssignmentScope, PermissionAssignmentScope(descriptor.Permission); got != want {
			t.Errorf("CustomRolePermissionDescriptors() scope for %q = %d, want %d", descriptor.Permission, got, want)
		}
	}
	for _, permission := range permissions {
		count := slicesx.CountFunc(descriptors, func(descriptor PermissionDescriptor) bool {
			return descriptor.Permission == permission
		})
		if wantCount := 1; count != wantCount {
			t.Errorf("CustomRolePermissionDescriptors() count for %q = %d, want %d", permission, count, wantCount)
		}
	}
}

func TestOrganizationAdminPermissions(t *testing.T) {
	customRolePermissions := PermissionsAssignableToCustomRoles()

	for _, permission := range OrganizationAdminPermissions() {
		if !slices.Contains(customRolePermissions, permission) {
			t.Errorf("OrganizationAdminPermissions() contains unavailable permission %q", permission)
		}
	}
}

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
