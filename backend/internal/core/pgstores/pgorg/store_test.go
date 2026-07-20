package pgorg

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_CreateOrganization(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := NewStore(pool)

	got, err := orgStore.CreateOrganization(ctx, CreateOrganization{Name: "acme", ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	want := Organization{
		Name:      "acme",
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(
		t, got, want,
		cmpopts.IgnoreFields(Organization{}, "ID", "ControlProjectID"),
		cmpopts.EquateApproxTime(time.Minute),
	)

	if got.ID == 0 {
		t.Error("CreateOrganization() ID = 0, want non-zero")
	}
	if got.ControlProjectID == 0 {
		t.Error("CreateOrganization() ControlProjectID = 0, want non-zero")
	}

	control, err := orgStore.ProjectByName(ctx, got.ID, "control")
	if err != nil {
		t.Fatalf("ProjectByName(%d, %q) error = %v, want the control project to have been created alongside the organization", got.ID, "control", err)
	}

	wantControl := Project{
		OrgID:     got.ID,
		Name:      "control",
		IsControl: true,
		CreatedAt: time.Now(),
	}

	testingx.AssertDiff(
		t, control, wantControl,
		cmpopts.IgnoreFields(Project{}, "ID"),
		cmpopts.EquateApproxTime(time.Minute),
	)

	if control.ID != got.ControlProjectID {
		t.Errorf("control project ID = %d, want %d (Organization.ControlProjectID)", control.ID, got.ControlProjectID)
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

	testingx.AssertDiff(
		t, got, want,
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

	t.Run("duplicate name", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := NewStore(pool)

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
				if _, err := orgStore.CreateProject(ctx, CreateProject{OrgID: org.ID, Name: tt.dup}); !errors.Is(err, pgdb.ErrAlreadyExists) {
					t.Errorf("CreateProject() error = %v, want pgdb.ErrAlreadyExists", err)
				}
			})
		}
	})
}

func TestStore_OrganizationByName(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := NewStore(pool)

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
	orgStore := NewStore(pool)

	if _, err := orgStore.OrganizationByName(ctx, "acme"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("OrganizationByName() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_ProjectByID(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := NewStore(pool)

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
	orgStore := NewStore(pool)

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
			orgStore := NewStore(pool)

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
	orgStore := NewStore(pool)

	org := seedOrg(t, orgStore, "acme")

	if _, err := orgStore.ProjectByName(ctx, org.ID, "widgets"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("ProjectByName() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_IsOrgMember(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	org := seedOrg(t, orgStore, "acme")
	usr, err := userStore.CreateUser(ctx, pguser.CreateUser{Email: "alice@test.com", Name: "Alice Smith"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO org.org_membership (user_id, org_id) VALUES ($1, $2)`, usr.ID, org.ID); err != nil {
		t.Fatalf("seed org membership: %v", err)
	}

	got, err := orgStore.IsOrgMember(ctx, usr.ID, org.ID)
	if err != nil {
		t.Fatalf("IsOrgMember() error = %v", err)
	}
	if !got {
		t.Error("IsOrgMember() = false, want true")
	}
}

func TestStore_IsOrgMember_notAMember(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	org := seedOrg(t, orgStore, "acme")
	usr, err := userStore.CreateUser(ctx, pguser.CreateUser{Email: "alice@test.com", Name: "Alice Smith"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	got, err := orgStore.IsOrgMember(ctx, usr.ID, org.ID)
	if err != nil {
		t.Fatalf("IsOrgMember() error = %v", err)
	}
	if got {
		t.Error("IsOrgMember() = true, want false")
	}
}

func TestProtectControlProjectTrigger(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		org := seedOrg(t, NewStore(pool), "acme")

		_, err := pool.Exec(ctx, `DELETE FROM org.projects WHERE id = $1`, org.ControlProjectID)
		if err == nil {
			t.Fatal("DELETE control project error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "cannot be deleted")
	})

	t.Run("update is_control on a control project", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		org := seedOrg(t, NewStore(pool), "acme")

		_, err := pool.Exec(ctx, `UPDATE org.projects SET is_control = false WHERE id = $1`, org.ControlProjectID)
		if err == nil {
			t.Fatal("UPDATE is_control error = nil, want error")
		}

		testingx.AssertErrContains(t, err, "cannot be changed after creation")
	})

	t.Run("update is_control on an ordinary project", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := NewStore(pool)
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
		org := seedOrg(t, NewStore(pool), "acme")

		if _, err := pool.Exec(ctx, `UPDATE org.projects SET name = 'renamed' WHERE id = $1`, org.ControlProjectID); err != nil {
			t.Errorf("UPDATE name error = %v, want nil", err)
		}
	})
}

func seedOrg(t *testing.T, s *Store, name string) Organization {
	t.Helper()

	org, err := s.CreateOrganization(t.Context(), CreateOrganization{Name: name, ControlProjectName: "control"})
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
