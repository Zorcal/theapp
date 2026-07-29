package pgrbac_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_CreateCustomRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "custom-role-org")

	got, err := rbacStore.CreateCustomRole(ctx, pgrbac.CreateCustomRole{
		OrgID:           org.ID,
		Name:            "project manager",
		PermissionNames: []string{"custom-role:update", "custom-role:read"},
	})
	if err != nil {
		t.Fatalf("CreateCustomRole() error = %v", err)
	}

	want := pgrbac.CustomRole{
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read", "custom-role:update"},
		CreatedAt:       time.Now(),
	}

	testingx.AssertDiff(t, got, want, cmp.Options{
		cmpopts.IgnoreFields(pgrbac.CustomRole{}, "ID", "ExternalID", "ETag"),
		cmpopts.EquateApproxTime(time.Minute),
	})

	if got.ID == 0 {
		t.Error("CreateCustomRole() ID = 0, want non-zero")
	}
	if got.ExternalID == uuid.Nil {
		t.Error("CreateCustomRole() ExternalID = zero UUID, want non-zero")
	}
	if got.ETag == uuid.Nil {
		t.Error("CreateCustomRole() ETag = zero UUID, want non-zero")
	}
}

func TestStore_CreateCustomRole_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "custom-role-error-org")

	// Establish the existing name used by the duplicate-name case.
	if _, err := rbacStore.CreateCustomRole(ctx, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "project manager"}); err != nil {
		t.Fatalf("CreateCustomRole() seed error = %v", err)
	}

	tests := []struct {
		name string
		in   pgrbac.CreateCustomRole
		want error
	}{
		{
			name: "duplicate name ignoring case",
			in: pgrbac.CreateCustomRole{
				OrgID: org.ID,
				Name:  "PROJECT MANAGER",
			},
			want: pgdb.ErrAlreadyExists,
		},
		{
			name: "unknown organization",
			in: pgrbac.CreateCustomRole{
				OrgID: 999999,
				Name:  "role in unknown organization",
			},
			want: sql.ErrNoRows,
		},
		{
			name: "unknown permission",
			in: pgrbac.CreateCustomRole{
				OrgID:           org.ID,
				Name:            "unknown permission role",
				PermissionNames: []string{"permission:unknown"},
			},
			want: sql.ErrNoRows,
		},
		{
			name: "empty name",
			in: pgrbac.CreateCustomRole{
				OrgID: org.ID,
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
		{
			name: "leading whitespace in name",
			in: pgrbac.CreateCustomRole{
				OrgID: org.ID,
				Name:  " project manager",
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
		{
			name: "trailing whitespace in name",
			in: pgrbac.CreateCustomRole{
				OrgID: org.ID,
				Name:  "project manager ",
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := rbacStore.CreateCustomRole(ctx, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("CreateCustomRole(%+v) error = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

func TestStore_UpdateCustomRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "update-custom-role-org")
	seeded := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})

	in := pgrbac.UpdateCustomRole{
		OrgID:           org.ID,
		ExternalID:      seeded.ExternalID,
		Fields:          pgrbac.CustomRoleUpdateFields{Name: true, PermissionNames: true},
		Name:            "editor",
		PermissionNames: []string{"custom-role:delete", "custom-role:update"},
	}
	got, err := rbacStore.UpdateCustomRole(ctx, in)
	if err != nil {
		t.Fatalf("UpdateCustomRole() error = %v", err)
	}

	want := seeded
	want.Name = in.Name
	want.PermissionNames = in.PermissionNames

	testingx.AssertDiff(t, got, want, cmp.Options{
		cmpopts.IgnoreFields(pgrbac.CustomRole{}, "UpdatedAt", "ETag"),
	})

	if got.UpdatedAt == nil {
		t.Error("UpdateCustomRole() UpdatedAt = nil, want non-nil")
	}
	if got.ETag == seeded.ETag {
		t.Error("UpdateCustomRole() ETag unchanged, want new ETag")
	}

	gotIgnored, err := rbacStore.UpdateCustomRole(ctx, pgrbac.UpdateCustomRole{
		OrgID:           org.ID,
		ExternalID:      seeded.ExternalID,
		Name:            "ignored",
		PermissionNames: []string{"permission:unknown"},
	})
	if err != nil {
		t.Fatalf("UpdateCustomRole() with no selected fields error = %v", err)
	}

	testingx.AssertDiff(t, gotIgnored, got, cmp.Options{
		cmpopts.IgnoreFields(pgrbac.CustomRole{}, "UpdatedAt", "ETag"),
	})
}

func TestStore_UpdateCustomRole_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-update-custom-role-org")
	secondOrg := seedOrg(t, orgStore, "second-update-custom-role-org")
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})
	seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: firstOrg.ID, Name: "editor"})

	tests := []struct {
		name string
		in   pgrbac.UpdateCustomRole
		want error
	}{
		{
			name: "role belongs to another organization",
			in: pgrbac.UpdateCustomRole{
				OrgID:      secondOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     pgrbac.CustomRoleUpdateFields{Name: true},
				Name:       "renamed",
			},
			want: sql.ErrNoRows,
		},
		{
			name: "unknown permission",
			in: pgrbac.UpdateCustomRole{
				OrgID:           firstOrg.ID,
				ExternalID:      role.ExternalID,
				Fields:          pgrbac.CustomRoleUpdateFields{PermissionNames: true},
				Name:            "renamed",
				PermissionNames: []string{"permission:unknown"},
			},
			want: sql.ErrNoRows,
		},
		{
			name: "duplicate name",
			in: pgrbac.UpdateCustomRole{
				OrgID:      firstOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     pgrbac.CustomRoleUpdateFields{Name: true},
				Name:       "EDITOR",
			},
			want: pgdb.ErrAlreadyExists,
		},
		{
			name: "empty name",
			in: pgrbac.UpdateCustomRole{
				OrgID:      firstOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     pgrbac.CustomRoleUpdateFields{Name: true},
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
		{
			name: "leading whitespace in name",
			in: pgrbac.UpdateCustomRole{
				OrgID:      firstOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     pgrbac.CustomRoleUpdateFields{Name: true},
				Name:       " renamed",
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
		{
			name: "trailing whitespace in name",
			in: pgrbac.UpdateCustomRole{
				OrgID:      firstOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     pgrbac.CustomRoleUpdateFields{Name: true},
				Name:       "renamed ",
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := rbacStore.UpdateCustomRole(ctx, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("UpdateCustomRole(%+v) error = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

func TestStore_ModifyCustomRolePermissions(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "modify-custom-role-permissions-org")
	seeded := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "role manager", PermissionNames: []string{"custom-role:read", "custom-role:update"}})

	in := pgrbac.ModifyCustomRolePermissions{
		OrgID:                 org.ID,
		ExternalID:            seeded.ExternalID,
		AddPermissionNames:    []string{"custom-role:delete"},
		RemovePermissionNames: []string{"custom-role:read"},
	}
	got, err := rbacStore.ModifyCustomRolePermissions(ctx, in)
	if err != nil {
		t.Fatalf("ModifyCustomRolePermissions() error = %v", err)
	}

	want := seeded
	want.PermissionNames = slices.Concat(in.AddPermissionNames, []string{"custom-role:update"})

	testingx.AssertDiff(t, got, want, cmp.Options{
		cmpopts.IgnoreFields(pgrbac.CustomRole{}, "UpdatedAt", "ETag"),
	})

	if got.UpdatedAt == nil {
		t.Error("ModifyCustomRolePermissions() UpdatedAt = nil, want non-nil")
	}
	if got.ETag == seeded.ETag {
		t.Error("ModifyCustomRolePermissions() ETag unchanged, want new ETag")
	}

	gotNoOp, err := rbacStore.ModifyCustomRolePermissions(ctx, in)
	if err != nil {
		t.Fatalf("ModifyCustomRolePermissions() no-op error = %v", err)
	}

	testingx.AssertDiff(t, gotNoOp, got)
}

func TestStore_ModifyCustomRolePermissions_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-modify-custom-role-org")
	secondOrg := seedOrg(t, orgStore, "second-modify-custom-role-org")
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})

	tests := []struct {
		name string
		in   pgrbac.ModifyCustomRolePermissions
	}{
		{
			name: "role belongs to another organization",
			in: pgrbac.ModifyCustomRolePermissions{
				OrgID:              secondOrg.ID,
				ExternalID:         role.ExternalID,
				AddPermissionNames: []string{"custom-role:read"},
			},
		},
		{
			name: "unknown permission",
			in: pgrbac.ModifyCustomRolePermissions{
				OrgID:              firstOrg.ID,
				ExternalID:         role.ExternalID,
				AddPermissionNames: []string{"permission:unknown"},
			},
		},
		{
			name: "unknown permission to remove",
			in: pgrbac.ModifyCustomRolePermissions{
				OrgID:                 firstOrg.ID,
				ExternalID:            role.ExternalID,
				RemovePermissionNames: []string{"permission:unknown"},
			},
		},
		{
			name: "role not found",
			in: pgrbac.ModifyCustomRolePermissions{
				OrgID:              firstOrg.ID,
				ExternalID:         uuid.New(),
				AddPermissionNames: []string{"custom-role:read"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := rbacStore.ModifyCustomRolePermissions(ctx, tt.in); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("ModifyCustomRolePermissions(%+v) error = %v, want sql.ErrNoRows", tt.in, err)
			}
		})
	}
}

func TestStore_DeleteCustomRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)
	userStore := pguser.NewStore(pool)

	org := seedOrg(t, orgStore, "delete-custom-role-org")
	project := seedProject(t, orgStore, org.ID, "project")
	user := seedUser(t, userStore, "delete-custom-role@test.com")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, role.ExternalID, project.ID)
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, role.ExternalID, org.ID)

	if err := rbacStore.DeleteCustomRole(ctx, org.ID, role.ExternalID); err != nil {
		t.Fatalf("DeleteCustomRole() error = %v", err)
	}

	if _, err := rbacStore.CustomRoleByExternalID(ctx, org.ID, role.ExternalID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("CustomRoleByExternalID() after delete error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_DeleteCustomRole_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-delete-custom-role-org")
	secondOrg := seedOrg(t, orgStore, "second-delete-custom-role-org")
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})

	if err := rbacStore.DeleteCustomRole(ctx, secondOrg.ID, role.ExternalID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteCustomRole() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_CustomRoles(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-custom-role-org")
	secondOrg := seedOrg(t, orgStore, "second-custom-role-org")
	firstRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: firstOrg.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})
	seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: secondOrg.ID, Name: "other org role"})

	gotFirstPage, err := rbacStore.CustomRoles(ctx, firstOrg.ID, 50, 0)
	if err != nil {
		t.Fatalf("CustomRoles() error = %v", err)
	}

	wantFirstPage := []pgrbac.CustomRole{firstRole}

	testingx.AssertDiff(t, gotFirstPage, wantFirstPage)

	gotSecondPage, err := rbacStore.CustomRoles(ctx, firstOrg.ID, 50, 1)
	if err != nil {
		t.Fatalf("CustomRoles() second page error = %v", err)
	}

	wantSecondPage := []pgrbac.CustomRole{}

	testingx.AssertDiff(t, gotSecondPage, wantSecondPage)
}

