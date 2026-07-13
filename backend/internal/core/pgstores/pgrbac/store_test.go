package pgrbac

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_Roles(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
	seedRole(t, ctx, pool, "role-with-no-permissions", nil)

	got, err := store.Roles(ctx)
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
