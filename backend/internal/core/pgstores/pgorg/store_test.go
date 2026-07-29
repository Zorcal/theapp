package pgorg_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_CreateOrganization(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	got, err := orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{Name: "acme", ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	want := pgorg.Organization{
		Name:      "acme",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(
		t, got, want,
		cmpopts.IgnoreFields(pgorg.Organization{}, "ID", "ControlProjectID"),
		cmpopts.EquateApproxTime(time.Minute),
	)

	if got.ID == 0 {
		t.Error("CreateOrganization() ID = 0, want non-zero")
	}
	if got.ControlProjectID == 0 {
		t.Error("CreateOrganization() ControlProjectID = 0, want non-zero")
	}

	control := mustProjectByName(t, orgStore, got.ID, "control")

	wantControl := pgorg.Project{
		OrgID:     got.ID,
		Name:      "control",
		IsControl: true,
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(
		t, control, wantControl,
		cmpopts.IgnoreFields(pgorg.Project{}, "ID", "ETag"),
		cmpopts.EquateApproxTime(time.Minute),
	)

	if control.ID != got.ControlProjectID {
		t.Errorf("control project ID = %d, want %d (Organization.ControlProjectID)", control.ID, got.ControlProjectID)
	}
	if control.ETag == uuid.Nil {
		t.Error("control project ETag is nil, want non-nil")
	}
}

func TestStore_CreateOrganization_error(t *testing.T) {
	t.Run("duplicate name", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)

		seedOrg(t, orgStore, "acme")

		if _, err := orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{Name: "acme"}); !errors.Is(err, pgdb.ErrAlreadyExists) {
			t.Errorf("CreateOrganization() error = %v, want pgdb.ErrAlreadyExists", err)
		}
	})
}

