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

func TestStore_AddOrganizationMember(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)

	org := seedOrg(t, orgStore, "membership-org")
	user := seedUser(t, userStore, "membership-user@test.com")

	if err := orgStore.AddOrganizationMember(ctx, user.ExternalID, org.ID); err != nil {
		t.Fatalf("AddOrganizationMember() error = %v", err)
	}

	if exists := checkOrgMembership(t, pool, user.ID, org.ID); !exists {
		t.Error("AddOrganizationMember() organization membership does not exist")
	}
}

func TestStore_AddOrganizationMember_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)

	notFoundOrg := seedOrg(t, orgStore, "membership-not-found-org")
	notFoundUser := seedUser(t, userStore, "membership-not-found-user@test.com")
	existingOrg := seedOrg(t, orgStore, "membership-already-exists-org")
	existingUser := seedUser(t, userStore, "membership-already-exists-user@test.com")
	seedOrgMembership(t, pool, existingUser.ID, existingOrg.ID)

	t.Run("not found", func(t *testing.T) {
		tests := []struct {
			name   string
			userID uuid.UUID
			orgID  int
		}{
			{
				name:   "user",
				userID: uuid.New(),
				orgID:  notFoundOrg.ID,
			},
			{
				name:   "organization",
				userID: notFoundUser.ExternalID,
				orgID:  999999,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := orgStore.AddOrganizationMember(ctx, tt.userID, tt.orgID); !errors.Is(err, sql.ErrNoRows) {
					t.Errorf("AddOrganizationMember() error = %v, want sql.ErrNoRows", err)
				}
			})
		}
	})

	t.Run("already exists", func(t *testing.T) {
		if err := orgStore.AddOrganizationMember(ctx, existingUser.ExternalID, existingOrg.ID); !errors.Is(err, pgdb.ErrAlreadyExists) {
			t.Errorf("AddOrganizationMember() error = %v, want pgdb.ErrAlreadyExists", err)
		}
	})
}

func TestStore_EnsureOrganizationMember(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)

	org := seedOrg(t, orgStore, "ensure-membership")
	user := seedUser(t, userStore, "ensure-membership@test.com")

	if err := orgStore.EnsureOrganizationMember(ctx, user.ExternalID, org.ID); err != nil {
		t.Fatalf("EnsureOrganizationMember() error = %v, want nil", err)
	}

	if !checkOrgMembership(t, pool, user.ID, org.ID) {
		t.Error("EnsureOrganizationMember() organization membership does not exist")
	}

	// Ensuring an existing membership is a no-op.

	if err := orgStore.EnsureOrganizationMember(ctx, user.ExternalID, org.ID); err != nil {
		t.Fatalf("EnsureOrganizationMember() existing membership error = %v, want nil", err)
	}
}

func TestStore_EnsureOrganizationMember_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)

	user := seedUser(t, userStore, "ensure-membership-not-found@test.com")
	org := seedOrg(t, orgStore, "ensure-membership-not-found")

	t.Run("not found", func(t *testing.T) {
		tests := []struct {
			name   string
			userID uuid.UUID
			orgID  int
		}{
			{
				name:   "user",
				userID: uuid.New(),
				orgID:  org.ID,
			},
			{
				name:   "organization",
				userID: user.ExternalID,
				orgID:  org.ID + 999,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := orgStore.EnsureOrganizationMember(ctx, tt.userID, tt.orgID); !errors.Is(err, sql.ErrNoRows) {
					t.Errorf("EnsureOrganizationMember() error = %v, want %v", err, sql.ErrNoRows)
				}
			})
		}
	})
}