func TestStore_CustomRoleCount(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-custom-role-count-org")
	secondOrg := seedOrg(t, orgStore, "second-custom-role-count-org")
	seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})
	seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: firstOrg.ID, Name: "editor"})
	seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: secondOrg.ID, Name: "other org role"})

	got, err := rbacStore.CustomRoleCount(ctx, firstOrg.ID)
	if err != nil {
		t.Fatalf("CustomRoleCount() error = %v", err)
	}

	if want := 2; got != want {
		t.Errorf("CustomRoleCount() = %d, want %d", got, want)
	}
}

func TestStore_CustomRoleByExternalID(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "custom-role-by-id-org")
	want := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{
		OrgID: org.ID,
		Name:  "reader",
	})

	got, err := rbacStore.CustomRoleByExternalID(ctx, org.ID, want.ExternalID)
	if err != nil {
		t.Fatalf("CustomRoleByExternalID() error = %v", err)
	}

	testingx.AssertDiff(t, got, want)
}

func TestStore_CustomRoleByExternalID_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-role-lookup-org")
	secondOrg := seedOrg(t, orgStore, "second-role-lookup-org")
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})

	if _, err := rbacStore.CustomRoleByExternalID(ctx, secondOrg.ID, role.ExternalID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("CustomRoleByExternalID() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_UserProjectCustomRoles(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "list-user-project-custom-roles-org")
	project := seedProject(t, orgStore, org.ID, "first project")
	user := seedUser(t, userStore, "list-user-project-custom-roles@test.com")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	firstProjectRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "project reader", PermissionNames: []string{"custom-role:read"}})
	secondProjectRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "project writer"})
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, firstProjectRole.ExternalID, project.ID)
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, secondProjectRole.ExternalID, project.ID)

	// Assignments in another project and at organization scope must not appear in the requested project.
	secondProject := seedProject(t, orgStore, org.ID, "second project")
	otherProjectRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "other project reader"})
	orgRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "organization reader"})
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, otherProjectRole.ExternalID, secondProject.ID)
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, orgRole.ExternalID, org.ID)

	tests := []struct {
		name       string
		pageOffset int
		want       []pgrbac.CustomRole
	}{
		{
			name:       "first page",
			pageOffset: 0,
			want:       []pgrbac.CustomRole{firstProjectRole},
		},
		{
			name:       "second page",
			pageOffset: 1,
			want:       []pgrbac.CustomRole{secondProjectRole},
		},
		{
			name:       "page after assignments",
			pageOffset: 2,
			want:       []pgrbac.CustomRole{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacStore.UserProjectCustomRoles(ctx, user.ExternalID, project.ID, 1, tt.pageOffset)
			if err != nil {
				t.Fatalf("UserProjectCustomRoles(%v, %d, 1, %d) error = %v", user.ExternalID, project.ID, tt.pageOffset, err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestStore_UserProjectCustomRoles_empty(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "empty-user-project-custom-roles-org")
	project := seedProject(t, orgStore, org.ID, "project")
	unassignedUser := seedUser(t, userStore, "empty-user-project-custom-roles@test.com")
	nonMember := seedUser(t, userStore, "non-member-project-custom-roles@test.com")
	seedOrgMembership(t, ctx, pool, unassignedUser.ID, org.ID)

	tests := []struct {
		name      string
		userID    uuid.UUID
		projectID int
	}{
		{
			name:      "no assignments",
			userID:    unassignedUser.ExternalID,
			projectID: project.ID,
		},
		{
			name:      "user missing",
			userID:    uuid.New(),
			projectID: project.ID,
		},
		{
			name:      "project missing",
			userID:    unassignedUser.ExternalID,
			projectID: -1,
		},
		{
			name:      "organization membership missing",
			userID:    nonMember.ExternalID,
			projectID: project.ID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacStore.UserProjectCustomRoles(ctx, tt.userID, tt.projectID, 50, 0)
			if err != nil {
				t.Fatalf("UserProjectCustomRoles(%v, %d) error = %v", tt.userID, tt.projectID, err)
			}

			want := []pgrbac.CustomRole{}

			testingx.AssertDiff(t, got, want)
		})
	}
}

func TestStore_UserProjectCustomRoleCount(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "count-user-project-custom-roles-org")
	project := seedProject(t, orgStore, org.ID, "first project")
	user := seedUser(t, userStore, "count-user-project-custom-roles@test.com")
	unassignedUser := seedUser(t, userStore, "count-empty-user-project-custom-roles@test.com")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	seedOrgMembership(t, ctx, pool, unassignedUser.ID, org.ID)
	firstRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})
	secondRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "writer"})
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, firstRole.ExternalID, project.ID)
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, secondRole.ExternalID, project.ID)

	// Assignments in another project and at organization scope must not contribute to the count.
	secondProject := seedProject(t, orgStore, org.ID, "second project")
	otherProjectRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "other project reader"})
	orgRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "organization reader"})
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, otherProjectRole.ExternalID, secondProject.ID)
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, orgRole.ExternalID, org.ID)

	tests := []struct {
		name   string
		userID uuid.UUID
		want   int
	}{
		{
			name:   "assigned roles",
			userID: user.ExternalID,
			want:   2,
		},
		{
			name:   "no assignments",
			userID: unassignedUser.ExternalID,
			want:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacStore.UserProjectCustomRoleCount(ctx, tt.userID, project.ID)
			if err != nil {
				t.Fatalf("UserProjectCustomRoleCount(%v, %d) error = %v", tt.userID, project.ID, err)
			}

			if got != tt.want {
				t.Errorf("UserProjectCustomRoleCount(%v, %d) = %d, want %d", tt.userID, project.ID, got, tt.want)
			}
		})
	}
}

