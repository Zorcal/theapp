package pgorg

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_CreateOrganization(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := NewStore(pool)

	got, err := orgStore.CreateOrganization(ctx, CreateOrganization{Name: "acme"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	want := Organization{
		Name:      "acme",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(t, got, want,
		cmpopts.IgnoreFields(Organization{}, "ID"),
		cmpopts.EquateApproxTime(time.Minute),
	)

	if got.ID == 0 {
		t.Error("CreateOrganization() ID = 0, want non-zero")
	}
}

func TestStore_CreateOrganization_error(t *testing.T) {
	t.Run("duplicate name", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := NewStore(pool)

		seedOrg(t, orgStore, "acme")

		if _, err := orgStore.CreateOrganization(ctx, CreateOrganization{Name: "acme"}); !errors.Is(err, pgdb.ErrAlreadyExists) {
			t.Errorf("CreateOrganization() error = %v, want pgdb.ErrAlreadyExists", err)
		}
	})
}

func TestStore_CreateProject(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := NewStore(pool)

	org := seedOrg(t, orgStore, "acme")

	got, err := orgStore.CreateProject(ctx, CreateProject{OrgID: org.ID, Name: "widgets"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	want := Project{
		OrgID:     org.ID,
		Name:      "widgets",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(t, got, want,
		cmpopts.IgnoreFields(Project{}, "ID"),
		cmpopts.EquateApproxTime(time.Minute),
	)

	if got.ID == 0 {
		t.Error("CreateProject() ID = 0, want non-zero")
	}
}

func TestStore_CreateProject_error(t *testing.T) {
	t.Run("org not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := NewStore(pool)

		if _, err := orgStore.CreateProject(ctx, CreateProject{OrgID: 999999, Name: "widgets"}); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("CreateProject() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("duplicate name in org", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := NewStore(pool)

		org := seedOrg(t, orgStore, "acme")
		seedProject(t, orgStore, org.ID, "widgets")

		if _, err := orgStore.CreateProject(ctx, CreateProject{OrgID: org.ID, Name: "widgets"}); !errors.Is(err, pgdb.ErrAlreadyExists) {
			t.Errorf("CreateProject() error = %v, want pgdb.ErrAlreadyExists", err)
		}
	})
}

func seedOrg(t *testing.T, s *Store, name string) Organization {
	t.Helper()

	org, err := s.CreateOrganization(t.Context(), CreateOrganization{Name: name})
	if err != nil {
		t.Fatalf("seed org %q: %v", name, err)
	}

	return org
}

func seedProject(t *testing.T, s *Store, orgID int, name string) Project {
	t.Helper()

	project, err := s.CreateProject(t.Context(), CreateProject{OrgID: orgID, Name: name})
	if err != nil {
		t.Fatalf("seed project %q: %v", name, err)
	}

	return project
}