func TestStore_IsOrganizationMember(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)

	org := seedOrg(t, orgStore, "organization-member-check")
	member := seedUser(t, userStore, "organization-member-check@test.com")
	nonmember := seedUser(t, userStore, "organization-nonmember-check@test.com")
	seedOrgMembership(t, pool, member.ID, org.ID)

	tests := []struct {
		name   string
		userID uuid.UUID
		orgID  int
		want   bool
	}{
		{
			name:   "member",
			userID: member.ExternalID,
			orgID:  org.ID,
			want:   true,
		},
		{
			name:   "nonmember",
			userID: nonmember.ExternalID,
			orgID:  org.ID,
			want:   false,
		},
		{
			name:   "unknown user",
			userID: uuid.New(),
			orgID:  org.ID,
			want:   false,
		},
		{
			name:   "unknown organization",
			userID: member.ExternalID,
			orgID:  org.ID + 999,
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := orgStore.IsOrganizationMember(ctx, tt.userID, tt.orgID)
			if err != nil {
				t.Fatalf("IsOrganizationMember(%s, %d) error = %v", tt.userID, tt.orgID, err)
			}

			if got != tt.want {
				t.Errorf("IsOrganizationMember(%s, %d) = %t, want %t", tt.userID, tt.orgID, got, tt.want)
			}
		})
	}
}