func TestStore_UserProjectCustomRoleCount_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "count-project-roles-error-org")
	project := seedProject(t, orgStore, org.ID, "project")
	member := seedUser(t, userStore, "count-project-roles-member@test.com")
	nonMember := seedUser(t, userStore, "count-project-roles-non-member@test.com")
	seedOrgMembership(t, ctx, pool, member.ID, org.ID)

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
			userID:    member.ExternalID,
			projectID: -1,
		},
		{
			name:      "organization membership missing",
			userID:    nonMember.ExternalID,
			projectID: project.ID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := rbacStore.UserProjectCustomRoleCount(ctx, tt.userID, tt.projectID); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("UserProjectCustomRoleCount(%v, %d) error = %v, want sql.ErrNoRows", tt.userID, tt.projectID, err)
			}
		})
	}
}

func TestStore_UserOrgCustomRoles(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "list-user-org-custom-roles-org")
	user := seedUser(t, userStore, "list-user-org-custom-roles@test.com")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	firstOrgRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "organization reader", PermissionNames: []string{"custom-role:read"}})
	secondOrgRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "organization writer"})
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, firstOrgRole.ExternalID, org.ID)
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, secondOrgRole.ExternalID, org.ID)

	// Project-scope and other-organization assignments must not appear in the requested organization.
	otherOrg := seedOrg(t, orgStore, "other-list-user-org-custom-roles-org")
	project := seedProject(t, orgStore, org.ID, "project")
	seedOrgMembership(t, ctx, pool, user.ID, otherOrg.ID)
	projectRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "project reader"})
	otherOrgRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: otherOrg.ID, Name: "other organization reader"})
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, projectRole.ExternalID, project.ID)
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, otherOrgRole.ExternalID, otherOrg.ID)

	tests := []struct {
		name       string
		pageOffset int
		want       []pgrbac.CustomRole
	}{
		{
			name:       "first page",
			pageOffset: 0,
			want:       []pgrbac.CustomRole{firstOrgRole},
		},
		{
			name:       "second page",
			pageOffset: 1,
			want:       []pgrbac.CustomRole{secondOrgRole},
		},
		{
			name:       "page after assignments",
			pageOffset: 2,
			want:       []pgrbac.CustomRole{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacStore.UserOrgCustomRoles(ctx, user.ExternalID, org.ID, 1, tt.pageOffset)
			if err != nil {
				t.Fatalf("UserOrgCustomRoles(%v, %d, 1, %d) error = %v", user.ExternalID, org.ID, tt.pageOffset, err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestStore_UserOrgCustomRoles_empty(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "empty-user-org-custom-roles-org")
	unassignedUser := seedUser(t, userStore, "empty-user-org-custom-roles@test.com")
	nonMember := seedUser(t, userStore, "non-member-org-custom-roles@test.com")
	seedOrgMembership(t, ctx, pool, unassignedUser.ID, org.ID)

	tests := []struct {
		name   string
		userID uuid.UUID
	}{
		{
			name:   "no assignments",
			userID: unassignedUser.ExternalID,
		},
		{
			name:   "user missing",
			userID: uuid.New(),
		},
		{
			name:   "organization membership missing",
			userID: nonMember.ExternalID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacStore.UserOrgCustomRoles(ctx, tt.userID, org.ID, 50, 0)
			if err != nil {
				t.Fatalf("UserOrgCustomRoles(%v, %d) error = %v", tt.userID, org.ID, err)
			}

			want := []pgrbac.CustomRole{}

			testingx.AssertDiff(t, got, want)
		})
	}
}

func TestStore_UserOrgCustomRoleCount(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "count-user-org-custom-roles-org")
	user := seedUser(t, userStore, "count-user-org-custom-roles@test.com")
	unassignedUser := seedUser(t, userStore, "count-empty-user-org-custom-roles@test.com")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	seedOrgMembership(t, ctx, pool, unassignedUser.ID, org.ID)
	firstRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})
	secondRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "writer"})
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, firstRole.ExternalID, org.ID)
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, secondRole.ExternalID, org.ID)

	// Project-scope and other-organization assignments must not contribute to the count.
	otherOrg := seedOrg(t, orgStore, "other-count-user-org-custom-roles-org")
	project := seedProject(t, orgStore, org.ID, "project")
	seedOrgMembership(t, ctx, pool, user.ID, otherOrg.ID)
	projectRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "project reader"})
	otherOrgRole := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: otherOrg.ID, Name: "other organization reader"})
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, projectRole.ExternalID, project.ID)
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, otherOrgRole.ExternalID, otherOrg.ID)

	tests := []struct {
		name   string
		userID uuid.UUID
		want   int
	}{
		{
			name:   "assigned roles",
			userID: user.ExternalID,
			want:   2,
		},
		{
			name:   "no assignments",
			userID: unassignedUser.ExternalID,
			want:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacStore.UserOrgCustomRoleCount(ctx, tt.userID, org.ID)
			if err != nil {
				t.Fatalf("UserOrgCustomRoleCount(%v, %d) error = %v", tt.userID, org.ID, err)
			}

			if got != tt.want {
				t.Errorf("UserOrgCustomRoleCount(%v, %d) = %d, want %d", tt.userID, org.ID, got, tt.want)
			}
		})
	}
}

