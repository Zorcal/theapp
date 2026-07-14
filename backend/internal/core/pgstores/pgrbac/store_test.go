package pgrbac

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_Roles(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
	seedRole(t, ctx, pool, "role-with-no-permissions", nil)

	got, err := rbacStore.Roles(ctx)
	if err != nil {
		t.Fatalf("Roles() error = %v", err)
	}
	want := []Role{
		{Name: "role-with-no-permissions", PermissionNames: []string{}},
		{Name: "superadmin", IsStatic: true, PermissionNames: []string{"user:create", "user:read", "user:update"}},
		{Name: "user-viewer", PermissionNames: []string{"user:read"}},
	}

	testingx.AssertDiff(t, got, want)
}

func TestIsStaticTrigger(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)

		_, err := pool.Exec(ctx, `UPDATE rbac.roles SET name = 'renamed' WHERE name = 'superadmin'`)
		if err == nil {
			t.Fatal("UPDATE static role error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "cannot be updated or deleted")
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)

		_, err := pool.Exec(ctx, `DELETE FROM rbac.roles WHERE name = 'superadmin'`)
		if err == nil {
			t.Fatal("DELETE static role error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "cannot be updated or deleted")
	})
}

func TestStore_SystemPermissions(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")

	if err := rbacStore.AssignSystemRole(ctx, usr.ID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}

	got, err := rbacStore.SystemPermissions(ctx, usr.ID)
	if err != nil {
		t.Fatalf("SystemPermissions() error = %v", err)
	}
	want := []string{"user:create", "user:read", "user:update"}
	testingx.AssertDiff(t, got, want)
}

func TestStore_SystemPermissions_noAssignments(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")

	got, err := rbacStore.SystemPermissions(ctx, usr.ID)
	if err != nil {
		t.Fatalf("SystemPermissions() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("SystemPermissions() = %v, want empty", got)
	}
}

func TestStore_AssignSystemRole_error(t *testing.T) {
	t.Run("role not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")

		if err := rbacStore.AssignSystemRole(ctx, usr.ID, "nonexistent"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("non-static role", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedRole(t, ctx, pool, "user-viewer", nil)

		err := rbacStore.AssignSystemRole(ctx, usr.ID, "user-viewer")
		if err == nil {
			t.Fatal("AssignSystemRole() error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "only a static role can be assigned at system scope")
	})
}

func seedUser(t *testing.T, s *pguser.Store, email string) pguser.User {
	t.Helper()

	usr, err := s.CreateUser(t.Context(), pguser.CreateUser{
		Email: email,
		Name:  "Test User",
	})
	if err != nil {
		t.Fatalf("seed user %q: %v", email, err)
	}

	return usr
}

func seedRole(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, permissionNames []string) {
	t.Helper()

	roleParams := pgx.NamedArgs{"name": name}
	const insertRole = `INSERT INTO rbac.roles (name, is_static, created_at) VALUES (@name, FALSE, NOW())`
	if _, err := pool.Exec(ctx, insertRole, roleParams); err != nil {
		t.Fatalf("seed role %q: %v", name, err)
	}

	const grantPermission = `
		INSERT INTO rbac.role_permissions (role_id, permission_id)
		SELECT r.id, p.id FROM rbac.roles r, rbac.permissions p WHERE r.name = @role_name AND p.name = @permission_name`
	for _, p := range permissionNames {
		grantParams := pgx.NamedArgs{"role_name": name, "permission_name": p}
		if _, err := pool.Exec(ctx, grantPermission, grantParams); err != nil {
			t.Fatalf("grant %q to role %q: %v", p, name, err)
		}
	}
}