func TestStore_IsOrganizationControlProject(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	org := seedOrg(t, orgStore, "organization-control-project-check")
	ordinaryProject := seedProject(t, orgStore, org.ID, "ordinary")
	otherOrg := seedOrg(t, orgStore, "other-organization-control-project-check")

	tests := []struct {
		name      string
		orgID     int
		projectID int
		want      bool
	}{
		{
			name:      "control project",
			orgID:     org.ID,
			projectID: org.ControlProjectID,
			want:      true,
		},
		{
			name:      "ordinary project",
			orgID:     org.ID,
			projectID: ordinaryProject.ID,
			want:      false,
		},
		{
			name:      "different organization",
			orgID:     org.ID,
			projectID: otherOrg.ControlProjectID,
			want:      false,
		},
		{
			name:      "unknown organization",
			orgID:     org.ID + 999,
			projectID: org.ControlProjectID,
			want:      false,
		},
		{
			name:      "unknown project",
			orgID:     org.ID,
			projectID: org.ControlProjectID + 999,
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := orgStore.IsOrganizationControlProject(ctx, tt.orgID, tt.projectID)
			if err != nil {
				t.Fatalf("IsOrganizationControlProject(%d, %d) error = %v", tt.orgID, tt.projectID, err)
			}
			if got != tt.want {
				t.Errorf("IsOrganizationControlProject(%d, %d) = %t, want %t", tt.orgID, tt.projectID, got, tt.want)
			}
		})
	}
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

	// Project-scoped access reaches only the assigned project, not its siblings.
	projectAssignedUser := seedUser(t, userStore, "project-assignment@test.com")
	projectAssignedOrg := seedOrg(t, orgStore, "project-assignment-org")
	projectAssignedControl := mustProjectByID(t, orgStore, projectAssignedOrg.ControlProjectID)
	projectAssignedProject := seedProject(t, orgStore, projectAssignedOrg.ID, "shared-zulu")
	seedProject(t, orgStore, projectAssignedOrg.ID, "second")
	seedOrgMembership(t, pool, projectAssignedUser.ID, projectAssignedOrg.ID)
	projectRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: projectAssignedOrg.ID, Name: "project role"})
	seedProjectRoleAssignment(t, ctx, rbacStore, projectAssignedUser.ExternalID, projectRole.ExternalID, projectAssignedProject.ID)

	// Organization-scoped access reaches every project in its organization, remains tenant-bound,
	// and deduplicates projects also reached through a direct assignment.
	orgAssignedUser := seedUser(t, userStore, "org-assignment@test.com")
	orgAssignedOrg := seedOrg(t, orgStore, "org-assignment-first")
	orgAssignedControl := mustProjectByID(t, orgStore, orgAssignedOrg.ControlProjectID)
	orgAssignedSharedProject := seedProject(t, orgStore, orgAssignedOrg.ID, "shared-alpha")
	orgAssignedSecondProject := seedProject(t, orgStore, orgAssignedOrg.ID, "second")
	orgAssignedProject2 := seedProject(t, orgStore, orgAssignedOrg.ID, "project-2")
	orgAssignedProject10 := seedProject(t, orgStore, orgAssignedOrg.ID, "project-10")
	seedOrg(t, orgStore, "org-assignment-second")
	seedOrgMembership(t, pool, orgAssignedUser.ID, orgAssignedOrg.ID)
	orgRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: orgAssignedOrg.ID, Name: "org role"})
	seedOrgRoleAssignment(t, ctx, rbacStore, orgAssignedUser.ExternalID, orgRole.ExternalID, orgAssignedOrg.ID)
	seedProjectRoleAssignment(t, ctx, rbacStore, orgAssignedUser.ExternalID, orgRole.ExternalID, orgAssignedSharedProject.ID)

	// System-scoped global discovery reaches projects across every seeded organization.
	systemAssignedUser := seedUser(t, userStore, "system-assignment@test.com")
	systemAssignedOrg := seedOrg(t, orgStore, "system-assignment-org")
	systemAssignedProject := seedProject(t, orgStore, systemAssignedOrg.ID, "match-project")
	seedSystemRoleAssignment(t, rbacStore, systemAssignedUser.ExternalID, "superadmin")

	// A system role without project:discover-all does not make any projects accessible.
	systemAssignedWithoutDiscoveryUser := seedUser(t, userStore, "system-assignment-without-discovery@test.com")
	systemAssignedWithoutDiscoveryOrg := seedOrg(t, orgStore, "system-assignment-without-discovery-org")
	systemAssignedWithoutDiscoveryProject := seedProject(t, orgStore, systemAssignedWithoutDiscoveryOrg.ID, "match-project")
	seedSystemRole(t, pool, "system-role:system-read", "system-role:read")
	seedSystemRoleAssignment(t, rbacStore, systemAssignedWithoutDiscoveryUser.ExternalID, "system-role:system-read")

	// A user without assignments provides the empty-access baseline.
	unassignedUser := seedUser(t, userStore, "unassigned@test.com")

	tests := []struct {
		name       string
		userID     uuid.UUID
		filter     pgorg.ProjectFilter
		pageSize   int
		pageOffset int
		want       []pgorg.Project
		wantCount  int
	}{
		{
			name:      "no assignments",
			userID:    unassignedUser.ExternalID,
			pageSize:  10,
			want:      []pgorg.Project{},
			wantCount: 0,
		},
		{
			name:      "project assignment",
			userID:    projectAssignedUser.ExternalID,
			pageSize:  10,
			want:      []pgorg.Project{projectAssignedProject},
			wantCount: 1,
		},
		{
			name:     "organization assignment",
			userID:   orgAssignedUser.ExternalID,
			pageSize: 10,
			want: []pgorg.Project{
				orgAssignedControl,
				orgAssignedProject2,
				orgAssignedProject10,
				orgAssignedSecondProject,
				orgAssignedSharedProject,
			},
			wantCount: 5,
		},
		{
			name:      "system assignment without global discovery",
			userID:    systemAssignedWithoutDiscoveryUser.ExternalID,
			pageSize:  10,
			want:      []pgorg.Project{},
			wantCount: 0,
		},
		{
			name:      "system assignment",
			userID:    systemAssignedUser.ExternalID,
			pageSize:  1,
			want:      []pgorg.Project{projectAssignedControl},
			wantCount: 13,
		},
		{
			name:      "whitespace-only name filter",
			userID:    projectAssignedUser.ExternalID,
			filter:    pgorg.ProjectFilter{Name: "   "},
			pageSize:  10,
			want:      []pgorg.Project{projectAssignedProject},
			wantCount: 1,
		},
		{
			name:      "name filter",
			userID:    systemAssignedUser.ExternalID,
			filter:    pgorg.ProjectFilter{Name: "match-"},
			pageSize:  10,
			want:      []pgorg.Project{systemAssignedProject, systemAssignedWithoutDiscoveryProject},
			wantCount: 2,
		},
		{
			name:      "natural name order",
			userID:    orgAssignedUser.ExternalID,
			filter:    pgorg.ProjectFilter{Name: "project-"},
			pageSize:  10,
			want:      []pgorg.Project{orgAssignedProject2, orgAssignedProject10},
			wantCount: 2,
		},
		{
			name:      "organization order",
			userID:    systemAssignedUser.ExternalID,
			filter:    pgorg.ProjectFilter{Name: "shared-"},
			pageSize:  10,
			want:      []pgorg.Project{projectAssignedProject, orgAssignedSharedProject},
			wantCount: 2,
		},
		{
			name:       "pagination",
			userID:     systemAssignedUser.ExternalID,
			filter:     pgorg.ProjectFilter{Name: "shared-"},
			pageSize:   1,
			pageOffset: 1,
			want:       []pgorg.Project{orgAssignedSharedProject},
			wantCount:  2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count, err := orgStore.AccessibleProjects(ctx, tt.userID, tt.filter, tt.pageSize, tt.pageOffset)
			if err != nil {
				t.Fatalf("AccessibleProjects(%s, %+v, %d, %d) error = %v", tt.userID, tt.filter, tt.pageSize, tt.pageOffset, err)
			}

			testingx.AssertDiff(t, got, tt.want)

			if count != tt.wantCount {
				t.Errorf("AccessibleProjects(%s, %+v, %d, %d) total count = %d, want %d", tt.userID, tt.filter, tt.pageSize, tt.pageOffset, count, tt.wantCount)
			}
		})
	}
}

