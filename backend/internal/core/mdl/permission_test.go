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
