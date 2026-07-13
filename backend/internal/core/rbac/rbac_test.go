package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestCore_integration(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	core := NewCore(pgrbac.NewStore(pool))

	diffOpts := cmp.Options{
		cmpopts.SortSlices(func(a, b mdl.Permission) bool { return a < b }),
	}

	roles, err := core.Roles(ctx)
	if err != nil {
		t.Fatalf("Roles() error = %v", err)
	}

	wantRoles := []mdl.Role{
		{Name: "superadmin", IsStatic: true, Permissions: mdl.AllPermissions},
	}
	testingx.AssertDiff(t, roles, wantRoles, diffOpts)
}

func TestCore_Roles(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		RolesFunc: func(_ context.Context) ([]pgrbac.Role, error) {
			return []pgrbac.Role{
				{Name: "superadmin", IsStatic: true, PermissionNames: []string{"user:create", "user:read"}},
				{Name: "user-viewer", IsStatic: false, PermissionNames: []string{"user:read"}},
			}, nil
		},
	}
	core := NewCore(roleStorer)

	got, err := core.Roles(t.Context())
	if err != nil {
		t.Fatalf("Roles() error = %v", err)
	}

	want := []mdl.Role{
		{Name: "superadmin", IsStatic: true, Permissions: []mdl.Permission{mdl.PermissionUserCreate, mdl.PermissionUserRead}},
		{Name: "user-viewer", IsStatic: false, Permissions: []mdl.Permission{mdl.PermissionUserRead}},
	}
	testingx.AssertDiff(t, got, want)
}

func TestCore_Roles_error(t *testing.T) {
	t.Run("store error", func(t *testing.T) {
		core := NewCore(&MockedRoleStorer{
			RolesFunc: func(_ context.Context) ([]pgrbac.Role, error) {
				return nil, errors.New("db down")
			},
		})

		_, err := core.Roles(t.Context())
		if err == nil {
			t.Fatal("Roles() error = nil, want error")
		}
		testingx.AssertErrContains(t, err, "db down")
	})
}
