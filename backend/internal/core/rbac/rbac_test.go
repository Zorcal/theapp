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

	staticRoles, err := core.StaticRoles(ctx)
	if err != nil {
		t.Fatalf("StaticRoles() error = %v", err)
	}

	wantStaticRoles := []mdl.RoleStatic{
		{Name: "superadmin", Permissions: mdl.AllPermissions},
	}

	testingx.AssertDiff(t, staticRoles, wantStaticRoles, diffOpts)

	usr, err := userStore.CreateUser(ctx, pguser.CreateUser{Email: "alice@test.com", Name: "Alice Smith"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if err := core.AssignSystemRole(ctx, usr.ExternalID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}
}

func TestCore_StaticRoles(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		StaticRolesFunc: func(_ context.Context) ([]pgrbac.RoleStatic, error) {
			return []pgrbac.RoleStatic{
				{Name: "superadmin", PermissionNames: []string{"user:create", "user:read"}},
			}, nil
		},
	}
	core := NewCore(roleStorer, &MockedUserStorer{})

	got, err := core.StaticRoles(t.Context())
	if err != nil {
		t.Fatalf("StaticRoles() error = %v", err)
	}

	want := []mdl.RoleStatic{
		{Name: "superadmin", Permissions: []mdl.Permission{mdl.PermissionUserCreate, mdl.PermissionUserRead}},
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_StaticRoles_error(t *testing.T) {
	dbErr := errors.New("db error")

	core := NewCore(&MockedRoleStorer{
		StaticRolesFunc: func(_ context.Context) ([]pgrbac.RoleStatic, error) {
			return nil, dbErr
		},
	}, &MockedUserStorer{})

	if _, err := core.StaticRoles(t.Context()); !errors.Is(err, dbErr) {
		t.Errorf("StaticRoles() error = %v, want %v", err, dbErr)
	}
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
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		userStorer *MockedUserStorer
		want       error
	}{
		{
			name:       "user not found",
			roleStorer: &MockedRoleStorer{},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				AssignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
					return sql.ErrNoRows
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name:       "store error, user lookup",
			roleStorer: &MockedRoleStorer{},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "store error, role assignment",
			roleStorer: &MockedRoleStorer{
				AssignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
					return dbErr
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, tt.userStorer)

			if err := core.AssignSystemRole(t.Context(), uuid.New(), "superadmin"); !errors.Is(err, tt.want) {
				t.Errorf("AssignSystemRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}