func TestStore_UserOrgCustomRoleCount_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "count-org-roles-error-org")
	nonMember := seedUser(t, userStore, "count-org-roles-non-member@test.com")

	tests := []struct {
		name   string
		userID uuid.UUID
	}{
		{
			name:   "user missing",
			userID: uuid.New(),
		},
		{
			name:   "organization membership missing",
			userID: nonMember.ExternalID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := rbacStore.UserOrgCustomRoleCount(ctx, tt.userID, org.ID); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("UserOrgCustomRoleCount(%v, %d) error = %v, want sql.ErrNoRows", tt.userID, org.ID, err)
			}
		})
	}
}

func TestStore_AssignCustomRoleToProject(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "assign-custom-role-project-org")
	project := seedProject(t, orgStore, org.ID, "project")
	user := seedUser(t, userStore, "assign-custom-role-project@test.com")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})

	if err := rbacStore.AssignCustomRoleToProject(ctx, user.ExternalID, role.ExternalID, project.ID); err != nil {
		t.Fatalf("AssignCustomRoleToProject() error = %v", err)
	}

	got, err := rbacStore.ProjectPermissions(ctx, user.ExternalID, project.ID)
	if err != nil {
		t.Fatalf("ProjectPermissions() error = %v", err)
	}

	want := pgrbac.ProjectPermissions{
		OrgID:           org.ID,
		PermissionNames: role.PermissionNames,
	}

	testingx.AssertDiff(t, got, want, cmpopts.EquateEmpty())
}

