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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ExternalID, projectID)
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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ExternalID, projectID)
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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ExternalID, projectID)
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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ExternalID, projectID)
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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ExternalID, projectID)
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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ExternalID, project.ID)
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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ExternalID, projectID)
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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ExternalID, otherProject.ID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{}}

		testingx.AssertDiff(t, got, want)
	})
}

func TestStore_ProjectPermissions_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)
	orgStore := pgorg.NewStore(pool)

	user := seedUser(t, userStore, "project-permissions-error@test.com")
	org := seedOrg(t, orgStore, "project-permissions-error-org")
	project := seedProject(t, orgStore, org.ID, "project-permissions-error-project")

	tests := []struct {
		name      string
		userID    uuid.UUID
		projectID int
	}{
		{
			name:      "user missing",
			userID:    uuid.New(),
			projectID: project.ID,
		},
		{
			name:      "project missing",
			userID:    user.ExternalID,
			projectID: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := rbacStore.ProjectPermissions(ctx, tt.userID, tt.projectID); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("ProjectPermissions(%v, %d) error = %v, want sql.ErrNoRows", tt.userID, tt.projectID, err)
			}
		})
	}
}

func TestStore_OrgPermissionsByProjectID(t *testing.T) {
	t.Run("system scope, unconditional on organization membership", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		user := seedUser(t, userStore, "org-permissions-system@test.com")
		seedSystemRoleAssignment(t, rbacStore, user.ExternalID, "superadmin")
		org := seedOrg(t, orgStore, "org-permissions-system-org")
		project := seedProject(t, orgStore, org.ID, "project")

		got, err := rbacStore.OrgPermissionsByProjectID(ctx, user.ExternalID, project.ID)
		if err != nil {
			t.Fatalf("OrgPermissionsByProjectID() error = %v", err)
		}

		want := OrgPermissions{
			OrgID:           org.ID,
			PermissionNames: seededSystemRole(t, "superadmin").PermissionNames,
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("organization scope excludes project scope", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		user := seedUser(t, userStore, "org-permissions-scope@test.com")
		org := seedOrg(t, orgStore, "org-permissions-scope-org")
		project := seedProject(t, orgStore, org.ID, "project")
		seedOrgMembership(t, ctx, pool, user.ID, org.ID)
		orgRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "org role", PermissionNames: []string{"user:create"}})

		// A project assignment must not contribute to organization-scoped authorization.
		projectRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "project role", PermissionNames: []string{"user:read"}})
		seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, projectRole.ExternalID, project.ID)
		seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, orgRole.ExternalID, org.ID)

		got, err := rbacStore.OrgPermissionsByProjectID(ctx, user.ExternalID, project.ID)
		if err != nil {
			t.Fatalf("OrgPermissionsByProjectID() error = %v", err)
		}

		want := OrgPermissions{
			OrgID:           org.ID,
			PermissionNames: []string{"user:create"},
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("membership removal suppresses organization scope", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		user := seedUser(t, userStore, "org-permissions-membership@test.com")
		org := seedOrg(t, orgStore, "org-permissions-membership-org")
		project := seedProject(t, orgStore, org.ID, "project")
		seedOrgMembership(t, ctx, pool, user.ID, org.ID)
		role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "org role", PermissionNames: []string{"user:create"}})
		seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, role.ExternalID, org.ID)
		deleteOrgMembership(t, ctx, pool, user.ID, org.ID)

		got, err := rbacStore.OrgPermissionsByProjectID(ctx, user.ExternalID, project.ID)
		if err != nil {
			t.Fatalf("OrgPermissionsByProjectID() error = %v", err)
		}

		want := OrgPermissions{
			OrgID:           org.ID,
			PermissionNames: []string{},
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("organization scope does not leak to another organization", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		user := seedUser(t, userStore, "org-permissions-isolation@test.com")
		firstOrg := seedOrg(t, orgStore, "first-org-permissions-isolation-org")
		secondOrg := seedOrg(t, orgStore, "second-org-permissions-isolation-org")
		firstProject := seedProject(t, orgStore, firstOrg.ID, "project")
		seedOrgMembership(t, ctx, pool, user.ID, firstOrg.ID)
		seedOrgMembership(t, ctx, pool, user.ID, secondOrg.ID)
		firstRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: firstOrg.ID, Name: "first org role", PermissionNames: []string{"user:create"}})
		secondRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: secondOrg.ID, Name: "second org role", PermissionNames: []string{"user:read"}})
		seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, firstRole.ExternalID, firstOrg.ID)
		seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, secondRole.ExternalID, secondOrg.ID)

		got, err := rbacStore.OrgPermissionsByProjectID(ctx, user.ExternalID, firstProject.ID)
		if err != nil {
			t.Fatalf("OrgPermissionsByProjectID() error = %v", err)
		}

		want := OrgPermissions{
			OrgID:           firstOrg.ID,
			PermissionNames: []string{"user:create"},
		}

		testingx.AssertDiff(t, got, want)
	})

	t.Run("no assignments", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		user := seedUser(t, userStore, "org-permissions-empty@test.com")
		org := seedOrg(t, orgStore, "org-permissions-empty-org")
		project := seedProject(t, orgStore, org.ID, "project")

		got, err := rbacStore.OrgPermissionsByProjectID(ctx, user.ExternalID, project.ID)
		if err != nil {
			t.Fatalf("OrgPermissionsByProjectID() error = %v", err)
		}

		want := OrgPermissions{
			OrgID:           org.ID,
			PermissionNames: []string{},
		}

		testingx.AssertDiff(t, got, want)
	})
}

func TestStore_OrgPermissionsByProjectID_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)
	orgStore := pgorg.NewStore(pool)

	user := seedUser(t, userStore, "org-permissions-by-project-error@test.com")
	org := seedOrg(t, orgStore, "org-permissions-by-project-error-org")
	project := seedProject(t, orgStore, org.ID, "project")

	tests := []struct {
		name      string
		userID    uuid.UUID
		projectID int
	}{
		{
			name:      "user missing",
			userID:    uuid.New(),
			projectID: project.ID,
		},
		{
			name:      "project missing",
			userID:    user.ExternalID,
			projectID: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := rbacStore.OrgPermissionsByProjectID(ctx, tt.userID, tt.projectID); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("OrgPermissionsByProjectID(%v, %d) error = %v, want sql.ErrNoRows", tt.userID, tt.projectID, err)
			}
		})
	}
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