func TestStore_CreateProject(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	org := seedOrg(t, orgStore, "acme")

	got, err := orgStore.CreateProject(ctx, pgorg.CreateProject{OrgID: org.ID, Name: "widgets"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	want := pgorg.Project{
		OrgID:     org.ID,
		Name:      "widgets",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(
		t, got, want,
		cmpopts.IgnoreFields(pgorg.Project{}, "ID", "ETag"),
		cmpopts.EquateApproxTime(time.Minute),
	)

	if got.ID == 0 {
		t.Error("CreateProject() ID = 0, want non-zero")
	}
	if got.ETag == uuid.Nil {
		t.Error("CreateProject() ETag is nil, want non-nil")
	}
}

func TestStore_CreateProject_error(t *testing.T) {
	t.Run("org not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)

		if _, err := orgStore.CreateProject(ctx, pgorg.CreateProject{OrgID: 999999, Name: "widgets"}); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("CreateProject() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)

		org := seedOrg(t, orgStore, "acme")
		seedProject(t, orgStore, org.ID, "widgets")

		tests := []struct {
			name string
			dup  string
		}{
			{
				name: "same case",
				dup:  "widgets",
			},
			{
				name: "different case",
				dup:  "WIDGETS",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if _, err := orgStore.CreateProject(ctx, pgorg.CreateProject{OrgID: org.ID, Name: tt.dup}); !errors.Is(err, pgdb.ErrAlreadyExists) {
					t.Errorf("CreateProject() error = %v, want pgdb.ErrAlreadyExists", err)
				}
			})
		}
	})
}

func TestStore_OrganizationByName(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	seeded := seedOrg(t, orgStore, "acme")

	got, err := orgStore.OrganizationByName(ctx, "acme")
	if err != nil {
		t.Fatalf("OrganizationByName() error = %v", err)
	}

	testingx.AssertDiff(t, got, seeded)
}

func TestStore_OrganizationByName_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	if _, err := orgStore.OrganizationByName(ctx, "acme"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("OrganizationByName() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_ProjectByID(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	org := seedOrg(t, orgStore, "acme")
	seeded := seedProject(t, orgStore, org.ID, "widgets")

	got, err := orgStore.ProjectByID(ctx, seeded.ID)
	if err != nil {
		t.Fatalf("ProjectByID() error = %v", err)
	}

	testingx.AssertDiff(t, got, seeded)
}

func TestStore_ProjectByID_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	if _, err := orgStore.ProjectByID(ctx, 999999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("ProjectByID() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_ProjectByName(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{
			name: "exact case",
			in:   "widgets",
		},
		{
			name: "different case",
			in:   "WIDGETS",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := pgtest.New(t, ctx)
			orgStore := pgorg.NewStore(pool)

			org := seedOrg(t, orgStore, "acme")
			seeded := seedProject(t, orgStore, org.ID, "widgets")

			got, err := orgStore.ProjectByName(ctx, org.ID, tt.in)
			if err != nil {
				t.Fatalf("ProjectByName(%q) error = %v", tt.in, err)
			}

			testingx.AssertDiff(t, got, seeded)
		})
	}
}

func TestStore_ProjectByName_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	org := seedOrg(t, orgStore, "acme")

	if _, err := orgStore.ProjectByName(ctx, org.ID, "widgets"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("ProjectByName() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_AccessibleProjects(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)
	userStore := pguser.NewStore(pool)

	// Give one user direct access to one project. The second project proves that a direct assignment
	// does not expand within its organization.
	projectAssignedUser := seedUser(t, userStore, "project-assignment@test.com")
	projectAssignedOrg := seedOrg(t, orgStore, "project-assignment-org")
	projectAssignedProject := seedProject(t, orgStore, projectAssignedOrg.ID, "first")
	seedProject(t, orgStore, projectAssignedOrg.ID, "second")
	seedOrgMembership(t, pool, projectAssignedUser.ID, projectAssignedOrg.ID)
	projectRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: projectAssignedOrg.ID, Name: "project role"})
	seedProjectRoleAssignment(t, ctx, rbacStore, projectAssignedUser.ExternalID, projectRole.ExternalID, projectAssignedProject.ID)

	// Give another user organization-wide access. The second organization proves that the assignment
	// does not cross tenant boundaries.
	orgAssignedUser := seedUser(t, userStore, "org-assignment@test.com")
	orgAssignedOrg := seedOrg(t, orgStore, "org-assignment-first")
	orgAssignedControl := mustProjectByID(t, orgStore, orgAssignedOrg.ControlProjectID)
	orgAssignedFirstProject := seedProject(t, orgStore, orgAssignedOrg.ID, "first")
	orgAssignedSecondProject := seedProject(t, orgStore, orgAssignedOrg.ID, "second")
	seedOrg(t, orgStore, "org-assignment-second")
	seedOrgMembership(t, pool, orgAssignedUser.ID, orgAssignedOrg.ID)
	orgRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: orgAssignedOrg.ID, Name: "org role"})
	seedOrgRoleAssignment(t, ctx, rbacStore, orgAssignedUser.ExternalID, orgRole.ExternalID, orgAssignedOrg.ID)
	// Overlapping direct access must not duplicate this project in the result.
	seedProjectRoleAssignment(t, ctx, rbacStore, orgAssignedUser.ExternalID, orgRole.ExternalID, orgAssignedFirstProject.ID)

	// Superadmin carries global project discovery and reaches every project seeded by this test.
	systemAssignedUser := seedUser(t, userStore, "system-assignment@test.com")
	systemAssignedOrg := seedOrg(t, orgStore, "system-assignment-org")
	systemAssignedProject := seedProject(t, orgStore, systemAssignedOrg.ID, "project")
	seedSystemRoleAssignment(t, rbacStore, systemAssignedUser.ExternalID, "superadmin")

	// A narrow system role proves that system scope alone does not enable global discovery.
	systemAssignedWithoutDiscoveryUser := seedUser(t, userStore, "system-assignment-without-discovery@test.com")
	systemAssignedWithoutDiscoveryOrg := seedOrg(t, orgStore, "system-assignment-without-discovery-org")
	systemAssignedWithoutDiscoveryProject := seedProject(t, orgStore, systemAssignedWithoutDiscoveryOrg.ID, "project")
	seedSystemRole(t, pool, "system-role:system-read", "system-role:read")
	seedSystemRoleAssignment(t, rbacStore, systemAssignedWithoutDiscoveryUser.ExternalID, "system-role:system-read")

	tests := []struct {
		name       string
		userID     uuid.UUID
		filter     pgorg.ProjectFilter
		pageSize   int
		pageOffset int
		want       []pgorg.Project
	}{
		{
			name:     "project assignment",
			userID:   projectAssignedUser.ExternalID,
			pageSize: 10,
			want:     []pgorg.Project{projectAssignedProject},
		},
		{
			name:     "whitespace-only name filter",
			userID:   projectAssignedUser.ExternalID,
			filter:   pgorg.ProjectFilter{Name: "   "},
			pageSize: 10,
			want:     []pgorg.Project{projectAssignedProject},
		},
		{
			name:     "organization assignment",
			userID:   orgAssignedUser.ExternalID,
			pageSize: 10,
			want:     []pgorg.Project{orgAssignedControl, orgAssignedFirstProject, orgAssignedSecondProject},
		},
		{
			name:     "system assignment across organizations",
			userID:   systemAssignedUser.ExternalID,
			filter:   pgorg.ProjectFilter{Name: "first"},
			pageSize: 10,
			want:     []pgorg.Project{projectAssignedProject, orgAssignedFirstProject},
		},
		{
			name:       "pagination",
			userID:     systemAssignedUser.ExternalID,
			filter:     pgorg.ProjectFilter{Name: "first"},
			pageSize:   1,
			pageOffset: 1,
			want:       []pgorg.Project{orgAssignedFirstProject},
		},
		{
			name:     "name filter",
			userID:   systemAssignedUser.ExternalID,
			filter:   pgorg.ProjectFilter{Name: "PRO"},
			pageSize: 10,
			// The filter is case-insensitive and both names match, so organization ID determines
			// their order.
			want: []pgorg.Project{systemAssignedProject, systemAssignedWithoutDiscoveryProject},
		},
		{
			name:     "system assignment without global discovery",
			userID:   systemAssignedWithoutDiscoveryUser.ExternalID,
			pageSize: 10,
			want:     []pgorg.Project{},
		},
		{
			name:     "empty",
			userID:   uuid.New(),
			pageSize: 10,
			// The list query deliberately cannot distinguish a missing user from an existing user
			// with no assignments; the companion count query performs that validation.
			want: []pgorg.Project{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := orgStore.AccessibleProjects(ctx, tt.userID, tt.filter, tt.pageSize, tt.pageOffset)
			if err != nil {
				t.Fatalf("AccessibleProjects(%s, %+v, %d, %d) error = %v", tt.userID, tt.filter, tt.pageSize, tt.pageOffset, err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestStore_AccessibleProjectCount(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)
	userStore := pguser.NewStore(pool)

	// Organization-wide access should reach exactly the control project and this ordinary project.
	org := seedOrg(t, orgStore, "count-org")
	project := seedProject(t, orgStore, org.ID, "project")
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "count role"})

	// This second organization proves organization assignments stay within their tenant, while
	// global discovery reaches matching projects across organizations.
	secondOrg := seedOrg(t, orgStore, "count-second-org")
	seedProject(t, orgStore, secondOrg.ID, "project")

	// This user proves that existence alone contributes no projects.
	unassignedUser := seedUser(t, userStore, "count-empty@test.com")

	// Give one user direct access to only the ordinary project.
	projectAssignedUser := seedUser(t, userStore, "count-project-assigned@test.com")
	seedOrgMembership(t, pool, projectAssignedUser.ID, org.ID)
	seedProjectRoleAssignment(t, ctx, rbacStore, projectAssignedUser.ExternalID, role.ExternalID, project.ID)

	// Give one user overlapping direct and organization-wide access to the ordinary project.
	orgAssignedUser := seedUser(t, userStore, "count-org-assigned@test.com")
	seedOrgMembership(t, pool, orgAssignedUser.ID, org.ID)
	seedProjectRoleAssignment(t, ctx, rbacStore, orgAssignedUser.ExternalID, role.ExternalID, project.ID)
	seedOrgRoleAssignment(t, ctx, rbacStore, orgAssignedUser.ExternalID, role.ExternalID, org.ID)

	// Superadmin carries global project discovery and therefore reaches both organizations.
	systemAssignedUser := seedUser(t, userStore, "count-system-assigned@test.com")
	seedSystemRoleAssignment(t, rbacStore, systemAssignedUser.ExternalID, "superadmin")

	// A narrow system role proves that system scope alone does not enable global discovery.
	systemAssignedWithoutDiscoveryUser := seedUser(t, userStore, "count-system-assigned-without-discovery@test.com")
	seedSystemRole(t, pool, "system-role:system-read", "system-role:read")
	seedSystemRoleAssignment(t, rbacStore, systemAssignedWithoutDiscoveryUser.ExternalID, "system-role:system-read")

	tests := []struct {
		name   string
		userID uuid.UUID
		filter pgorg.ProjectFilter
		want   int
	}{
		{
			name:   "no assignments",
			userID: unassignedUser.ExternalID,
			want:   0,
		},
		{
			name:   "project assignment",
			userID: projectAssignedUser.ExternalID,
			want:   1,
		},
		{
			name:   "whitespace-only name filter",
			userID: projectAssignedUser.ExternalID,
			filter: pgorg.ProjectFilter{Name: "   "},
			want:   1,
		},
		{
			name:   "organization assignment deduplicates scopes",
			userID: orgAssignedUser.ExternalID,
			// The organization assignment reaches the control project and project. The direct
			// assignment also reaches project, but that overlap must not increase the count beyond two.
			want: 2,
		},
		{
			name:   "system assignment across organizations",
			userID: systemAssignedUser.ExternalID,
			filter: pgorg.ProjectFilter{Name: "PRO"},
			want:   2,
		},
		{
			name:   "system assignment",
			userID: systemAssignedUser.ExternalID,
			want:   4,
		},
		{
			name:   "system assignment without global discovery",
			userID: systemAssignedWithoutDiscoveryUser.ExternalID,
			want:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := orgStore.AccessibleProjectCount(ctx, tt.userID, tt.filter)
			if err != nil {
				t.Fatalf("AccessibleProjectCount(%s, %+v) error = %v", tt.userID, tt.filter, err)
			}

			if got != tt.want {
				t.Errorf("AccessibleProjectCount(%s, %+v) = %d, want %d", tt.userID, tt.filter, got, tt.want)
			}
		})
	}
}

func TestStore_AccessibleProjectCount_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		if _, err := orgStore.AccessibleProjectCount(ctx, uuid.New(), pgorg.ProjectFilter{}); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AccessibleProjectCount() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestProtectControlProjectTrigger(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		org := seedOrg(t, pgorg.NewStore(pool), "acme")

		_, err := pool.Exec(ctx, `DELETE FROM org.projects WHERE id = $1`, org.ControlProjectID)
		if err == nil {
			t.Fatal("DELETE control project error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "cannot be deleted")
	})

	t.Run("update is_control on a control project", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		org := seedOrg(t, pgorg.NewStore(pool), "acme")

		_, err := pool.Exec(ctx, `UPDATE org.projects SET is_control = false WHERE id = $1`, org.ControlProjectID)
		if err == nil {
			t.Fatal("UPDATE is_control error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "cannot be changed after creation")
	})

	t.Run("update is_control on an ordinary project", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		org := seedOrg(t, orgStore, "acme")

		project := seedProject(t, orgStore, org.ID, "widgets")

		_, err := pool.Exec(ctx, `UPDATE org.projects SET is_control = true WHERE id = $1`, project.ID)
		if err == nil {
			t.Fatal("UPDATE is_control error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "cannot be changed after creation")
	})

	t.Run("rename a control project", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		org := seedOrg(t, pgorg.NewStore(pool), "acme")

		if _, err := pool.Exec(ctx, `UPDATE org.projects SET name = 'renamed' WHERE id = $1`, org.ControlProjectID); err != nil {
			t.Errorf("UPDATE name error = %v, want nil", err)
		}
	})
}

func seedOrg(t *testing.T, orgStore *pgorg.Store, name string) pgorg.Organization {
	t.Helper()

	org, err := orgStore.CreateOrganization(t.Context(), pgorg.CreateOrganization{Name: name, ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("seed org %q: %v", name, err)
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

func seedUser(t *testing.T, orgStore *pguser.Store, email string) pguser.User {
	t.Helper()

	user, err := orgStore.CreateUser(t.Context(), pguser.CreateUser{Email: email, Name: "Test User"})
	if err != nil {
		t.Fatalf("seed user %q: %v", email, err)
	}

	return user
}

func seedOrgMembership(t *testing.T, pool *pgxpool.Pool, userID, orgID int) {
	t.Helper()

	if _, err := pool.Exec(t.Context(), "INSERT INTO org.org_membership (user_id, org_id) VALUES ($1, $2)", userID, orgID); err != nil {
		t.Fatalf("seed organization membership (user %d, org %d): %v", userID, orgID, err)
	}
}

func seedCustomRole(t *testing.T, orgStore *pgrbac.Store, role pgrbac.CreateCustomRole) pgrbac.CustomRole {
	t.Helper()

	created, err := orgStore.CreateCustomRole(t.Context(), role)
	if err != nil {
		t.Fatalf("seed custom role %q: %v", role.Name, err)
	}

	return created
}

func seedProjectRoleAssignment(t *testing.T, ctx context.Context, orgStor *pgrbac.Store, userID, roleID uuid.UUID, projectID int) {
	t.Helper()

	if err := orgStor.AssignCustomRoleToProject(ctx, userID, roleID, projectID); err != nil {
		t.Fatalf("seed project role assignment (user %s, role %s, project %d): %v", userID, roleID, projectID, err)
	}
}

func seedOrgRoleAssignment(t *testing.T, ctx context.Context, orgStore *pgrbac.Store, userID, roleID uuid.UUID, orgID int) {
	t.Helper()

	if err := orgStore.AssignCustomRoleToOrg(ctx, userID, roleID, orgID); err != nil {
		t.Fatalf("seed organization role assignment (user %s, role %s, org %d): %v", userID, roleID, orgID, err)
	}
}

func seedSystemRoleAssignment(t *testing.T, orgStore *pgrbac.Store, userID uuid.UUID, roleName string) {
	t.Helper()

	if err := orgStore.AssignSystemRole(t.Context(), userID, roleName); err != nil {
		t.Fatalf("seed system role assignment (user %s, role %q): %v", userID, roleName, err)
	}
}

func seedSystemRole(t *testing.T, pool *pgxpool.Pool, name, permissionName string) {
	t.Helper()

	if _, err := pool.Exec(t.Context(), `
		INSERT INTO rbac.system_roles (external_id, name, created_at)
		VALUES (gen_random_uuid(), $1, NOW())`, name); err != nil {
		t.Fatalf("seed system role %q: %v", name, err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO rbac.system_role_permissions (role_id, permission_id)
		SELECT role.id, permission.id
		FROM rbac.system_roles AS role
		JOIN rbac.permissions AS permission ON permission.name = $2
		WHERE role.name = $1`, name, permissionName); err != nil {
		t.Fatalf("seed system role %q permission %q: %v", name, permissionName, err)
	}
}

func mustProjectByID(t *testing.T, orgStore *pgorg.Store, projectID int) pgorg.Project {
	t.Helper()

	project, err := orgStore.ProjectByID(t.Context(), projectID)
	if err != nil {
		t.Fatalf("ProjectByID(%d) error = %v", projectID, err)
	}

	return project
}

func mustProjectByName(t *testing.T, orgStore *pgorg.Store, orgID int, name string) pgorg.Project {
	t.Helper()

	project, err := orgStore.ProjectByName(t.Context(), orgID, name)
	if err != nil {
		t.Fatalf("ProjectByName(%d, %s) error = %v", orgID, name, err)
	}

	return project
}