func TestStore_AssignCustomRoleToProject_error(t *testing.T) {
	t.Run("not an organization member", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "nonmember-assign-custom-role-project-org")
		project := seedProject(t, orgStore, org.ID, "project")
		user := seedUser(t, userStore, "nonmember-assign-project@test.com")
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})

		if err := rbacStore.AssignCustomRoleToProject(ctx, user.ExternalID, role.ExternalID, project.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignCustomRoleToProject() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("different organization", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		firstOrg := seedOrg(t, orgStore, "first-assign-custom-role-project-org")
		secondOrg := seedOrg(t, orgStore, "second-assign-custom-role-project-org")
		project := seedProject(t, orgStore, firstOrg.ID, "project")
		user := seedUser(t, userStore, "different-org-assign-project@test.com")
		seedOrgMembership(t, ctx, pool, user.ID, firstOrg.ID)
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: secondOrg.ID, Name: "reader"})

		if err := rbacStore.AssignCustomRoleToProject(ctx, user.ExternalID, role.ExternalID, project.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignCustomRoleToProject() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("already assigned", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "duplicate-assign-custom-role-project-org")
		project := seedProject(t, orgStore, org.ID, "project")
		user := seedUser(t, userStore, "duplicate-assign-project@test.com")
		seedOrgMembership(t, ctx, pool, user.ID, org.ID)
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})
		seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, role.ExternalID, project.ID)

		if err := rbacStore.AssignCustomRoleToProject(ctx, user.ExternalID, role.ExternalID, project.ID); !errors.Is(err, pgdb.ErrAlreadyExists) {
			t.Errorf("AssignCustomRoleToProject() error = %v, want pgdb.ErrAlreadyExists", err)
		}
	})
}