func TestStore_AccessibleProjects_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		if _, _, err := orgStore.AccessibleProjects(ctx, uuid.New(), pgorg.ProjectFilter{}, 10, 0); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AccessibleProjects() error = %v, want sql.ErrNoRows", err)
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

// TODO: Exercise the membership-removal store operation here once it exists; until then these
// direct deletes prove that callers cannot leave role assignments dangling.
func TestOrgMembershipDeletionWithRoleAssignments(t *testing.T) {
	t.Run("project role assignment", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "project-assignment-membership-org")
		project := seedProject(t, orgStore, org.ID, "project")
		user := seedUser(t, userStore, "project-assignment-membership@test.com")
		seedOrgMembership(t, pool, user.ID, org.ID)
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "project-role"})
		seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, role.ExternalID, project.ID)

		if _, err := pool.Exec(
			ctx,
			"DELETE FROM org.org_membership WHERE user_id = $1 AND org_id = $2",
			user.ID,
			org.ID,
		); err == nil {
			t.Error("membership deletion succeeded with a project role assignment, want foreign key error")
		}
	})

	t.Run("organization role assignment", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "org-assignment-membership-org")
		user := seedUser(t, userStore, "org-assignment-membership@test.com")
		seedOrgMembership(t, pool, user.ID, org.ID)
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "org-role"})
		seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, role.ExternalID, org.ID)

		if _, err := pool.Exec(
			ctx,
			"DELETE FROM org.org_membership WHERE user_id = $1 AND org_id = $2",
			user.ID,
			org.ID,
		); err == nil {
			t.Error("membership deletion succeeded with an organization role assignment, want foreign key error")
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

func checkOrgMembership(t *testing.T, pool *pgxpool.Pool, userID, orgID int) bool {
	t.Helper()

	var exists bool
	if err := pool.QueryRow(
		t.Context(),
		"SELECT EXISTS (SELECT FROM org.org_membership WHERE user_id = $1 AND org_id = $2)",
		userID,
		orgID,
	).Scan(&exists); err != nil {
		t.Fatalf("check organization membership (user %d, org %d): %v", userID, orgID, err)
	}

	return exists
}

func mustProjectByName(t *testing.T, orgStore *pgorg.Store, orgID int, name string) pgorg.Project {
	t.Helper()

	project, err := orgStore.ProjectByName(t.Context(), orgID, name)
	if err != nil {
		t.Fatalf("ProjectByName(%d, %s) error = %v", orgID, name, err)
	}

	return project
}
