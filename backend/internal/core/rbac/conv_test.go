package rbac

import (
	"testing"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
)

func TestCustomRoleFromPg_managedRole(t *testing.T) {
	got := customRoleFromPg(pgrbac.CustomRole{
		ManagedKey: new(mdl.ManagedRoleKeyOrganizationAdmin),
	})

	if want := mdl.RoleKindOrganizationAdmin; got.Kind != want {
		t.Errorf("customRoleFromPg() kind = %d, want %d", got.Kind, want)
	}
}

func TestCustomRoleFromPg_panicsForUnknownManagedKey(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("customRoleFromPg() did not panic")
		}
	}()

	customRoleFromPg(pgrbac.CustomRole{ManagedKey: new("unknown")})
}