func TestStore_UnassignCustomRoleFromProject(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "unassign-custom-role-project-org")
	project := seedProject(t, orgStore, org.ID, "project")
	user := seedUser(t, userStore, "unassign-custom-role-project@test.com")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})
	seedProjectRoleAssignment(t, ctx, rbacStore, user.ExternalID, role.ExternalID, project.ID)

	if err := rbacStore.UnassignCustomRoleFromProject(ctx, user.ExternalID, role.ExternalID, project.ID); err != nil {
		t.Fatalf("UnassignCustomRoleFromProject() error = %v", err)
	}

	got, err := rbacStore.ProjectPermissions(ctx, user.ExternalID, project.ID)
	if err != nil {
		t.Fatalf("ProjectPermissions() error = %v", err)
	}

	want := pgrbac.ProjectPermissions{
		OrgID: org.ID,
	}

	testingx.AssertDiff(t, got, want, cmpopts.EquateEmpty())
}

func TestStore_UnassignCustomRoleFromProject_error(t *testing.T) {
	t.Run("assignment missing", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "missing-unassign-custom-role-project-org")
		project := seedProject(t, orgStore, org.ID, "project")
		user := seedUser(t, userStore, "missing-unassign-project@test.com")
		seedOrgMembership(t, ctx, pool, user.ID, org.ID)
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})

		if err := rbacStore.UnassignCustomRoleFromProject(ctx, user.ExternalID, role.ExternalID, project.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignCustomRoleFromProject() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("not an organization member", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "nonmember-unassign-custom-role-project-org")
		project := seedProject(t, orgStore, org.ID, "project")
		user := seedUser(t, userStore, "nonmember-unassign-project@test.com")
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})

		if err := rbacStore.UnassignCustomRoleFromProject(ctx, user.ExternalID, role.ExternalID, project.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignCustomRoleFromProject() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_AssignCustomRoleToOrg(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "assign-custom-role-org-org")
	project := seedProject(t, orgStore, org.ID, "project")
	user := seedUser(t, userStore, "assign-custom-role-org@test.com")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})

	if err := rbacStore.AssignCustomRoleToOrg(ctx, user.ExternalID, role.ExternalID, org.ID); err != nil {
		t.Fatalf("AssignCustomRoleToOrg() error = %v", err)
	}

	got, err := rbacStore.ProjectPermissions(ctx, user.ExternalID, project.ID)
	if err != nil {
		t.Fatalf("ProjectPermissions() error = %v", err)
	}

	want := pgrbac.ProjectPermissions{
		OrgID:           org.ID,
		PermissionNames: role.PermissionNames,
	}

	testingx.AssertDiff(t, got, want, cmpopts.EquateEmpty())
}

