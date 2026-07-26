package pgrbac

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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_CreateCustomRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := NewStore(pool)

	org := seedOrg(t, orgStore, "custom-role-org")

	got, err := rbacStore.CreateCustomRole(ctx, CreateCustomRole{
		OrgID:           org.ID,
		Name:            "project manager",
		PermissionNames: []string{"custom-role:update", "custom-role:read"},
	})
	if err != nil {
		t.Fatalf("CreateCustomRole() error = %v", err)
	}

	want := CustomRole{
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read", "custom-role:update"},
		CreatedAt:       time.Now(),
	}

	testingx.AssertDiff(t, got, want, cmp.Options{
		cmpopts.IgnoreFields(CustomRole{}, "ID", "ExternalID", "ETag"),
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
	rbacStore := NewStore(pool)

	org := seedOrg(t, orgStore, "custom-role-error-org")

	// Establish the existing name used by the duplicate-name case.
	if _, err := rbacStore.CreateCustomRole(ctx, CreateCustomRole{OrgID: org.ID, Name: "project manager"}); err != nil {
		t.Fatalf("CreateCustomRole() seed error = %v", err)
	}

	tests := []struct {
		name string
		in   CreateCustomRole
		want error
	}{
		{
			name: "duplicate name ignoring case",
			in: CreateCustomRole{
				OrgID: org.ID,
				Name:  "PROJECT MANAGER",
			},
			want: pgdb.ErrAlreadyExists,
		},
		{
			name: "unknown organization",
			in: CreateCustomRole{
				OrgID: 999999,
				Name:  "role in unknown organization",
			},
			want: sql.ErrNoRows,
		},
		{
			name: "unknown permission",
			in: CreateCustomRole{
				OrgID:           org.ID,
				Name:            "unknown permission role",
				PermissionNames: []string{"permission:unknown"},
			},
			want: sql.ErrNoRows,
		},
		{
			name: "empty name",
			in: CreateCustomRole{
				OrgID: org.ID,
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
		{
			name: "leading whitespace in name",
			in: CreateCustomRole{
				OrgID: org.ID,
				Name:  " project manager",
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
		{
			name: "trailing whitespace in name",
			in: CreateCustomRole{
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
	rbacStore := NewStore(pool)

	org := seedOrg(t, orgStore, "update-custom-role-org")
	seeded := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})

	in := UpdateCustomRole{
		OrgID:           org.ID,
		ExternalID:      seeded.ExternalID,
		Fields:          CustomRoleUpdateFields{Name: true, PermissionNames: true},
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
		cmpopts.IgnoreFields(CustomRole{}, "UpdatedAt", "ETag"),
	})

	if got.UpdatedAt == nil {
		t.Error("UpdateCustomRole() UpdatedAt = nil, want non-nil")
	}
	if got.ETag == seeded.ETag {
		t.Error("UpdateCustomRole() ETag unchanged, want new ETag")
	}

	gotIgnored, err := rbacStore.UpdateCustomRole(ctx, UpdateCustomRole{
		OrgID:           org.ID,
		ExternalID:      seeded.ExternalID,
		Name:            "ignored",
		PermissionNames: []string{"permission:unknown"},
	})
	if err != nil {
		t.Fatalf("UpdateCustomRole() with no selected fields error = %v", err)
	}

	testingx.AssertDiff(t, gotIgnored, got, cmp.Options{
		cmpopts.IgnoreFields(CustomRole{}, "UpdatedAt", "ETag"),
	})
}

func TestStore_UpdateCustomRole_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-update-custom-role-org")
	secondOrg := seedOrg(t, orgStore, "second-update-custom-role-org")
	role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})
	seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: firstOrg.ID, Name: "editor"})

	tests := []struct {
		name string
		in   UpdateCustomRole
		want error
	}{
		{
			name: "role belongs to another organization",
			in: UpdateCustomRole{
				OrgID:      secondOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     CustomRoleUpdateFields{Name: true},
				Name:       "renamed",
			},
			want: sql.ErrNoRows,
		},
		{
			name: "unknown permission",
			in: UpdateCustomRole{
				OrgID:           firstOrg.ID,
				ExternalID:      role.ExternalID,
				Fields:          CustomRoleUpdateFields{PermissionNames: true},
				Name:            "renamed",
				PermissionNames: []string{"permission:unknown"},
			},
			want: sql.ErrNoRows,
		},
		{
			name: "duplicate name",
			in: UpdateCustomRole{
				OrgID:      firstOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     CustomRoleUpdateFields{Name: true},
				Name:       "EDITOR",
			},
			want: pgdb.ErrAlreadyExists,
		},
		{
			name: "empty name",
			in: UpdateCustomRole{
				OrgID:      firstOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     CustomRoleUpdateFields{Name: true},
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
		{
			name: "leading whitespace in name",
			in: UpdateCustomRole{
				OrgID:      firstOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     CustomRoleUpdateFields{Name: true},
				Name:       " renamed",
			},
			want: pgdb.ErrCheckConstraintViolated,
		},
		{
			name: "trailing whitespace in name",
			in: UpdateCustomRole{
				OrgID:      firstOrg.ID,
				ExternalID: role.ExternalID,
				Fields:     CustomRoleUpdateFields{Name: true},
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
	rbacStore := NewStore(pool)

	org := seedOrg(t, orgStore, "modify-custom-role-permissions-org")
	seeded := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "role manager", PermissionNames: []string{"custom-role:read", "custom-role:update"}})

	in := ModifyCustomRolePermissions{
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
		cmpopts.IgnoreFields(CustomRole{}, "UpdatedAt", "ETag"),
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
	rbacStore := NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-modify-custom-role-org")
	secondOrg := seedOrg(t, orgStore, "second-modify-custom-role-org")
	role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})

	tests := []struct {
		name string
		in   ModifyCustomRolePermissions
	}{
		{
			name: "role belongs to another organization",
			in: ModifyCustomRolePermissions{
				OrgID:              secondOrg.ID,
				ExternalID:         role.ExternalID,
				AddPermissionNames: []string{"custom-role:read"},
			},
		},
		{
			name: "unknown permission",
			in: ModifyCustomRolePermissions{
				OrgID:              firstOrg.ID,
				ExternalID:         role.ExternalID,
				AddPermissionNames: []string{"permission:unknown"},
			},
		},
		{
			name: "unknown permission to remove",
			in: ModifyCustomRolePermissions{
				OrgID:                 firstOrg.ID,
				ExternalID:            role.ExternalID,
				RemovePermissionNames: []string{"permission:unknown"},
			},
		},
		{
			name: "role not found",
			in: ModifyCustomRolePermissions{
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
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	org := seedOrg(t, orgStore, "delete-custom-role-org")
	project := seedProject(t, orgStore, org.ID, "project")
	user := seedUser(t, userStore, "delete-custom-role@test.com")
	role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})
	seedProjectRoleAssignment(t, ctx, pool, user.ID, role.Name, project.ID)
	seedOrgRoleAssignment(t, ctx, pool, user.ID, role.Name, org.ID)

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
	rbacStore := NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-delete-custom-role-org")
	secondOrg := seedOrg(t, orgStore, "second-delete-custom-role-org")
	role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})

	if err := rbacStore.DeleteCustomRole(ctx, secondOrg.ID, role.ExternalID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteCustomRole() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_CustomRoles(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-custom-role-org")
	secondOrg := seedOrg(t, orgStore, "second-custom-role-org")
	firstRole := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: firstOrg.ID, Name: "reader", PermissionNames: []string{"custom-role:read"}})
	seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: secondOrg.ID, Name: "other org role"})

	gotFirstPage, err := rbacStore.CustomRoles(ctx, firstOrg.ID, 50, 0)
	if err != nil {
		t.Fatalf("CustomRoles() error = %v", err)
	}

	wantFirstPage := []CustomRole{firstRole}

	testingx.AssertDiff(t, gotFirstPage, wantFirstPage)

	gotSecondPage, err := rbacStore.CustomRoles(ctx, firstOrg.ID, 50, 1)
	if err != nil {
		t.Fatalf("CustomRoles() second page error = %v", err)
	}

	wantSecondPage := []CustomRole{}

	testingx.AssertDiff(t, gotSecondPage, wantSecondPage)
}

