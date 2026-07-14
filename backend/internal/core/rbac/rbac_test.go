package rbac

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestCore_integration(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	core := NewCore(pgrbac.NewStore(pool), userStore)

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

	usr, err := userStore.CreateUser(ctx, pguser.CreateUser{Email: "alice@test.com", Name: "Alice Smith"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if err := core.AssignSystemRole(ctx, usr.ExternalID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}
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
	core := NewCore(roleStorer, &MockedUserStorer{})

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
		}, &MockedUserStorer{})

		_, err := core.Roles(t.Context())
		if err == nil {
			t.Fatal("Roles() error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "db down")
	})
}

func TestCore_AssignSystemRole(t *testing.T) {
	userStorer := &MockedUserStorer{
		UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
			return pguser.User{ID: 7}, nil
		},
	}
	roleStorer := &MockedRoleStorer{
		AssignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
			return nil
		},
	}
	core := NewCore(roleStorer, userStorer)

	if err := core.AssignSystemRole(t.Context(), uuid.New(), "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}
}

func TestCore_AssignSystemRole_error(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		core := NewCore(&MockedRoleStorer{}, &MockedUserStorer{
			UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
				return pguser.User{}, sql.ErrNoRows
			},
		})

		if err := core.AssignSystemRole(t.Context(), uuid.New(), "superadmin"); !errors.Is(err, mdl.ErrNotFound) {
			t.Errorf("AssignSystemRole() error = %v, want mdl.ErrNotFound", err)
		}
	})

	t.Run("role not found", func(t *testing.T) {
		core := NewCore(&MockedRoleStorer{
			AssignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
				return sql.ErrNoRows
			},
		}, &MockedUserStorer{
			UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
				return pguser.User{ID: 7}, nil
			},
		})

		if err := core.AssignSystemRole(t.Context(), uuid.New(), "nonexistent"); !errors.Is(err, mdl.ErrNotFound) {
			t.Errorf("AssignSystemRole() error = %v, want mdl.ErrNotFound", err)
		}
	})

	t.Run("store error", func(t *testing.T) {
		core := NewCore(&MockedRoleStorer{
			AssignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
				return errors.New("db down")
			},
		}, &MockedUserStorer{
			UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
				return pguser.User{ID: 7}, nil
			},
		})

		err := core.AssignSystemRole(t.Context(), uuid.New(), "superadmin")
		if err == nil {
			t.Fatal("AssignSystemRole() error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "db down")
	})
}
