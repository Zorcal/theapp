package org

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

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

	org, err := core.CreateOrganization(ctx, mdl.CreateOrganization{Name: "acme", ProjectName: "acme"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	wantOrg := mdl.Organization{
		Name:      "acme",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(
		t, org, wantOrg,
		cmpopts.IgnoreFields(mdl.Organization{}, "ID"),
		cmpopts.EquateApproxTime(time.Minute),
	)

	if org.ID == 0 {
		t.Error("CreateOrganization() ID = 0, want non-zero")
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

	testingx.AssertDiff(
		t, gotProject, wantProject,
		cmpopts.IgnoreFields(mdl.Project{}, "ID"),
		cmpopts.EquateApproxTime(time.Minute),
	)

	if gotProject.ID == 0 {
		t.Error("CreateProject() ID = 0, want non-zero")
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
	core := NewCore(orgStorer, noopTransactor{})

	got, err := core.CreateOrganization(t.Context(), mdl.CreateOrganization{Name: "acme", ProjectName: "acme"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	want := mdl.Organization{ID: 1, Name: "acme"}

	testingx.AssertDiff(t, got, want)
}

func TestCore_CreateOrganization_error(t *testing.T) {
	t.Run("invalid input", func(t *testing.T) {
		core := NewCore(&MockedOrgStorer{}, noopTransactor{})

		if _, err := core.CreateOrganization(t.Context(), mdl.CreateOrganization{}); !errors.Is(err, mdl.ErrValidation) {
			t.Errorf("CreateOrganization() error = %v, want mdl.ErrValidation", err)
		}
	})

	t.Run("already exists", func(t *testing.T) {
		core := NewCore(&MockedOrgStorer{
			CreateOrganizationFunc: func(_ context.Context, _ pgorg.CreateOrganization) (pgorg.Organization, error) {
				return pgorg.Organization{}, pgdb.ErrAlreadyExists
			},
		}, noopTransactor{})

		if _, err := core.CreateOrganization(t.Context(), mdl.CreateOrganization{Name: "acme", ProjectName: "acme"}); !errors.Is(err, mdl.ErrAlreadyExists) {
			t.Errorf("CreateOrganization() error = %v, want mdl.ErrAlreadyExists", err)
		}
	})

	t.Run("store error", func(t *testing.T) {
		core := NewCore(&MockedOrgStorer{
			CreateOrganizationFunc: func(_ context.Context, _ pgorg.CreateOrganization) (pgorg.Organization, error) {
				return pgorg.Organization{}, errors.New("db down")
			},
		}, noopTransactor{})

		_, err := core.CreateOrganization(t.Context(), mdl.CreateOrganization{Name: "acme", ProjectName: "acme"})
		if err == nil {
			t.Fatal("CreateOrganization() error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "db down")
	})
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.orgStorer, noopTransactor{})

			if _, err := core.CreateProject(t.Context(), tt.in); !errors.Is(err, tt.want) {
				t.Errorf("CreateProject(%+v) error = %v, want %v", tt.in, err, tt.want)
			}
		})
	}

	t.Run("store error", func(t *testing.T) {
		core := NewCore(&MockedOrgStorer{
			CreateProjectFunc: func(_ context.Context, _ pgorg.CreateProject) (pgorg.Project, error) {
				return pgorg.Project{}, errors.New("db down")
			},
		}, noopTransactor{})

		_, err := core.CreateProject(t.Context(), mdl.CreateProject{OrgID: 7, Name: "widgets"})
		if err == nil {
			t.Fatal("CreateProject() error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "db down")
	})
}

type noopTransactor struct{}

func (noopTransactor) RunTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
