package pgrbac

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_ProjectPermissions(t *testing.T) {
	t.Run("system scope, unconditional on project", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedSystemRoleAssignment(t, rbacStore, usr.ExternalID, "superadmin")
		org := seedOrg(t, orgStore, "acme")
		project := seedProject(t, orgStore, org.ID, "acme-project")
		orgID, projectID := org.ID, project.ID

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{
			OrgID:           orgID,
			PermissionNames: seededSystemRole(t, "superadmin").PermissionNames,
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project scope, direct", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, orgStore, "acme")
		project := seedProject(t, orgStore, org.ID, "acme-project")
		orgID, projectID := org.ID, project.ID
		seedOrgMembership(t, ctx, pool, usr.ID, orgID)
		role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "user-viewer", PermissionNames: []string{"user:read"}})
		seedProjectRoleAssignment(t, ctx, rbacStore, usr.ExternalID, role.ExternalID, projectID)

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
		orgStore := pgorg.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, orgStore, "acme")
		project := seedProject(t, orgStore, org.ID, "acme-project")
		orgID, projectID := org.ID, project.ID
		seedOrgMembership(t, ctx, pool, usr.ID, orgID)
		role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "user-viewer", PermissionNames: []string{"user:read"}})
		seedOrgRoleAssignment(t, ctx, rbacStore, usr.ExternalID, role.ExternalID, orgID)

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
		orgStore := pgorg.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, orgStore, "acme")
		project := seedProject(t, orgStore, org.ID, "acme-project")
		orgID, projectID := org.ID, project.ID
		seedOrgMembership(t, ctx, pool, usr.ID, orgID)
		projectRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "project-role", PermissionNames: []string{"user:read"}})
		orgRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "org-role", PermissionNames: []string{"user:read", "user:create"}})
		seedProjectRoleAssignment(t, ctx, rbacStore, usr.ExternalID, projectRole.ExternalID, projectID)
		seedOrgRoleAssignment(t, ctx, rbacStore, usr.ExternalID, orgRole.ExternalID, orgID)

		seedSystemRoleAssignment(t, rbacStore, usr.ExternalID, "superadmin")

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{
			OrgID:           orgID,
			PermissionNames: seededSystemRole(t, "superadmin").PermissionNames,
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project and org scope alone union to more than their intersection", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, orgStore, "acme")
		project := seedProject(t, orgStore, org.ID, "acme-project")
		orgID, projectID := org.ID, project.ID
		seedOrgMembership(t, ctx, pool, usr.ID, orgID)
		projectRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "project-role", PermissionNames: []string{"user:read"}})
		orgRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "org-role", PermissionNames: []string{"user:create"}})
		seedProjectRoleAssignment(t, ctx, rbacStore, usr.ExternalID, projectRole.ExternalID, projectID)
		seedOrgRoleAssignment(t, ctx, rbacStore, usr.ExternalID, orgRole.ExternalID, orgID)

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

	t.Run("membership removal suppresses project and org scope", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, orgStore, "acme")
		project := seedProject(t, orgStore, org.ID, "acme-project")
		seedOrgMembership(t, ctx, pool, usr.ID, org.ID)
		projectRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "project-role", PermissionNames: []string{"user:read"}})
		orgRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "org-role", PermissionNames: []string{"user:create"}})
		seedProjectRoleAssignment(t, ctx, rbacStore, usr.ExternalID, projectRole.ExternalID, project.ID)
		seedOrgRoleAssignment(t, ctx, rbacStore, usr.ExternalID, orgRole.ExternalID, org.ID)
		deleteOrgMembership(t, ctx, pool, usr.ID, org.ID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, project.ID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{
			OrgID:           org.ID,
			PermissionNames: []string{},
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("no assignments", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, orgStore, "acme")
		project := seedProject(t, orgStore, org.ID, "acme-project")
		orgID, projectID := org.ID, project.ID

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
		orgStore := pgorg.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, orgStore, "acme")
		project := seedProject(t, orgStore, org.ID, "acme-project")
		orgID, projectID := org.ID, project.ID
		seedOrgMembership(t, ctx, pool, usr.ID, orgID)
		role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "user-viewer", PermissionNames: []string{"user:read"}})
		seedProjectRoleAssignment(t, ctx, rbacStore, usr.ExternalID, role.ExternalID, projectID)

		otherProject := seedProject(t, orgStore, orgID, "other")

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
		seedSystemRoleAssignment(t, rbacStore, usr.ExternalID, "superadmin")

		if _, err := rbacStore.ProjectPermissions(ctx, usr.ID, 999999); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ProjectPermissions() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func seedUser(t *testing.T, userStore *pguser.Store, email string) pguser.User {
	t.Helper()

	usr, err := userStore.CreateUser(t.Context(), pguser.CreateUser{
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
				"custom-role:assign-org",
				"custom-role:assign-project",
				"custom-role:create",
				"custom-role:delete",
				"custom-role:read",
				"custom-role:read-org-assignments",
				"custom-role:read-project-assignments",
				"custom-role:unassign-org",
				"custom-role:unassign-project",
				"custom-role:update",
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

func seedCustomRole(t *testing.T, rbacStore *Store, role CreateCustomRole) CustomRole {
	t.Helper()

	created, err := rbacStore.CreateCustomRole(t.Context(), role)
	if err != nil {
		t.Fatalf("seed custom role %q: %v", role.Name, err)
	}

	return created
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

func seedSystemRoleAssignment(t *testing.T, rbacStore *Store, userID uuid.UUID, roleName string) {
	t.Helper()

	if err := rbacStore.AssignSystemRole(t.Context(), userID, roleName); err != nil {
		t.Fatalf("seed system role assignment (user %s, role %q): %v", userID, roleName, err)
	}
}

func seedOrg(t *testing.T, orgStore *pgorg.Store, name string) pgorg.Organization {
	t.Helper()

	org, err := orgStore.CreateOrganization(t.Context(), pgorg.CreateOrganization{
		Name:               name,
		ControlProjectName: "control",
	})
	if err != nil {
		t.Fatalf("seed organization %q: %v", name, err)
	}

	return org
}

func seedProject(t *testing.T, orgStore *pgorg.Store, orgID int, name string) pgorg.Project {
	t.Helper()

	project, err := orgStore.CreateProject(t.Context(), pgorg.CreateProject{OrgID: orgID, Name: name})
	if err != nil {
		t.Fatalf("seed project %q: %v", name, err)
	}

	return project
}

func seedOrgMembership(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	userID, orgID int,
) {
	t.Helper()

	if _, err := pool.Exec(
		ctx,
		"INSERT INTO org.org_membership (user_id, org_id) VALUES ($1, $2)",
		userID,
		orgID,
	); err != nil {
		t.Fatalf("seed org membership (user %d, org %d): %v", userID, orgID, err)
	}
}

func deleteOrgMembership(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	userID, orgID int,
) {
	t.Helper()

	if _, err := pool.Exec(
		ctx,
		"DELETE FROM org.org_membership WHERE user_id = $1 AND org_id = $2",
		userID,
		orgID,
	); err != nil {
		t.Fatalf("delete org membership (user %d, org %d): %v", userID, orgID, err)
	}
}

func seedProjectRoleAssignment(t *testing.T, ctx context.Context, rbacStore *Store, userID, roleID uuid.UUID, projectID int) {
	t.Helper()

	if err := rbacStore.AssignCustomRoleToProject(ctx, userID, roleID, projectID); err != nil {
		t.Fatalf("seed project role assignment (user %s, role %s, project %d): %v", userID, roleID, projectID, err)
	}
}

func seedOrgRoleAssignment(t *testing.T, ctx context.Context, rbacStore *Store, userID, roleID uuid.UUID, orgID int) {
	t.Helper()

	if err := rbacStore.AssignCustomRoleToOrg(ctx, userID, roleID, orgID); err != nil {
		t.Fatalf("seed org role assignment (user %s, role %s, org %d): %v", userID, roleID, orgID, err)
	}
}
