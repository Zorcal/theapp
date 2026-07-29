package conv

import (
	"testing"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

func TestCustomRoleToPB_managedRole(t *testing.T) {
	got := CustomRoleToPB(mdl.CustomRole{
		Kind:        mdl.RoleKindOrganizationAdmin,
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleAssignProject},
	})

	if want := pb.RoleKind_ROLE_KIND_ORGANIZATION_ADMIN; got.GetKind() != want {
		t.Errorf("CustomRoleToPB() kind = %v, want %v", got.GetKind(), want)
	}
	if want := pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION; got.GetMinimumAssignmentScope() != want {
		t.Errorf("CustomRoleToPB() minimum assignment scope = %v, want %v", got.GetMinimumAssignmentScope(), want)
	}
}

func TestCustomRoleToPB_panicsForUnknownKind(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("CustomRoleToPB() did not panic")
		}
	}()

	CustomRoleToPB(mdl.CustomRole{Kind: mdl.RoleKind(999)})
}