func TestStore_AssignCustomRoleToOrg_error(t *testing.T) {
	t.Run("not an organization member", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "nonmember-assign-custom-role-org")
		user := seedUser(t, userStore, "nonmember-assign-org@test.com")
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})

		if err := rbacStore.AssignCustomRoleToOrg(ctx, user.ExternalID, role.ExternalID, org.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignCustomRoleToOrg() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("different organization", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		firstOrg := seedOrg(t, orgStore, "first-assign-custom-role-org")
		secondOrg := seedOrg(t, orgStore, "second-assign-custom-role-org")
		user := seedUser(t, userStore, "different-org-assign-org@test.com")
		seedOrgMembership(t, ctx, pool, user.ID, firstOrg.ID)
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: secondOrg.ID, Name: "reader"})

		if err := rbacStore.AssignCustomRoleToOrg(ctx, user.ExternalID, role.ExternalID, firstOrg.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignCustomRoleToOrg() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("already assigned", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "duplicate-assign-custom-role-org")
		user := seedUser(t, userStore, "duplicate-assign-org@test.com")
		seedOrgMembership(t, ctx, pool, user.ID, org.ID)
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})
		seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, role.ExternalID, org.ID)

		if err := rbacStore.AssignCustomRoleToOrg(ctx, user.ExternalID, role.ExternalID, org.ID); !errors.Is(err, pgdb.ErrAlreadyExists) {
			t.Errorf("AssignCustomRoleToOrg() error = %v, want pgdb.ErrAlreadyExists", err)
		}
	})
}