func TestStore_CustomRoleCount(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	rbacStore := NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-custom-role-count-org")
	secondOrg := seedOrg(t, orgStore, "second-custom-role-count-org")
	seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})
	seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: firstOrg.ID, Name: "editor"})
	seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: secondOrg.ID, Name: "other org role"})

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
	rbacStore := NewStore(pool)

	org := seedOrg(t, orgStore, "custom-role-by-id-org")
	want := seedCustomRole(t, rbacStore, CreateCustomRole{
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
	rbacStore := NewStore(pool)

	firstOrg := seedOrg(t, orgStore, "first-role-lookup-org")
	secondOrg := seedOrg(t, orgStore, "second-role-lookup-org")
	role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: firstOrg.ID, Name: "reader"})

	if _, err := rbacStore.CustomRoleByExternalID(ctx, secondOrg.ID, role.ExternalID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("CustomRoleByExternalID() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_LockSystemRoleUser(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	rbacStore := NewStore(pool)

	usr, err := userStore.CreateUser(ctx, pguser.CreateUser{Email: "lock-system-role@test.com", Name: "Lock User"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if err := pgdb.NewTransactor(pool).RunTx(ctx, func(ctx context.Context) error {
		if err := rbacStore.LockSystemRoleUser(ctx, usr.ExternalID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("LockSystemRoleUser() error = %v", err)
	}
}

func TestStore_LockSystemRoleManagement(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	if err := pgdb.NewTransactor(pool).RunTx(ctx, func(ctx context.Context) error {
		if err := rbacStore.LockSystemRoleManagement(ctx); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("LockSystemRoleManagement() error = %v", err)
	}
}

func TestStore_SystemRoles(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	gotFirstPage, err := rbacStore.SystemRoles(ctx, 50, 0)
	if err != nil {
		t.Fatalf("SystemRoles() error = %v", err)
	}

	wantFirstPage := seededSystemRoles()

	testingx.AssertDiff(t, gotFirstPage, wantFirstPage)

	gotSecondPage, err := rbacStore.SystemRoles(ctx, 50, 1)
	if err != nil {
		t.Fatalf("SystemRoles() second page error = %v", err)
	}

	wantSecondPage := []SystemRole{}

	testingx.AssertDiff(t, gotSecondPage, wantSecondPage)
}

func TestStore_SystemRoleCount(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	got, err := rbacStore.SystemRoleCount(ctx)
	if err != nil {
		t.Fatalf("SystemRoleCount() error = %v", err)
	}

	if want := len(seededSystemRoles()); got != want {
		t.Errorf("SystemRoleCount() = %d, want %d", got, want)
	}
}

func TestStore_SystemRoleByName(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	got, err := rbacStore.SystemRoleByName(ctx, "superadmin")
	if err != nil {
		t.Fatalf("SystemRoleByName() error = %v", err)
	}

	want := seededSystemRole(t, "superadmin")

	testingx.AssertDiff(t, got, want)
}

func TestStore_SystemRoleByName_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)

	if _, err := rbacStore.SystemRoleByName(ctx, "nonexistent"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("SystemRoleByName() error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_UserSystemRolesByExternalID(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")

	gotBeforeAssignment, err := rbacStore.UserSystemRolesByExternalID(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRolesByExternalID() before assignment error = %v", err)
	}

	wantBeforeAssignment := []SystemRole{}

	testingx.AssertDiff(t, gotBeforeAssignment, wantBeforeAssignment)

	seedSystemRoleAssignment(t, rbacStore, usr.ExternalID, "superadmin")

	gotAfterAssignment, err := rbacStore.UserSystemRolesByExternalID(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRolesByExternalID() error = %v", err)
	}

	wantAfterAssignment := seededSystemRoles()

	testingx.AssertDiff(t, gotAfterAssignment, wantAfterAssignment)

	gotCount, err := rbacStore.UserSystemRoleCountByExternalID(ctx, usr.ExternalID)
	if err != nil {
		t.Fatalf("UserSystemRoleCountByExternalID() error = %v", err)
	}
	if wantCount := 1; gotCount != wantCount {
		t.Errorf("UserSystemRoleCountByExternalID() = %d, want %d", gotCount, wantCount)
	}
}

func TestStore_UserSystemRoleCountByExternalID_error(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)

		if _, err := rbacStore.UserSystemRoleCountByExternalID(ctx, uuid.New()); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UserSystemRoleCountByExternalID() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_UserSystemPermissionsByExternalID(t *testing.T) {
	t.Run("system scope", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedSystemRoleAssignment(t, rbacStore, usr.ExternalID, "superadmin")

		got, err := rbacStore.UserSystemPermissionsByExternalID(ctx, usr.ExternalID)
		if err != nil {
			t.Fatalf("UserSystemPermissionsByExternalID() error = %v", err)
		}

		want := seededSystemRole(t, "superadmin").PermissionNames

		testingx.AssertDiff(t, got, want)
	})

	t.Run("project and org scope do not leak into system scope", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)
		orgStore := pgorg.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, orgStore, "acme")
		project := seedProject(t, orgStore, org.ID, "acme-project")
		orgID, projectID := org.ID, project.ID
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "user-viewer", PermissionNames: []string{"user:read"}})
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", orgID)

		got, err := rbacStore.UserSystemPermissionsByExternalID(ctx, usr.ExternalID)
		if err != nil {
			t.Fatalf("UserSystemPermissionsByExternalID() error = %v", err)
		}

		// User was assigned a project role but not a system role.
		if len(got) != 0 {
			t.Errorf("UserSystemPermissionsByExternalID() = %v, want empty", got)
		}
	})
}

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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
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
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "user-viewer", PermissionNames: []string{"user:read"}})
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
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
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "user-viewer", PermissionNames: []string{"user:read"}})
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", orgID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
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
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "project-role", PermissionNames: []string{"user:read"}})
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "org-role", PermissionNames: []string{"user:read", "user:create"}})
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "project-role", projectID)
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "org-role", orgID)

		seedSystemRoleAssignment(t, rbacStore, usr.ExternalID, "superadmin")

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
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
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "project-role", PermissionNames: []string{"user:read"}})
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "org-role", PermissionNames: []string{"user:create"}})
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "project-role", projectID)
		seedOrgRoleAssignment(t, ctx, pool, usr.ID, "org-role", orgID)

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{
			OrgID:           orgID,
			PermissionNames: []string{"user:create", "user:read"},
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

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, projectID)
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
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "user-viewer", PermissionNames: []string{"user:read"}})
		seedProjectRoleAssignment(t, ctx, pool, usr.ID, "user-viewer", projectID)

		otherProject := seedProject(t, orgStore, orgID, "other")

		got, err := rbacStore.ProjectPermissions(ctx, usr.ID, otherProject.ID)
		if err != nil {
			t.Fatalf("ProjectPermissions() error = %v", err)
		}

		want := ProjectPermissions{OrgID: orgID, PermissionNames: []string{}}

		testingx.AssertDiff(t, got, want)
	})
}

