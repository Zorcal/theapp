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

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
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
	core := NewCore(orgStore, pgdb.NewTransactor(pool))

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(mdl.Organization{}, "ID", "ControlProjectID"),
		cmpopts.IgnoreFields(mdl.Project{}, "ID", "ETag"),
		cmpopts.EquateApproxTime(time.Minute),
	}

	// Create an organization with its default and control projects.

	createdOrg, err := core.CreateOrganization(ctx, mdl.CreateOrganization{Name: "acme", ProjectName: "acme"})
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

func TestCore_CreateOrganization(t *testing.T) {
	orgStorer := &MockedOrgStorer{
		CreateOrganizationFunc: func(_ context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error) {
			return pgorg.Organization{ID: 1, Name: co.Name}, nil
		},
		CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
			return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
		},
	}
	core := NewCore(orgStorer, immediateTransactor{})

	got, err := core.CreateOrganization(t.Context(), mdl.CreateOrganization{Name: "acme", ProjectName: "acme"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	want := mdl.Organization{ID: 1, Name: "acme"}

	testingx.AssertDiff(t, got, want)
}

func TestCore_CreateOrganization_error(t *testing.T) {
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
			core := NewCore(tt.orgStorer, immediateTransactor{})

			if _, err := core.CreateOrganization(t.Context(), tt.in); !errors.Is(err, tt.want) {
				t.Errorf("CreateOrganization(%+v) error = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

func TestCore_CreateProject(t *testing.T) {
	orgStorer := &MockedOrgStorer{
		CreateProjectFunc: func(_ context.Context, cp pgorg.CreateProject) (pgorg.Project, error) {
			return pgorg.Project{ID: 1, OrgID: cp.OrgID, Name: cp.Name}, nil
		},
	}
	core := NewCore(orgStorer, immediateTransactor{})

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
			core := NewCore(tt.orgStorer, immediateTransactor{})

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
	core := NewCore(orgStorer, immediateTransactor{})

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
			}, immediateTransactor{})

			if _, err := core.OrganizationByName(t.Context(), "acme"); !errors.Is(err, tt.want) {
				t.Errorf("OrganizationByName() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_ProjectByName(t *testing.T) {
	orgStorer := &MockedOrgStorer{
		ProjectByNameFunc: func(_ context.Context, orgID int, name string) (pgorg.Project, error) {
			return pgorg.Project{ID: 1, OrgID: orgID, Name: name}, nil
		},
	}
	core := NewCore(orgStorer, immediateTransactor{})

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
			}, immediateTransactor{})

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
		AccessibleProjectsFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter, _, _ int) ([]pgorg.Project, error) {
			return []pgorg.Project{mockedProject}, nil
		},
		AccessibleProjectCountFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter) (int, error) {
			return 1, nil
		},
	}
	core := NewCore(orgStorer, immediateTransactor{})
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
				AccessibleProjectsFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter, _, _ int) ([]pgorg.Project, error) {
					return nil, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "user not found",
			orgStorer: &MockedOrgStorer{
				AccessibleProjectsFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter, _, _ int) ([]pgorg.Project, error) {
					return nil, nil
				},
				AccessibleProjectCountFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter) (int, error) {
					return 0, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "count store error",
			orgStorer: &MockedOrgStorer{
				AccessibleProjectsFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter, _, _ int) ([]pgorg.Project, error) {
					return nil, nil
				},
				AccessibleProjectCountFunc: func(_ context.Context, _ uuid.UUID, _ pgorg.ProjectFilter) (int, error) {
					return 0, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.orgStorer, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{User: mdl.AuthUser{UserID: uuid.New()}})

			if _, _, err := core.AccessibleProjects(ctx, mdl.ProjectFilter{}, 10, 0); !errors.Is(err, tt.want) {
				t.Errorf("AccessibleProjects() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("auth session missing", func(t *testing.T) {
		core := NewCore(&MockedOrgStorer{}, immediateTransactor{})

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
