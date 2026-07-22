package pgrbac

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_StaticRoles(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	got, err := rbacStore.StaticRoles(ctx)
	if err != nil {
		t.Fatalf("StaticRoles() error = %v", err)
	}
	want := []RoleStatic{
		{Name: "superadmin", PermissionNames: []string{
			"role:assign", "role:assign-system", "role:create", "role:delete", "role:read", "role:read-system",
			"role:unassign", "role:unassign-system", "role:update", "user:create", "user:read", "user:update",
		}},
	}

	testingx.AssertDiff(t, got, want, cmpopts.IgnoreFields(RoleStatic{}, "ID", "ExternalID", "CreatedAt", "UpdatedAt"))
}

func TestStore_StaticRoleByName(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	got, err := rbacStore.StaticRoleByName(ctx, "superadmin")
	if err != nil {
		t.Fatalf("StaticRoleByName() error = %v", err)
	}

	want := RoleStatic{Name: "superadmin", PermissionNames: []string{
		"role:assign", "role:assign-system", "role:create", "role:delete", "role:read", "role:read-system",
		"role:unassign", "role:unassign-system", "role:update", "user:create", "user:read", "user:update",
	}}

	testingx.AssertDiff(t, got, want, cmpopts.IgnoreFields(RoleStatic{}, "ID", "ExternalID", "CreatedAt", "UpdatedAt"))
}

func TestStore_StaticRoleByName_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	if _, err := rbacStore.StaticRoleByName(ctx, "nonexistent"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("StaticRoleByName() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_SystemRoleAssignmentsForUser(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")

	got, err := rbacStore.SystemRoleAssignmentsForUser(ctx, usr.ID)
	if err != nil {
		t.Fatalf("SystemRoleAssignmentsForUser() error = %v", err)
	}
	testingx.AssertDiff(t, got, []string{})

	if err := rbacStore.AssignSystemRole(ctx, usr.ID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}

	got, err = rbacStore.SystemRoleAssignmentsForUser(ctx, usr.ID)
	if err != nil {
		t.Fatalf("SystemRoleAssignmentsForUser() error = %v", err)
	}
	testingx.AssertDiff(t, got, []string{"superadmin"})
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
			"role:assign",
			"role:assign-system",
			"role:create",
			"role:delete",
			"role:read",
			"role:read-system",
			"role:unassign",
			"role:unassign-system",
			"role:update",
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

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{
			"role:assign", "role:assign-system", "role:create", "role:delete", "role:read", "role:read-system",
			"role:unassign", "role:unassign-system", "role:update", "user:create", "user:read", "user:update",
		}}

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

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{
			"role:assign", "role:assign-system", "role:create", "role:delete", "role:read", "role:read-system",
			"role:unassign", "role:unassign-system", "role:update", "user:create", "user:read", "user:update",
		}}

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

	// A name that only matches a custom role, not a static one, resolves the same as a name that
	// matches nothing at all: assignSystemRoleQuery looks up rbac.static_roles specifically, so a
	// custom role's name simply isn't there to find.
	t.Run("name matches a custom role, not a static one", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedRole(t, ctx, pool, "user-viewer", nil)

		if err := rbacStore.AssignSystemRole(ctx, usr.ID, "user-viewer"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_AssignProjectRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")
	seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
	_, projectID := seedOrgAndProject(t, pool, "acme")
	roleID := customRoleIDByName(t, ctx, pool, "user-viewer")

	if err := rbacStore.AssignProjectRole(ctx, usr.ID, roleID, projectID); err != nil {
		t.Fatalf("AssignProjectRole() error = %v", err)
	}

	// Assigning again is a no-op.
	if err := rbacStore.AssignProjectRole(ctx, usr.ID, roleID, projectID); err != nil {
		t.Fatalf("AssignProjectRole() again error = %v", err)
	}

	got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
	if err != nil {
		t.Fatalf("ProjectPermissions() error = %v", err)
	}
	testingx.AssertDiff(t, got.PermissionNames, []string{"user:read"})
}

func TestStore_AssignOrgRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")
	seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
	orgID, projectID := seedOrgAndProject(t, pool, "acme")
	roleID := customRoleIDByName(t, ctx, pool, "user-viewer")

	if err := rbacStore.AssignOrgRole(ctx, usr.ID, roleID, orgID); err != nil {
		t.Fatalf("AssignOrgRole() error = %v", err)
	}

	got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
	if err != nil {
		t.Fatalf("ProjectPermissions() error = %v", err)
	}
	testingx.AssertDiff(t, got.PermissionNames, []string{"user:read"})
}

func TestStore_UnassignProjectRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")
	seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
	_, projectID := seedOrgAndProject(t, pool, "acme")
	roleID := customRoleIDByName(t, ctx, pool, "user-viewer")
	seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)

	if err := rbacStore.UnassignProjectRole(ctx, usr.ID, roleID, projectID); err != nil {
		t.Fatalf("UnassignProjectRole() error = %v", err)
	}

	got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
	if err != nil {
		t.Fatalf("ProjectPermissions() error = %v", err)
	}
	testingx.AssertDiff(t, got.PermissionNames, []string{})

	// Unassigning again (no such grant) is a no-op.
	if err := rbacStore.UnassignProjectRole(ctx, usr.ID, roleID, projectID); err != nil {
		t.Fatalf("UnassignProjectRole() again error = %v", err)
	}
}

