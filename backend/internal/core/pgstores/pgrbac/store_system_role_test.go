package pgrbac

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

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
		seedOrgMembership(t, ctx, pool, usr.ID, orgID)
		role := seedCustomRole(t, rbacStore, CreateCustomRole{OrgID: orgID, Name: "user-viewer", PermissionNames: []string{"user:read"}})
		seedProjectRoleAssignment(t, ctx, rbacStore, usr.ExternalID, role.ExternalID, projectID)
		seedOrgRoleAssignment(t, ctx, rbacStore, usr.ExternalID, role.ExternalID, orgID)

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
