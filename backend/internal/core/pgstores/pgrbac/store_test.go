package pgrbac

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_LockSystemRoleUser(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	rbacStore := NewStore(pool)

	usr, err := userStore.CreateUser(ctx, pguser.CreateUser{Email: "lock-system-role@test.com", Name: "Lock User"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if err := pgdb.NewTransactor(pool).RunTx(ctx, func(ctx context.Context) error {
		if err := rbacStore.LockSystemRoleUser(ctx, usr.ExternalID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("LockSystemRoleUser() error = %v", err)
	}
}

func TestStore_LockSystemRoleManagement(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	if err := pgdb.NewTransactor(pool).RunTx(ctx, func(ctx context.Context) error {
		if err := rbacStore.LockSystemRoleManagement(ctx); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("LockSystemRoleManagement() error = %v", err)
	}
}

func TestStore_SystemRoles(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	gotFirstPage, err := rbacStore.SystemRoles(ctx, 50, 0)
	if err != nil {
		t.Fatalf("SystemRoles() error = %v", err)
	}

	wantFirstPage := seededSystemRoles()

	testingx.AssertDiff(t, gotFirstPage, wantFirstPage)

	count, err := rbacStore.SystemRoleCount(ctx)
	if err != nil {
		t.Fatalf("SystemRoleCount() error = %v", err)
	}
	if wantCount := len(wantFirstPage); count != wantCount {
		t.Errorf("SystemRoleCount() = %d, want %d", count, wantCount)
	}

	gotSecondPage, err := rbacStore.SystemRoles(ctx, 50, 1)
	if err != nil {
		t.Fatalf("SystemRoles() second page error = %v", err)
	}

	wantSecondPage := []SystemRole{}

	testingx.AssertDiff(t, gotSecondPage, wantSecondPage)
}

func TestStore_SystemRoleByName(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	got, err := rbacStore.SystemRoleByName(ctx, "superadmin")
	if err != nil {
		t.Fatalf("SystemRoleByName() error = %v", err)
	}

	want := seededSystemRole(t, "superadmin")

	testingx.AssertDiff(t, got, want)
}

func TestStore_SystemRoleByName_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	if _, err := rbacStore.SystemRoleByName(ctx, "nonexistent"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("SystemRoleByName() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_UserSystemRolesByExternalID(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)
	usr := seedUser(t, userStore, "alice@test.com")

	gotBeforeAssignment, err := rbacStore.UserSystemRolesByExternalID(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRolesByExternalID() before assignment error = %v", err)
	}

	wantBeforeAssignment := []SystemRole{}

	testingx.AssertDiff(t, gotBeforeAssignment, wantBeforeAssignment)

	seedSystemRoleAssignment(t, ctx, pool, usr.ID, "superadmin")

	gotAfterAssignment, err := rbacStore.UserSystemRolesByExternalID(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRolesByExternalID() error = %v", err)
	}

	wantAfterAssignment := seededSystemRoles()

	testingx.AssertDiff(t, gotAfterAssignment, wantAfterAssignment)

	gotCount, err := rbacStore.UserSystemRoleCountByExternalID(ctx, usr.ExternalID)
	if err != nil {
		t.Fatalf("UserSystemRoleCountByExternalID() error = %v", err)
	}
	if wantCount := 1; gotCount != wantCount {
		t.Errorf("UserSystemRoleCountByExternalID() = %d, want %d", gotCount, wantCount)
	}
}

func TestStore_UserSystemRoleCountByExternalID_error(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)

		if _, err := rbacStore.UserSystemRoleCountByExternalID(ctx, uuid.New()); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UserSystemRoleCountByExternalID() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_UserSystemPermissionsByExternalID(t *testing.T) {
	t.Run("system scope", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")

		seedSystemRoleAssignment(t, ctx, pool, usr.ID, "superadmin")

		got, err := rbacStore.UserSystemPermissionsByExternalID(ctx, usr.ExternalID)
		if err != nil {
			t.Fatalf("UserSystemPermissionsByExternalID() error = %v", err)
		}

		want := []string{
			"system-role:assign",
			"system-role:read",
			"system-role:unassign",
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
		seedCustomRole(t, ctx, pool, "user-viewer", []string{"user:read"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", orgID)

		got, err := rbacStore.UserSystemPermissionsByExternalID(ctx, usr.ExternalID)
		if err != nil {
			t.Fatalf("UserSystemPermissionsByExternalID() error = %v", err)
		}

		// User was assigned a project role but not a system role.
		if len(got) != 0 {
			t.Errorf("UserSystemPermissionsByExternalID() = %v, want empty", got)
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

		seedSystemRoleAssignment(t, ctx, pool, usr.ID, "superadmin")

		orgID, projectID := seedOrgAndProject(t, pool, "acme")

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{
			OrgID: orgID,
			PermissionNames: []string{
				"system-role:assign",
				"system-role:read",
				"system-role:unassign",
				"user:create",
				"user:read",
				"user:update",
			},
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project scope, direct", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedCustomRole(t, ctx, pool, "user-viewer", []string{"user:read"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{
			OrgID:           orgID,
			PermissionNames: []string{"user:read"},
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("org scope, via project's org", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedCustomRole(t, ctx, pool, "user-viewer", []string{"user:read"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", orgID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{
			OrgID:           orgID,
			PermissionNames: []string{"user:read"},
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("union of project, org, and system scope is deduplicated", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedCustomRole(t, ctx, pool, "project-role", []string{"user:read"})
		seedCustomRole(t, ctx, pool, "org-role", []string{"user:read", "user:create"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "project-role", projectID)
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "org-role", orgID)

		seedSystemRoleAssignment(t, ctx, pool, usr.ID, "superadmin")

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{
			OrgID: orgID,
			PermissionNames: []string{
				"system-role:assign",
				"system-role:read",
				"system-role:unassign",
				"user:create",
				"user:read",
				"user:update",
			},
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project and org scope alone union to more than their intersection", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedCustomRole(t, ctx, pool, "project-role", []string{"user:read"})
		seedCustomRole(t, ctx, pool, "org-role", []string{"user:create"})
		orgID, projectID := seedOrgAndProject(t, pool, "acme")
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "project-role", projectID)
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "org-role", orgID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{
			OrgID:           orgID,
			PermissionNames: []string{"user:create", "user:read"},
		}

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
		seedCustomRole(t, ctx, pool, "user-viewer", []string{"user:read"})
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
		seedSystemRoleAssignment(t, ctx, pool, usr.ID, "superadmin")

		if _, err := rbacStore.ProjectPermissions(ctx, usr.ID, 999999); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ProjectPermissions() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_AssignSystemRole_error(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)

		if err := rbacStore.AssignSystemRole(ctx, uuid.New(), "superadmin"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("role not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")

		if err := rbacStore.AssignSystemRole(ctx, usr.ExternalID, "nonexistent"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("name matches a custom role, not a system one", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedCustomRole(t, ctx, pool, "some-project-role", nil)

		if err := rbacStore.AssignSystemRole(ctx, usr.ExternalID, "some-project-role"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("role already assigned", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedSystemRoleAssignment(t, ctx, pool, usr.ID, "superadmin")

		if err := rbacStore.AssignSystemRole(ctx, usr.ExternalID, "superadmin"); !errors.Is(err, pgdb.ErrAlreadyExists) {
			t.Errorf("AssignSystemRole() error = %v, want pgdb.ErrAlreadyExists", err)
		}
	})
}

func TestStore_UnassignSystemRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")
	seedSystemRoleAssignment(t, ctx, pool, usr.ID, "superadmin")

	if err := rbacStore.UnassignSystemRole(ctx, usr.ExternalID, "superadmin"); err != nil {
		t.Fatalf("UnassignSystemRole() error = %v", err)
	}

	got, err := rbacStore.UserSystemRolesByExternalID(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRolesByExternalID() error = %v", err)
	}

	want := []SystemRole{}

	testingx.AssertDiff(t, got, want)
}

func TestStore_SystemPermissionsRemainAfterUnassign(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	perms := []string{"system-role:assign", "system-role:unassign"}

	manager := seedUser(t, userStore, "manager@test.com")
	otherManager := seedUser(t, userStore, "other-manager@test.com")

	seedSystemRoleAssignment(t, ctx, pool, manager.ID, "superadmin")

	got, err := rbacStore.SystemPermissionsRemainAfterUnassign(ctx, manager.ExternalID, "superadmin", perms)
	if err != nil {
		t.Fatalf("SystemPermissionsRemainAfterUnassign() before second assignment error = %v", err)
	}
	if got {
		t.Error("SystemPermissionsRemainAfterUnassign() before second assignment = true, want false")
	}

	seedSystemRoleAssignment(t, ctx, pool, otherManager.ID, "superadmin")

	got, err = rbacStore.SystemPermissionsRemainAfterUnassign(ctx, manager.ExternalID, "superadmin", perms)
	if err != nil {
		t.Fatalf("SystemPermissionsRemainAfterUnassign() after second assignment error = %v", err)
	}
	if !got {
		t.Error("SystemPermissionsRemainAfterUnassign() after second assignment = false, want true")
	}
}

func TestStore_SystemPermissionsRemainAfterUnassign_error(t *testing.T) {
	t.Run("assignment not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		usr := seedUser(t, userStore, "no-assignment@test.com")

		if _, err := rbacStore.SystemPermissionsRemainAfterUnassign(ctx, usr.ExternalID, "superadmin", []string{"system-role:assign"}); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("SystemPermissionsRemainAfterUnassign() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_UnassignSystemRole_error(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)

		if err := rbacStore.UnassignSystemRole(ctx, uuid.New(), "superadmin"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("assignment not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")

		if err := rbacStore.UnassignSystemRole(ctx, usr.ExternalID, "superadmin"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("name matches a custom role, not a system one", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedCustomRole(t, ctx, pool, "some-project-role", nil)

		if err := rbacStore.UnassignSystemRole(ctx, usr.ExternalID, "some-project-role"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
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

func seededSystemRoles() []SystemRole {
	return []SystemRole{
		{
			Name: "superadmin",
			PermissionNames: []string{
				"system-role:assign",
				"system-role:read",
				"system-role:unassign",
				"user:create",
				"user:read",
				"user:update",
			},
		},
	}
}

func seededSystemRole(t *testing.T, name string) SystemRole {
	t.Helper()

	roles := seededSystemRoles()

	roleIdx := slices.IndexFunc(roles, func(role SystemRole) bool { return role.Name == name })
	if roleIdx == -1 {
		t.Fatalf("slices.IndexFunc(seededSystemRoles(), %q) = -1, want an index", name)
	}

	return roles[roleIdx]
}

func seedSystemRoleAssignment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int, roleName string) {
	t.Helper()

	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName}
	const q = `
		INSERT INTO rbac.system_role_assignments (user_id, role_id)
		SELECT @user_id, r.id FROM rbac.system_roles r WHERE r.name = @role_name`
	if _, err := pool.Exec(ctx, q, params); err != nil {
		t.Fatalf("seed system role assignment (user %d, role %q): %v", userID, roleName, err)
	}
}

// seedCustomRole seeds a custom role, owned by a dedicated throwaway org (custom_roles.org_id is
// NOT NULL, and the role's own owning org is irrelevant to what these tests exercise).
func seedCustomRole(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, permissionNames []string) {
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
