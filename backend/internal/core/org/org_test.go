package org

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestCore_integration(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	core := NewCore(pgorg.NewStore(pool), pgdb.NewTransactor(pool))

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(mdl.Organization{}, "ID", "ControlProjectID"),
		cmpopts.IgnoreFields(mdl.Project{}, "ID"),
		cmpopts.EquateApproxTime(time.Minute),
	}

	org, err := core.CreateOrganization(ctx, mdl.CreateOrganization{Name: "acme", ProjectName: "acme"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	wantOrg := mdl.Organization{
		Name:      "acme",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(t, org, wantOrg, diffOpts...)

	if org.ID == 0 {
		t.Error("CreateOrganization() ID = 0, want non-zero")
	}
	if org.ControlProjectID == 0 {
		t.Error("CreateOrganization() ControlProjectID = 0, want non-zero")
	}

	control, err := core.ProjectByName(ctx, org.ID, "control")
	if err != nil {
		t.Fatalf("ProjectByName(%d, %q) error = %v, want the control project to have been created alongside the organization", org.ID, "control", err)
	}

	wantControl := mdl.Project{
		OrgID:     org.ID,
		Name:      "control",
		IsControl: true,
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(t, control, wantControl, diffOpts...)

	if control.ID != org.ControlProjectID {
		t.Errorf("control project ID = %d, want %d (Organization.ControlProjectID)", control.ID, org.ControlProjectID)
	}

	if _, err := core.CreateOrganization(ctx, mdl.CreateOrganization{Name: "widgets-inc", ProjectName: "control"}); !errors.Is(err, mdl.ErrControlProjectNameConflict) {
		t.Errorf("CreateOrganization() error = %v, want mdl.ErrControlProjectNameConflict", err)
	}

	// Creating an organization seeds a default project of the same name, so this collides.
	if _, err := core.CreateProject(ctx, mdl.CreateProject{OrgID: org.ID, Name: "acme"}); !errors.Is(err, mdl.ErrAlreadyExists) {
		t.Errorf("CreateProject() error = %v, want mdl.ErrAlreadyExists", err)
	}

	gotProject, err := core.CreateProject(ctx, mdl.CreateProject{OrgID: org.ID, Name: "widgets"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	wantProject := mdl.Project{
		OrgID:     org.ID,
		Name:      "widgets",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(t, gotProject, wantProject, diffOpts...)

	if gotProject.ID == 0 {
		t.Error("CreateProject() ID = 0, want non-zero")
	}

	orgByName, err := core.OrganizationByName(ctx, "acme")
	if err != nil {
		t.Fatalf("OrganizationByName() error = %v", err)
	}

	testingx.AssertDiff(t, orgByName, org)

	projectByName, err := core.ProjectByName(ctx, org.ID, "widgets")
	if err != nil {
		t.Fatalf("ProjectByName() error = %v", err)
	}

	testingx.AssertDiff(t, projectByName, gotProject)
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
	core := NewCore(orgStorer, noopTransactor{})

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
			core := NewCore(tt.orgStorer, noopTransactor{})

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
	core := NewCore(orgStorer, noopTransactor{})

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
			core := NewCore(tt.orgStorer, noopTransactor{})

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
	core := NewCore(orgStorer, noopTransactor{})

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
			}, noopTransactor{})

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
	core := NewCore(orgStorer, noopTransactor{})

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
			}, noopTransactor{})

			if _, err := core.ProjectByName(t.Context(), 7, "control"); !errors.Is(err, tt.want) {
				t.Errorf("ProjectByName() error = %v, want %v", err, tt.want)
			}
		})
	}
}

type noopTransactor struct{}

func (noopTransactor) RunTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
