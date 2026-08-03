package org

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgschema"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

// TestCore_integration_organizationLifecycle exercises organization and project creation, lookup,
// and accessible-project discovery through the core and PostgreSQL store layers.
func TestCore_integration_organizationLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)
	userStore := pguser.NewStore(pool)
	core := NewCore(orgStore, userStore, rbacStore, pgdb.NewTransactor(pool))

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(mdl.Organization{}, "ID", "ControlProjectID"),
		cmpopts.IgnoreFields(mdl.Project{}, "ID", "ETag"),
		cmpopts.IgnoreFields(mdl.User{}, "ID", "CreatedAt", "ETag"),
		cmpopts.IgnoreFields(pgrbac.CustomRole{}, "ID", "ExternalID", "CreatedAt", "UpdatedAt", "ETag"),
		cmpopts.EquateApproxTime(time.Minute),
		cmpopts.SortSlices(func(a, b string) bool { return a < b }),
	}

	// Create an organization with its default and control projects.

	creator := seedUser(t, userStore, "organization-creator@test.com", "Organization Creator")

	createCtx := mdl.ContextWithAuthSession(ctx, mdl.AuthSession{User: mdl.AuthUser{UserID: creator.ExternalID}})
	createdOrg, err := core.CreateOrganization(createCtx, mdl.CreateOrganization{Name: "acme", ProjectName: "acme"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	wantCreatedOrg := mdl.Organization{
		Name:      "acme",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(t, createdOrg, wantCreatedOrg, diffOpts...)

	if createdOrg.ID == 0 {
		t.Error("CreateOrganization() ID = 0, want non-zero")
	}
	if createdOrg.ControlProjectID == 0 {
		t.Error("CreateOrganization() ControlProjectID = 0, want non-zero")
	}

	// List the managed role assigned to the organization creator.

	creatorRoles, creatorRoleCount, err := rbacStore.UserOrgCustomRoles(ctx, creator.ExternalID, createdOrg.ID, 50, 0)
	if err != nil {
		t.Fatalf("UserOrgCustomRoles() error = %v", err)
	}
	if wantCount := 1; creatorRoleCount != wantCount {
		t.Errorf("UserOrgCustomRoles() total count = %d, want %d", creatorRoleCount, wantCount)
	}

	testingx.AssertDiff(t, creatorRoles, []pgrbac.CustomRole{{
		Name:            "Organization Administrator",
		ManagedKey:      new(mdl.ManagedRoleKeyOrganizationAdmin),
		PermissionNames: permissionsToPg(mdl.OrganizationAdminPermissions()),
	}}, diffOpts...)

	// Create an organization user and repeat the operation without duplicating the user or membership.

	orgCtx := mdl.ContextWithAuthSession(ctx, mdl.AuthSession{User: mdl.AuthUser{UserID: creator.ExternalID}, Project: &mdl.AuthProject{OrgID: createdOrg.ID}})

	createdOrgUser, err := core.CreateOrganizationUser(orgCtx, mdl.CreateOrganizationUser{
		Email: "organization-user@test.com",
	})
	if err != nil {
		t.Fatalf("CreateOrganizationUser() error = %v", err)
	}

	testingx.AssertDiff(t, createdOrgUser, mdl.User{Email: "organization-user@test.com"}, diffOpts...)

	if !checkOrgMembership(t, pool, createdOrgUser.ID, createdOrg.ID) {
		t.Error("organization membership = false, want true")
	}

	// Creating the user with the organization again is a no-op.
	existingOrgUser, err := core.CreateOrganizationUser(orgCtx, mdl.CreateOrganizationUser{
		Email: createdOrgUser.Email,
	})
	if err != nil {
		t.Fatalf("CreateOrganizationUser() repeated error = %v", err)
	}

	testingx.AssertDiff(t, existingOrgUser, createdOrgUser)

	// Create another project in the organization.

	createdProject, err := core.CreateProject(ctx, mdl.CreateProject{OrgID: createdOrg.ID, Name: "widgets"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	wantCreatedProject := mdl.Project{
		OrgID:     createdOrg.ID,
		Name:      "widgets",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(t, createdProject, wantCreatedProject, diffOpts...)

	if createdProject.ID == 0 {
		t.Error("CreateProject() ID = 0, want non-zero")
	}
	if createdProject.ETag == uuid.Nil {
		t.Error("CreateProject() ETag is nil, want non-nil")
	}

	// Fetch the organization by name.

	fetchedOrg, err := core.OrganizationByName(ctx, "acme")
	if err != nil {
		t.Fatalf("OrganizationByName() error = %v", err)
	}

	testingx.AssertDiff(t, fetchedOrg, createdOrg)

	// Fetch the default project created with the organization.

	fetchedDefaultProject, err := core.ProjectByName(ctx, createdOrg.ID, "acme")
	if err != nil {
		t.Fatalf("ProjectByName(%d, %q) error = %v", createdOrg.ID, "acme", err)
	}

	wantDefaultProject := mdl.Project{
		OrgID:     createdOrg.ID,
		Name:      "acme",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(t, fetchedDefaultProject, wantDefaultProject, diffOpts...)

	if fetchedDefaultProject.ID == 0 {
		t.Errorf("ProjectByName(%d, %q) ID = 0, want non-zero", createdOrg.ID, "acme")
	}
	if fetchedDefaultProject.ETag == uuid.Nil {
		t.Errorf("ProjectByName(%d, %q) ETag is nil, want non-nil", createdOrg.ID, "acme")
	}

	// Grant global discovery and list accessible projects with a normalized name filter.

	discoveringUser := seedUser(t, userStore, "project-list@test.com", "Project List")
	seedSystemRoleAssignment(t, rbacStore, discoveringUser.ExternalID, "superadmin")

	discoveryCtx := mdl.ContextWithAuthSession(ctx, mdl.AuthSession{User: mdl.AuthUser{UserID: discoveringUser.ExternalID}})
	accessibleProjects, accessibleProjectCount, err := core.AccessibleProjects(discoveryCtx, mdl.ProjectFilter{Name: " WID "}, 10, 0)
	if err != nil {
		t.Fatalf("AccessibleProjects() error = %v", err)
	}

	testingx.AssertDiff(t, accessibleProjects, []mdl.Project{createdProject})

	if got, want := accessibleProjectCount, 1; got != want {
		t.Errorf("AccessibleProjects() total size = %d, want %d", got, want)
	}
}

func TestCore_integration_organizationAdminSeedSynchronization(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)
	userStore := pguser.NewStore(pool)
	core := NewCore(orgStore, userStore, rbacStore, pgdb.NewTransactor(pool))

	// Create an organization with its managed administrator role.

	creator := seedUser(t, userStore, "managed-role-sync@test.com", "Managed Role Sync")

	createCtx := mdl.ContextWithAuthSession(ctx, mdl.AuthSession{User: mdl.AuthUser{UserID: creator.ExternalID}})
	createdOrg, err := core.CreateOrganization(createCtx, mdl.CreateOrganization{
		Name:        "managed-role-sync",
		ProjectName: "managed-role-sync",
	})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	roles, _, err := rbacStore.UserOrgCustomRoles(ctx, creator.ExternalID, createdOrg.ID, 50, 0)
	if err != nil {
		t.Fatalf("UserOrgCustomRoles() error = %v", err)
	}
	if wantCount := 1; len(roles) != wantCount {
		t.Fatalf("UserOrgCustomRoles() count = %d, want %d", len(roles), wantCount)
	}

	// Replace its canonical permissions with a stale permission set.

	staleRole := mustUpdateCustomRole(t, rbacStore, pgrbac.UpdateCustomRole{
		OrgID:           createdOrg.ID,
		ExternalID:      roles[0].ExternalID,
		Fields:          pgrbac.CustomRoleUpdateFields{PermissionNames: true},
		PermissionNames: []string{"custom-role:read", "org:create"},
	})

	// Reapply seed data and verify that it restores the canonical permission set.

	if err := pgschema.Seed(ctx, pool); err != nil {
		t.Fatalf("Seed() synchronization error = %v", err)
	}

	gotSynchronizedRole, err := rbacStore.CustomRoleByExternalID(ctx, createdOrg.ID, staleRole.ExternalID)
	if err != nil {
		t.Fatalf("CustomRoleByExternalID() error = %v", err)
	}

	wantSynchronizedRole := permissionsToPg(mdl.OrganizationAdminPermissions())

	testingx.AssertDiff(t, gotSynchronizedRole.PermissionNames, wantSynchronizedRole, cmp.Options{
		cmpopts.SortSlices(func(a, b string) bool { return a < b }),
	})
}

func TestCore_CreateOrganization(t *testing.T) {
	creatorID := uuid.New()
	orgStorer := &MockedOrgStorer{
		CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
			return pgorg.Organization{ID: 1, Name: co.Name}, nil
		},
		CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
			return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
		},
		AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
			return nil
		},
	}
	roleBootstrapper := &MockedRoleBootstrapperStore{
		CreateOrganizationAdminRoleFunc: func(_ context.Context, _ int, _ []string) (pgrbac.CustomRole, error) {
			return pgrbac.CustomRole{ExternalID: uuid.New()}, nil
		},
		AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
			return nil
		},
	}
	core := NewCore(orgStorer, nil, roleBootstrapper, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User: mdl.AuthUser{UserID: creatorID},
	})

	got, err := core.CreateOrganization(ctx, mdl.CreateOrganization{Name: "acme", ProjectName: "acme"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	want := mdl.Organization{ID: 1, Name: "acme"}

	testingx.AssertDiff(t, got, want)
}

func TestCore_BootstrapOrganization_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name      string
		in        mdl.CreateOrganization
		orgStorer *MockedOrgStorer
		want      error
	}{
		{
			name:      "invalid input",
			in:        mdl.CreateOrganization{},
			orgStorer: &MockedOrgStorer{},
			want:      mdl.ErrValidation,
		},
		{
			name: "already exists",
			in:   mdl.CreateOrganization{Name: "acme", ProjectName: "acme"},
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, _ pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{}, pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "project name conflicts with control project",
			in:   mdl.CreateOrganization{Name: "acme", ProjectName: "control"},
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, _ pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{}, pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrControlProjectNameConflict,
		},
		{
			name: "store error",
			in:   mdl.CreateOrganization{Name: "acme", ProjectName: "acme"},
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, _ pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.orgStorer, nil, nil, immediateTransactor{})

			if _, err := core.BootstrapOrganization(t.Context(), tt.in); !errors.Is(err, tt.want) {
				t.Errorf("BootstrapOrganization(%+v) error = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

func TestCore_CreateOrganization_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name                  string
		orgStorer             *MockedOrgStorer
		roleBootstrapperStore *MockedRoleBootstrapperStore
		want                  error
	}{
		{
			name: "default project organization not found",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, _ pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{}, sql.ErrNoRows
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{},
			// A newly created organization cannot disappear within the transaction, so
			// sql.ErrNoRows must remain an internal error.
			want: sql.ErrNoRows,
		},
		{
			name: "creator not found",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
				},
				AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return sql.ErrNoRows
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{},
			want:                  mdl.ErrNotFound,
		},
		{
			name: "creator already a member",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
				},
				AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return pgdb.ErrAlreadyExists
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{},
			// A fresh organization cannot already contain its creator membership, so
			// pgdb.ErrAlreadyExists must remain an internal error.
			want: pgdb.ErrAlreadyExists,
		},
		{
			name: "add creator as member store error",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
				},
				AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return dbErr
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{},
			want:                  dbErr,
		},
		{
			name: "administrator role dependency not found",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
				},
				AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return nil
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{
				CreateOrganizationAdminRoleFunc: func(_ context.Context, _ int, _ []string) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			// The new organization and canonical permissions must exist at this point, so
			// sql.ErrNoRows must remain an internal error.
			want: sql.ErrNoRows,
		},
		{
			name: "administrator role already exists",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
				},
				AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return nil
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{
				CreateOrganizationAdminRoleFunc: func(_ context.Context, _ int, _ []string) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, pgdb.ErrAlreadyExists
				},
			},
			// A fresh organization cannot already contain its managed role, so
			// pgdb.ErrAlreadyExists must remain an internal error.
			want: pgdb.ErrAlreadyExists,
		},
		{
			name: "create administrator role store error",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
				},
				AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return nil
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{
				CreateOrganizationAdminRoleFunc: func(_ context.Context, _ int, _ []string) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "administrator assignment dependency not found",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
				},
				AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return nil
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{
				CreateOrganizationAdminRoleFunc: func(_ context.Context, _ int, _ []string) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{ExternalID: uuid.New()}, nil
				},
				AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return sql.ErrNoRows
				},
			},
			// The organization, creator membership, and managed role were established earlier in
			// the transaction, so sql.ErrNoRows must remain an internal error.
			want: sql.ErrNoRows,
		},
		{
			name: "administrator assignment already exists",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
				},
				AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return nil
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{
				CreateOrganizationAdminRoleFunc: func(_ context.Context, _ int, _ []string) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{ExternalID: uuid.New()}, nil
				},
				AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return pgdb.ErrAlreadyExists
				},
			},
			// A newly created managed role cannot already be assigned to its creator, so
			// pgdb.ErrAlreadyExists must remain an internal error.
			want: pgdb.ErrAlreadyExists,
		},
		{
			name: "assign administrator role store error",
			orgStorer: &MockedOrgStorer{
				CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
					return pgorg.Organization{ID: 1, Name: co.Name}, nil
				},
				CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
				},
				AddOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return nil
				},
			},
			roleBootstrapperStore: &MockedRoleBootstrapperStore{
				CreateOrganizationAdminRoleFunc: func(_ context.Context, _ int, _ []string) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{ExternalID: uuid.New()}, nil
				},
				AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
				User: mdl.AuthUser{UserID: uuid.New()},
			})
			core := NewCore(tt.orgStorer, nil, tt.roleBootstrapperStore, immediateTransactor{})

			if _, err := core.CreateOrganization(ctx, mdl.CreateOrganization{Name: "acme", ProjectName: "acme"}); !errors.Is(err, tt.want) {
				t.Errorf("CreateOrganization() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		core := NewCore(&MockedOrgStorer{}, nil, &MockedRoleBootstrapperStore{}, immediateTransactor{})

		if _, err := core.CreateOrganization(t.Context(), mdl.CreateOrganization{Name: "acme", ProjectName: "acme"}); err == nil {
			t.Error("CreateOrganization() error = nil, want error")
		}
	})
}