func TestStore_UnassignOrgRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")
	seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
	orgID, projectID := seedOrgAndProject(t, pool, "acme")
	roleID := customRoleIDByName(t, ctx, pool, "user-viewer")
	seedOrgRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", orgID)

	if err := rbacStore.UnassignOrgRole(ctx, usr.ID, roleID, orgID); err != nil {
		t.Fatalf("UnassignOrgRole() error = %v", err)
	}

	got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
	if err != nil {
		t.Fatalf("ProjectPermissions() error = %v", err)
	}
	testingx.AssertDiff(t, got.PermissionNames, []string{})
}

func TestStore_OrgAssignmentExists(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")
	seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
	orgID, _ := seedOrgAndProject(t, pool, "acme")
	roleID := customRoleIDByName(t, ctx, pool, "user-viewer")

	got, err := rbacStore.OrgAssignmentExists(ctx, usr.ID, roleID, orgID)
	if err != nil {
		t.Fatalf("OrgAssignmentExists() error = %v", err)
	}
	if got {
		t.Error("OrgAssignmentExists() = true, want false")
	}

	seedOrgRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", orgID)

	got, err = rbacStore.OrgAssignmentExists(ctx, usr.ID, roleID, orgID)
	if err != nil {
		t.Fatalf("OrgAssignmentExists() error = %v", err)
	}
	if !got {
		t.Error("OrgAssignmentExists() = false, want true")
	}
}

func TestStore_DeleteProjectAssignmentsForOrg(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")
	seedRole(t, ctx, pool, "user-viewer", []string{"user:read"})
	orgID, projectID := seedOrgAndProject(t, pool, "acme")
	roleID := customRoleIDByName(t, ctx, pool, "user-viewer")
	seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)

	if err := rbacStore.DeleteProjectAssignmentsForOrg(ctx, usr.ID, roleID, orgID); err != nil {
		t.Fatalf("DeleteProjectAssignmentsForOrg() error = %v", err)
	}

	got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
	if err != nil {
		t.Fatalf("ProjectPermissions() error = %v", err)
	}
	testingx.AssertDiff(t, got.PermissionNames, []string{})
}

func TestStore_RoleAssignmentsForUser(t *testing.T) {
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

	got, err := rbacStore.RoleAssignmentsForUser(ctx, usr.ID, orgID)
	if err != nil {
		t.Fatalf("RoleAssignmentsForUser() error = %v", err)
	}

	want := []RoleAssignment{
		{RoleName: "org-role", OrgID: &orgID},
		{RoleName: "project-role", ProjectID: &projectID},
	}

	testingx.AssertDiff(t, got, want, cmpopts.IgnoreFields(RoleAssignment{}, "RoleExternalID"))
}

func customRoleIDByName(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) int {
	t.Helper()

	var id int
	if err := pool.QueryRow(ctx, `SELECT id FROM rbac.custom_roles WHERE name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("custom role id by name %q: %v", name, err)
	}
	return id
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

// seedRole seeds a custom role, owned by a dedicated throwaway org (custom_roles.org_id is
// NOT NULL, and the role's own owning org is irrelevant to what these tests exercise).
func seedRole(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, permissionNames []string) {
	t.Helper()

	org, err := pgorg.NewStore(pool).CreateOrganization(ctx, pgorg.CreateOrganization{Name: name + "-role-org", ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("seed org for role %q: %v", name, err)
	}

	roleParams := pgx.NamedArgs{"name": name, "org_id": org.ID}
	const insertRole = `
		INSERT INTO rbac.custom_roles (external_id, org_id, name, created_at, etag)
		VALUES (gen_random_uuid(), @org_id, @name, NOW(), gen_random_uuid())`
	if _, err := pool.Exec(ctx, insertRole, roleParams); err != nil {
		t.Fatalf("seed role %q: %v", name, err)
	}

	const grantPermission = `
		INSERT INTO rbac.custom_role_permissions (role_id, permission_id)
		SELECT r.id, p.id FROM rbac.custom_roles r, rbac.permissions p WHERE r.name = @role_name AND p.name = @permission_name`
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
		SELECT @user_id, r.id, @project_id FROM rbac.custom_roles r WHERE r.name = @role_name`
	if _, err := pool.Exec(ctx, q, params); err != nil {
		t.Fatalf("seed project role assignment (user %d, role %q, project %d): %v", userID, roleName, projectID, err)
	}
}

func seedOrgRoleAssignment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int, roleName string, orgID int) {
	t.Helper()

	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName, "org_id": orgID}
	const q = `
		INSERT INTO rbac.org_role_assignments (user_id, role_id, org_id)
		SELECT @user_id, r.id, @org_id FROM rbac.custom_roles r WHERE r.name = @role_name`
	if _, err := pool.Exec(ctx, q, params); err != nil {
		t.Fatalf("seed org role assignment (user %d, role %q, org %d): %v", userID, roleName, orgID, err)
	}
}