func TestStore_ProjectPermissions_error(t *testing.T) {
	t.Run("project not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedSystemRoleAssignment(t, rbacStore, usr.ExternalID, "superadmin")

		if _, err := rbacStore.ProjectPermissions(ctx, usr.ID, 999999); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ProjectPermissions() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_AssignSystemRole_error(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)

		if err := rbacStore.AssignSystemRole(ctx, uuid.New(), "superadmin"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("role not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")

		if err := rbacStore.AssignSystemRole(ctx, usr.ExternalID, "nonexistent"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("name matches a custom role, not a system one", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, pgorg.NewStore(pool), "assign-system-role-custom-role-org")
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "some-project-role"})

		if err := rbacStore.AssignSystemRole(ctx, usr.ExternalID, "some-project-role"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("AssignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("role already assigned", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		seedSystemRoleAssignment(t, rbacStore, usr.ExternalID, "superadmin")

		if err := rbacStore.AssignSystemRole(ctx, usr.ExternalID, "superadmin"); !errors.Is(err, pgdb.ErrAlreadyExists) {
			t.Errorf("AssignSystemRole() error = %v, want pgdb.ErrAlreadyExists", err)
		}
	})
}

func TestStore_UnassignSystemRole(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	usr := seedUser(t, userStore, "alice@test.com")
	seedSystemRoleAssignment(t, rbacStore, usr.ExternalID, "superadmin")

	if err := rbacStore.UnassignSystemRole(ctx, usr.ExternalID, "superadmin"); err != nil {
		t.Fatalf("UnassignSystemRole() error = %v", err)
	}

	got, err := rbacStore.UserSystemRolesByExternalID(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRolesByExternalID() error = %v", err)
	}

	want := []SystemRole{}

	testingx.AssertDiff(t, got, want)
}

func TestStore_SystemPermissionsRemainAfterUnassign(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	rbacStore := NewStore(pool)
	userStore := pguser.NewStore(pool)

	perms := []string{"system-role:assign", "system-role:unassign"}

	manager := seedUser(t, userStore, "manager@test.com")
	otherManager := seedUser(t, userStore, "other-manager@test.com")

	seedSystemRoleAssignment(t, rbacStore, manager.ExternalID, "superadmin")

	got, err := rbacStore.SystemPermissionsRemainAfterUnassign(ctx, manager.ExternalID, "superadmin", perms)
	if err != nil {
		t.Fatalf("SystemPermissionsRemainAfterUnassign() before second assignment error = %v", err)
	}
	if got {
		t.Error("SystemPermissionsRemainAfterUnassign() before second assignment = true, want false")
	}

	seedSystemRoleAssignment(t, rbacStore, otherManager.ExternalID, "superadmin")

	got, err = rbacStore.SystemPermissionsRemainAfterUnassign(ctx, manager.ExternalID, "superadmin", perms)
	if err != nil {
		t.Fatalf("SystemPermissionsRemainAfterUnassign() after second assignment error = %v", err)
	}
	if !got {
		t.Error("SystemPermissionsRemainAfterUnassign() after second assignment = false, want true")
	}
}

func TestStore_SystemPermissionsRemainAfterUnassign_error(t *testing.T) {
	t.Run("assignment not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "no-assignment@test.com")

		if _, err := rbacStore.SystemPermissionsRemainAfterUnassign(ctx, usr.ExternalID, "superadmin", []string{"system-role:assign"}); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("SystemPermissionsRemainAfterUnassign() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_UnassignSystemRole_error(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)

		if err := rbacStore.UnassignSystemRole(ctx, uuid.New(), "superadmin"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("assignment not found", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")

		if err := rbacStore.UnassignSystemRole(ctx, usr.ExternalID, "superadmin"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("name matches a custom role, not a system one", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		rbacStore := NewStore(pool)
		userStore := pguser.NewStore(pool)

		usr := seedUser(t, userStore, "alice@test.com")
		org := seedOrg(t, pgorg.NewStore(pool), "unassign-system-role-custom-role-org")
		seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: org.ID, Name: "some-project-role"})

		if err := rbacStore.UnassignSystemRole(ctx, usr.ExternalID, "some-project-role"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UnassignSystemRole() error = %v, want sql.ErrNoRows", err)
		}
	})
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

func seedProjectRoleAssignment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int, roleName string, projectID int) {
	t.Helper()

	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName, "project_id": projectID}
	const q = `
		INSERT INTO rbac.project_role_assignments (user_id, role_id, project_id)
		SELECT @user_id, r.id, @project_id FROM rbac.custom_roles r WHERE r.name = @role_name`
	if _, err := pool.Exec(ctx, q, params); err != nil {
		t.Fatalf("seed project role assignment (user %d, role %q, project %d): %v", userID, roleName, projectID, err)
	}
}

func seedOrgRoleAssignment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int, roleName string, orgID int) {
	t.Helper()

	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName, "org_id": orgID}
	const q = `
		INSERT INTO rbac.org_role_assignments (user_id, role_id, org_id)
		SELECT @user_id, r.id, @org_id FROM rbac.custom_roles r WHERE r.name = @role_name`
	if _, err := pool.Exec(ctx, q, params); err != nil {
		t.Fatalf("seed org role assignment (user %d, role %q, org %d): %v", userID, roleName, orgID, err)
	}
}
