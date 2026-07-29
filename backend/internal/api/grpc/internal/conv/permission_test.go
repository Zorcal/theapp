package conv

import (
	"slices"
	"testing"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

func TestPermissionToPB(t *testing.T) {
	tests := []struct {
		name   string
		in     mdl.Permission
		want   pb.Permission
		wantOK bool
	}{
		{
			name:   "known",
			in:     mdl.PermissionUserRead,
			want:   pb.Permission_PERMISSION_USER_READ,
			wantOK: true,
		},
		{
			name:   "unknown",
			in:     mdl.Permission("unknown"),
			want:   pb.Permission_PERMISSION_UNSPECIFIED,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PermissionToPB(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("PermissionToPB(%q) ok = %t, want %t", tt.in, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("PermissionToPB(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestPermissionFromPB(t *testing.T) {
	tests := []struct {
		name   string
		in     pb.Permission
		want   mdl.Permission
		wantOK bool
	}{
		{
			name:   "known",
			in:     pb.Permission_PERMISSION_USER_READ,
			want:   mdl.PermissionUserRead,
			wantOK: true,
		},
		{
			name:   "unspecified",
			in:     pb.Permission_PERMISSION_UNSPECIFIED,
			want:   "",
			wantOK: false,
		},
		{
			name:   "unknown",
			in:     pb.Permission(999),
			want:   "",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PermissionFromPB(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("PermissionFromPB(%v) ok = %t, want %t", tt.in, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("PermissionFromPB(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPermissionsFromPB(t *testing.T) {
	tests := []struct {
		name   string
		in     []pb.Permission
		want   []mdl.Permission
		wantOK bool
	}{
		{
			name:   "known",
			in:     []pb.Permission{pb.Permission_PERMISSION_USER_READ},
			want:   []mdl.Permission{mdl.PermissionUserRead},
			wantOK: true,
		},
		{
			name:   "unknown",
			in:     []pb.Permission{pb.Permission(999)},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PermissionsFromPB(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("PermissionsFromPB(%v) ok = %t, want %t", tt.in, ok, tt.wantOK)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("PermissionsFromPB(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestPermissionsToPB_panicsForUnknownPermission(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("PermissionsToPB() did not panic")
		}
	}()

	PermissionsToPB([]mdl.Permission{"unknown"})
}

func TestAssignmentScopeToPB_panicsForUnknownScope(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("AssignmentScopeToPB() did not panic")
		}
	}()

	AssignmentScopeToPB(mdl.AssignmentScope(999))
}

func TestPermissionConversions_exhaustive(t *testing.T) {
	mdlPerms := mdl.AllPermissions()

	if got, want := len(permissionToPB), len(mdlPerms); got != want {
		t.Errorf("permission conversion count = %d, want %d model permissions", got, want)
	}

	if got, want := len(permissionToPB), len(pb.Permission_name)-1; got != want {
		t.Errorf("permission conversion count = %d, want %d protobuf permissions", got, want)
	}

	for _, perm := range mdlPerms {
		pbPerm, ok := PermissionToPB(perm)
		if !ok {
			t.Errorf("PermissionToPB(%q) ok = false, want true", perm)
			continue
		}

		got, ok := PermissionFromPB(pbPerm)
		if !ok {
			t.Errorf("PermissionFromPB(%v) ok = false, want true", pbPerm)
			continue
		}

		if got != perm {
			t.Errorf("permission round trip = %q, want %q", got, perm)
		}
	}

	for value := range pb.Permission_name {
		perm := pb.Permission(value)
		if perm == pb.Permission_PERMISSION_UNSPECIFIED {
			continue
		}

		mdlPerm, ok := PermissionFromPB(perm)
		if !ok {
			t.Errorf("PermissionFromPB(%v) ok = false, want true", perm)
			continue
		}

		got, ok := PermissionToPB(mdlPerm)
		if !ok {
			t.Errorf("PermissionToPB(%q) ok = false, want true", mdlPerm)
			continue
		}

		if got != perm {
			t.Errorf("permission round trip = %v, want %v", got, perm)
		}
	}
}
