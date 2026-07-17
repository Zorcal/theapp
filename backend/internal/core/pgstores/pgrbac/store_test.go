package pgrbac

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
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
	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "update",
			sql:  `UPDATE rbac.roles SET name = 'renamed' WHERE name = 'superadmin'`,
		},
		{
			name: "delete",
			sql:  `DELETE FROM rbac.roles WHERE name = 'superadmin'`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := pgtest.New(t, ctx)

			_, err := pool.Exec(ctx, tt.sql)
			if err == nil {
				t.Fatalf("Exec(%q) error = nil, want error", tt.sql)
			}

			testingx.AssertErrContains(t, err, "cannot be updated or deleted")
		})
	}
}

func TestStore_SystemPermissions(t *testing.T) {
	t.Run("system scope", func(t *testing.T) {
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

		want := []string{
			"user:create",
			"user:read",
			"user:update",
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project and org scope do not leak into system scope", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", orgID)

		got, err := rbacStore.SystemPermissions(ctx, usr.ID)
		if err != nil {
			t.Fatalf("SystemPermissions() error = %v", err)
		}

		// User was assigned a project role but not a system role.
		if len(got) != 0 {
			t.Errorf("SystemPermissions() = %v, want empty", got)
		}
	})
}

func TestStore_ProjectPermissions(t *testing.T) {
	t.Run("system scope, unconditional on project", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		if err := rbacStore.AssignSystemRole(ctx, usr.ID, "superadmin"); err != nil {
			t.Fatalf("AssignSystemRole() error = %v", err)
		}
		orgID, projectID := seedOrgAndProject(t, pool, "acme")

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{"user:create", "user:read", "user:update"}}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project scope, direct", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{"user:read"}}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("org scope, via project's org", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", orgID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{"user:read"}}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("union of project, org, and system scope is deduplicated", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedRole(t, ctx, pool, "project-role", []string{"user:read"})
		seedRole(t, ctx, pool, "org-role", []string{"user:read", "user:create"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "project-role", projectID)
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "org-role", orgID)
		if err := rbacStore.AssignSystemRole(ctx, usr.ID, "superadmin"); err != nil {
			t.Fatalf("AssignSystemRole() error = %v", err)
		}

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{"user:create", "user:read", "user:update"}}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project and org scope alone union to more than their intersection", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedRole(t, ctx, pool, "project-role", []string{"user:read"})
		seedRole(t, ctx, pool, "org-role", []string{"user:create"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "project-role", projectID)
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "org-role", orgID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{"user:create", "user:read"}}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("no assignments", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		orgID, projectID := seedOrgAndProject(t, pool, "acme")

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{}}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project scope does not leak to a different project", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)

		otherProject, err := pgorg.NewStore(pool).CreateProject(ctx, pgorg.CreateProject{OrgID: orgID, Name: "other"})
		if err != nil {
			t.Fatalf("seed other project: %v", err)
		}

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, otherProject.ID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{}}

		testingx.AssertDiff(t, got, want)
	})
}

func TestStore_ProjectPermissions_error(t *testing.T) {
	t.Run("project not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		if err := rbacStore.AssignSystemRole(ctx, usr.ID, "superadmin"); err != nil {
			t.Fatalf("AssignSystemRole() error = %v", err)
		}

		if _, err := rbacStore.ProjectPermissions(ctx, usr.ID, 999999); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ProjectPermissions() error = %v, want sql.ErrNoRows", err)
		}
	})
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

func seedOrgAndProject(t *testing.T, pool *pgxpool.Pool, name string) (orgID, projectID int) {
	t.Helper()

	orgStore := pgorg.NewStore(pool)

	org, err := orgStore.CreateOrganization(t.Context(), pgorg.CreateOrganization{Name: name, ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("seed org %q: %v", name, err)
	}

	project, err := orgStore.CreateProject(t.Context(), pgorg.CreateProject{OrgID: org.ID, Name: name + "-project"})
	if err != nil {
		t.Fatalf("seed project for org %q: %v", name, err)
	}

	return org.ID, project.ID
}

func seedProjectRoleAssignment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int, roleName string, projectID int) {
	t.Helper()

	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName, "project_id": projectID}
	const q = `
		INSERT INTO rbac.project_role_assignments (user_id, role_id, project_id)
		SELECT @user_id, r.id, @project_id FROM rbac.roles r WHERE r.name = @role_name`
	if _, err := pool.Exec(ctx, q, params); err != nil {
		t.Fatalf("seed project role assignment (user %d, role %q, project %d): %v", userID, roleName, projectID, err)
	}
}

func seedOrgRoleAssignment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int, roleName string, orgID int) {
	t.Helper()

	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName, "org_id": orgID}
	const q = `
		INSERT INTO rbac.org_role_assignments (user_id, role_id, org_id)
		SELECT @user_id, r.id, @org_id FROM rbac.roles r WHERE r.name = @role_name`
	if _, err := pool.Exec(ctx, q, params); err != nil {
		t.Fatalf("seed org role assignment (user %d, role %q, org %d): %v", userID, roleName, orgID, err)
	}
}