func TestStore_UnassignCustomRoleFromOrg(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)

	org := seedOrg(t, orgStore, "unassign-custom-role-org-org")
	project := seedProject(t, orgStore, org.ID, "project")
	user := seedUser(t, userStore, "unassign-custom-role-org@test.com")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})
	seedOrgRoleAssignment(t, ctx, rbacStore, user.ExternalID, role.ExternalID, org.ID)

	if err := rbacStore.UnassignCustomRoleFromOrg(ctx, user.ExternalID, role.ExternalID, org.ID); err != nil {
		t.Fatalf("UnassignCustomRoleFromOrg() error = %v", err)
	}

	got, err := rbacStore.ProjectPermissions(ctx, user.ExternalID, project.ID)
	if err != nil {
		t.Fatalf("ProjectPermissions() error = %v", err)
	}

	want := pgrbac.ProjectPermissions{
		OrgID: org.ID,
	}

	testingx.AssertDiff(t, got, want, cmpopts.EquateEmpty())
}

func TestStore_UnassignCustomRoleFromOrg_error(t *testing.T) {
	t.Run("assignment missing", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "missing-unassign-custom-role-org")
		user := seedUser(t, userStore, "missing-unassign-org@test.com")
		seedOrgMembership(t, ctx, pool, user.ID, org.ID)
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})

		if err := rbacStore.UnassignCustomRoleFromOrg(ctx, user.ExternalID, role.ExternalID, org.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignCustomRoleFromOrg() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("not an organization member", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		orgStore := pgorg.NewStore(pool)
		userStore := pguser.NewStore(pool)
		rbacStore := pgrbac.NewStore(pool)

		org := seedOrg(t, orgStore, "nonmember-unassign-custom-role-org")
		user := seedUser(t, userStore, "nonmember-unassign-org@test.com")
		role := seedCustomRole(t, rbacStore, pgrbac.CreateCustomRole{OrgID: org.ID, Name: "reader"})

		if err := rbacStore.UnassignCustomRoleFromOrg(ctx, user.ExternalID, role.ExternalID, org.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignCustomRoleFromOrg() error = %v, want sql.ErrNoRows", err)
		}
	})
}