func TestCore_CreateProject(t *testing.T) {
	orgStorer := &MockedOrgStorer{
		CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
			return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
		},
	}
	core := NewCore(orgStorer, nil, nil, immediateTransactor{})

	got, err := core.CreateProject(t.Context(), mdl.CreateProject{OrgID: 7, Name: "widgets"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	want := mdl.Project{ID: 1, OrgID: 7, Name: "widgets"}

	testingx.AssertDiff(t, got, want)
}

func TestCore_CreateProject_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name      string
		in        mdl.CreateProject
		orgStorer *MockedOrgStorer
		want      error
	}{
		{
			name:      "invalid input",
			in:        mdl.CreateProject{},
			orgStorer: nil,
			want:      mdl.ErrValidation,
		},
		{
			name: "org not found",
			in:   mdl.CreateProject{OrgID: 7, Name: "widgets"},
			orgStorer: &MockedOrgStorer{
				CreateProjectFunc: func(_ context.Context, _ pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "already exists",
			in:   mdl.CreateProject{OrgID: 7, Name: "widgets"},
			orgStorer: &MockedOrgStorer{
				CreateProjectFunc: func(_ context.Context, _ pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{}, pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "store error",
			in:   mdl.CreateProject{OrgID: 7, Name: "widgets"},
			orgStorer: &MockedOrgStorer{
				CreateProjectFunc: func(_ context.Context, _ pgorg.CreateProject) (pgorg.Project, error) {
					return pgorg.Project{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.orgStorer, nil, nil, immediateTransactor{})

			if _, err := core.CreateProject(t.Context(), tt.in); !errors.Is(err, tt.want) {
				t.Errorf("CreateProject(%+v) error = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

func TestCore_OrganizationByName(t *testing.T) {
	orgStorer := &MockedOrgStorer{
		OrganizationByNameFunc: func(_ context.Context, name string) (pgorg.Organization, error) {
			return pgorg.Organization{ID: 1, Name: name}, nil
		},
	}
	core := NewCore(orgStorer, nil, nil, immediateTransactor{})

	got, err := core.OrganizationByName(t.Context(), "acme")
	if err != nil {
		t.Fatalf("OrganizationByName() error = %v", err)
	}

	want := mdl.Organization{ID: 1, Name: "acme"}

	testingx.AssertDiff(t, got, want)
}

func TestCore_OrganizationByName_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name    string
		mockErr error
		want    error
	}{
		{
			name:    "not found",
			mockErr: sql.ErrNoRows,
			want:    mdl.ErrNotFound,
		},
		{
			name:    "store error",
			mockErr: dbErr,
			want:    dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(&MockedOrgStorer{
				OrganizationByNameFunc: func(_ context.Context, _ string) (pgorg.Organization, error) {
					return pgorg.Organization{}, tt.mockErr
				},
			}, nil, nil, immediateTransactor{})

			if _, err := core.OrganizationByName(t.Context(), "acme"); !errors.Is(err, tt.want) {
				t.Errorf("OrganizationByName() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_CreateOrganizationUser(t *testing.T) {
	now := time.Now()
	userID := uuid.New()
	want := mdl.User{ID: userID, Email: "member@test.com", CreatedAt: now, ETag: uuid.NewString()}
	pgUser := pguser.User{
		ID:         1,
		ExternalID: want.ID,
		Email:      want.Email,
		CreatedAt:  want.CreatedAt,
		ETag:       uuid.MustParse(want.ETag),
	}

	orgStorer := &MockedOrgStorer{
		EnsureOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
			return nil
		},
	}
	orgUserStore := &MockedOrganizationUserStore{
		GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
			return pgUser, nil
		},
	}
	core := NewCore(orgStorer, orgUserStore, nil, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{Project: &mdl.AuthProject{OrgID: 1}})

	got, err := core.CreateOrganizationUser(ctx, mdl.CreateOrganizationUser{Email: want.Email})
	if err != nil {
		t.Fatalf("CreateOrganizationUser() error = %v, want nil", err)
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_CreateOrganizationUser_error(t *testing.T) {
	dbErr := errors.New("db error")
	pgUser := pguser.User{ExternalID: uuid.New()}

	tests := []struct {
		name         string
		in           mdl.CreateOrganizationUser
		orgStorer    OrgStorer
		orgUserStore OrganizationUserStore
		want         error
	}{
		{
			name:         "validation",
			in:           mdl.CreateOrganizationUser{},
			orgStorer:    &MockedOrgStorer{},
			orgUserStore: &MockedOrganizationUserStore{},
			want:         mdl.ErrValidation,
		},
		{
			name:      "user store",
			in:        mdl.CreateOrganizationUser{Email: "member@test.com"},
			orgStorer: &MockedOrgStorer{},
			orgUserStore: &MockedOrganizationUserStore{
				GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
					return pguser.User{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "membership store",
			in:   mdl.CreateOrganizationUser{Email: "member@test.com"},
			orgStorer: &MockedOrgStorer{
				EnsureOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return dbErr
				},
			},
			orgUserStore: &MockedOrganizationUserStore{
				GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
					return pgUser, nil
				},
			},
			want: dbErr,
		},
		{
			name: "missing membership dependency",
			in:   mdl.CreateOrganizationUser{Email: "member@test.com"},
			orgStorer: &MockedOrgStorer{
				EnsureOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) error {
					return sql.ErrNoRows
				},
			},
			orgUserStore: &MockedOrganizationUserStore{
				GetOrCreateUserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
					return pgUser, nil
				},
			},
			// The user and organization should have been established earlier, so this known SQL
			// error deliberately remains internal.
			want: sql.ErrNoRows,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.orgStorer, tt.orgUserStore, nil, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{Project: &mdl.AuthProject{OrgID: 1}})

			if _, err := core.CreateOrganizationUser(ctx, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("CreateOrganizationUser() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		core := NewCore(&MockedOrgStorer{}, &MockedOrganizationUserStore{}, nil, immediateTransactor{})

		if _, err := core.CreateOrganizationUser(t.Context(), mdl.CreateOrganizationUser{Email: "member@test.com"}); err == nil {
			t.Error("CreateOrganizationUser() error = nil, want error")
		}
	})
}

func TestCore_ProjectByName(t *testing.T) {
	orgStorer := &MockedOrgStorer{
		ProjectByNameFunc: func(_ context.Context, orgID int, name string) (pgorg.Project, error) {
			return pgorg.Project{ID: 1, OrgID: orgID, Name: name}, nil
		},
	}
	core := NewCore(orgStorer, nil, nil, immediateTransactor{})

	got, err := core.ProjectByName(t.Context(), 7, "control")
	if err != nil {
		t.Fatalf("ProjectByName() error = %v", err)
	}

	want := mdl.Project{ID: 1, OrgID: 7, Name: "control"}

	testingx.AssertDiff(t, got, want)
}

func TestCore_ProjectByName_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name    string
		mockErr error
		want    error
	}{
		{
			name:    "not found",
			mockErr: sql.ErrNoRows,
			want:    mdl.ErrNotFound,
		},
		{
			name:    "store error",
			mockErr: dbErr,
			want:    dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(&MockedOrgStorer{
				ProjectByNameFunc: func(_ context.Context, _ int, _ string) (pgorg.Project, error) {
					return pgorg.Project{}, tt.mockErr
				},
			}, nil, nil, immediateTransactor{})

			if _, err := core.ProjectByName(t.Context(), 7, "control"); !errors.Is(err, tt.want) {
				t.Errorf("ProjectByName() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_AccessibleProjects(t *testing.T) {
	mockedProject := pgorg.Project{
		ID:        1,
		OrgID:     2,
		Name:      "control",
		IsControl: true,
		CreatedAt: time.Now(),
		UpdatedAt: new(time.Now()),
		ETag:      uuid.New(),
	}
	filter := mdl.ProjectFilter{Name: "con"}
	orgStorer := &MockedOrgStorer{
		AccessibleProjectsFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter, _, _ int) ([]pgorg.Project, int, error) {
			return []pgorg.Project{mockedProject}, 1, nil
		},
	}
	core := NewCore(orgStorer, nil, nil, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{User: mdl.AuthUser{UserID: uuid.New()}})

	got, totalSize, err := core.AccessibleProjects(ctx, filter, 10, 0)
	if err != nil {
		t.Fatalf("AccessibleProjects() error = %v", err)
	}

	want := []mdl.Project{
		{
			ID:        mockedProject.ID,
			OrgID:     mockedProject.OrgID,
			Name:      mockedProject.Name,
			IsControl: mockedProject.IsControl,
			CreatedAt: mockedProject.CreatedAt,
			UpdatedAt: mockedProject.UpdatedAt,
			ETag:      mockedProject.ETag,
		},
	}

	testingx.AssertDiff(t, got, want)

	if got, want := totalSize, 1; got != want {
		t.Errorf("AccessibleProjects() total size = %d, want %d", got, want)
	}
}

func TestCore_AccessibleProjects_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name      string
		orgStorer *MockedOrgStorer
		want      error
	}{
		{
			name: "list store error",
			orgStorer: &MockedOrgStorer{
				AccessibleProjectsFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter, _, _ int) ([]pgorg.Project, int, error) {
					return nil, 0, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "user not found",
			orgStorer: &MockedOrgStorer{
				AccessibleProjectsFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter, _, _ int) ([]pgorg.Project, int, error) {
					return nil, 0, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.orgStorer, nil, nil, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{User: mdl.AuthUser{UserID: uuid.New()}})

			if _, _, err := core.AccessibleProjects(ctx, mdl.ProjectFilter{}, 10, 0); !errors.Is(err, tt.want) {
				t.Errorf("AccessibleProjects() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("auth session missing", func(t *testing.T) {
		core := NewCore(&MockedOrgStorer{}, nil, nil, immediateTransactor{})

		if _, _, err := core.AccessibleProjects(t.Context(), mdl.ProjectFilter{}, 10, 0); err == nil {
			t.Error("AccessibleProjects() error = nil, want error")
		}
	})
}

type immediateTransactor struct{}

func (immediateTransactor) RunTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func seedUser(t *testing.T, userStore *pguser.Store, email, name string) pguser.User {
	t.Helper()

	user, err := userStore.CreateUser(t.Context(), pguser.CreateUser{Email: email, Name: name})
	if err != nil {
		t.Fatalf("seed user %q: %v", email, err)
	}

	return user
}

func seedSystemRoleAssignment(t *testing.T, rbacStore *pgrbac.Store, userID uuid.UUID, roleName string) {
	t.Helper()

	if err := rbacStore.AssignSystemRole(t.Context(), userID, roleName); err != nil {
		t.Fatalf("seed system role assignment (user %s, role %q): %v", userID, roleName, err)
	}
}

func mustUpdateCustomRole(t *testing.T, rbacStore *pgrbac.Store, update pgrbac.UpdateCustomRole) pgrbac.CustomRole {
	t.Helper()

	role, err := rbacStore.UpdateCustomRole(t.Context(), update)
	if err != nil {
		t.Fatalf("seed custom role permissions: %v", err)
	}

	return role
}

func checkOrgMembership(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID, orgID int) bool {
	t.Helper()

	var isMember bool
	if err := pool.QueryRow(
		t.Context(),
		`SELECT EXISTS (
			SELECT
			FROM org.org_membership AS membership
			JOIN useraccess.users AS usr ON usr.id = membership.user_id
			WHERE usr.external_id = $1 AND membership.org_id = $2
		)`,
		userID,
		orgID,
	).Scan(&isMember); err != nil {
		t.Fatalf("check organization membership: %v", err)
	}

	return isMember
}
