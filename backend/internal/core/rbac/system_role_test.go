package rbac

import (
	"context"
	"database/sql"
	"errors"
	"testing"

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

// TestCore_integration_systemRoleAssignmentLifecycle exercises listing, bootstrap assignment,
// normal assignment, and unassignment against the database.
func TestCore_integration_systemRoleAssignmentLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	core := NewCore(pgrbac.NewStore(pool), pgdb.NewTransactor(pool))

	diffOpts := cmp.Options{
		cmpopts.SortSlices(func(a, b mdl.Permission) bool { return a < b }),
	}

	// List the available system roles.

	systemRoles, totalCount, err := core.SystemRoles(ctx, 50, 0)
	if err != nil {
		t.Fatalf("SystemRoles() error = %v", err)
	}
	if wantCount := 1; totalCount != wantCount {
		t.Errorf("SystemRoles() total count = %d, want %d", totalCount, wantCount)
	}

	wantSystemRoles := seededSystemRoles()

	testingx.AssertDiff(t, systemRoles, wantSystemRoles, diffOpts)

	// Create the actor and target users, then bootstrap the actor.

	actor := seedUser(t, ctx, userStore, "admin@test.com", "Admin User")
	if err := core.BootstrapAssignSystemRole(ctx, actor.ExternalID, "superadmin"); err != nil {
		t.Fatalf("BootstrapAssignSystemRole() error = %v", err)
	}
	actorCtx := contextWithAuthUser(ctx, actor.ExternalID)

	usr := seedUser(t, ctx, userStore, "alice@test.com", "Alice Smith")

	// Assign a system role and list the user's assignments.

	if err := core.AssignSystemRole(actorCtx, usr.ExternalID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}

	gotAssignedRoles, gotAssignedCount, err := core.UserSystemRoles(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRoles() after assignment error = %v", err)
	}
	if wantCount := 1; gotAssignedCount != wantCount {
		t.Errorf("UserSystemRoles() after assignment total count = %d, want %d", gotAssignedCount, wantCount)
	}

	wantAssignedRoles := []mdl.SystemRole{seededSystemRole(t, "superadmin")}

	testingx.AssertDiff(t, gotAssignedRoles, wantAssignedRoles, diffOpts)

	// Unassign the system role and verify that the assignment was removed.

	if err := core.UnassignSystemRole(actorCtx, usr.ExternalID, "superadmin"); err != nil {
		t.Fatalf("UnassignSystemRole() error = %v", err)
	}

	gotUnassignedRoles, gotUnassignedCount, err := core.UserSystemRoles(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRoles() after unassignment error = %v", err)
	}
	if wantCount := 0; gotUnassignedCount != wantCount {
		t.Errorf("UserSystemRoles() after unassignment total count = %d, want %d", gotUnassignedCount, wantCount)
	}

	wantUnassignedRoles := []mdl.SystemRole{}

	testingx.AssertDiff(t, gotUnassignedRoles, wantUnassignedRoles)

	// Preserve the final fully privileged system administrator.

	if err := core.UnassignSystemRole(actorCtx, actor.ExternalID, "superadmin"); !errors.Is(err, mdl.ErrLastFullyPrivilegedSystemAdmin) {
		t.Errorf("UnassignSystemRole() last fully privileged system administrator error = %v, want mdl.ErrLastFullyPrivilegedSystemAdmin", err)
	}
}

// TestCore_integration_systemRoleChangesRequireSystemScope verifies that permissions granted
// through an org-scoped custom role cannot authorize system-role assignment changes.
func TestCore_integration_systemRoleChangesRequireSystemScope(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)
	core := NewCore(rbacStore, pgdb.NewTransactor(pool))

	actor := seedUser(t, ctx, userStore, "admin@test.com", "Admin User")
	assignTarget := seedUser(t, ctx, userStore, "assign@test.com", "Assign Target")
	unassignTarget := seedUser(t, ctx, userStore, "unassign@test.com", "Unassign Target")
	// Matching permission names at org scope must not authorize a system-scope role change.
	seedOrgScopedRoleWithAllPermissions(t, ctx, pool, orgStore, rbacStore, actor.ID)
	seedSystemRoleAssignment(t, ctx, pool, unassignTarget.ID, "superadmin")
	actorCtx := contextWithAuthUser(ctx, actor.ExternalID)

	if err := core.AssignSystemRole(actorCtx, assignTarget.ExternalID, "superadmin"); !errors.Is(err, mdl.ErrPermissionDenied) {
		t.Errorf("AssignSystemRole() error = %v, want mdl.ErrPermissionDenied", err)
	}
	if err := core.UnassignSystemRole(actorCtx, unassignTarget.ExternalID, "superadmin"); !errors.Is(err, mdl.ErrPermissionDenied) {
		t.Errorf("UnassignSystemRole() error = %v, want mdl.ErrPermissionDenied", err)
	}

	gotAssignRoles, gotAssignCount, err := core.UserSystemRoles(ctx, assignTarget.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRoles() assign target error = %v", err)
	}

	if wantCount := 0; gotAssignCount != wantCount {
		t.Errorf("UserSystemRoles() assign target total count = %d, want %d", gotAssignCount, wantCount)
	}

	testingx.AssertDiff(t, gotAssignRoles, []mdl.SystemRole{})

	gotUnassignRoles, gotUnassignCount, err := core.UserSystemRoles(ctx, unassignTarget.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRoles() unassign target error = %v", err)
	}
	if wantCount := 1; gotUnassignCount != wantCount {
		t.Errorf("UserSystemRoles() unassign target total count = %d, want %d", gotUnassignCount, wantCount)
	}

	wantUnassignRoles := []mdl.SystemRole{seededSystemRole(t, "superadmin")}

	testingx.AssertDiff(t, gotUnassignRoles, wantUnassignRoles, cmp.Options{
		cmpopts.SortSlices(func(a, b mdl.Permission) bool { return a < b }),
	})
}

// TestCore_integration_customRoleLifecycle exercises custom-role validation and persistence
// through the core and PostgreSQL store layers.
func TestCore_SystemRoles(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		SystemRolesFunc: func(_ context.Context, _, _ int) ([]pgrbac.SystemRole, int, error) {
			return []pgrbac.SystemRole{
				{Name: "superadmin", PermissionNames: []string{"user:create", "user:read"}},
			}, 7, nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})

	got, count, err := core.SystemRoles(t.Context(), 25, 5)
	if err != nil {
		t.Fatalf("SystemRoles() error = %v", err)
	}
	if count != 7 {
		t.Errorf("SystemRoles() total count = %d, want 7", count)
	}

	want := []mdl.SystemRole{
		{Name: "superadmin", Permissions: []mdl.Permission{mdl.PermissionUserCreate, mdl.PermissionUserRead}},
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_SystemRoles_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
	}{
		{
			name: "system roles store error",
			roleStorer: &MockedRoleStorer{
				SystemRolesFunc: func(_ context.Context, _, _ int) ([]pgrbac.SystemRole, int, error) {
					return nil, 0, dbErr
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if _, _, err := core.SystemRoles(t.Context(), 25, 0); !errors.Is(err, dbErr) {
				t.Errorf("SystemRoles() error = %v, want %v", err, dbErr)
			}
		})
	}
}

func TestCore_UserSystemRoles(t *testing.T) {
	userID := uuid.New()
	roleStorer := &MockedRoleStorer{
		UserSystemRolesByExternalIDFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]pgrbac.SystemRole, int, error) {
			return []pgrbac.SystemRole{
				{Name: "test-role", PermissionNames: []string{"user:read"}},
			}, 1, nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})

	got, count, err := core.UserSystemRoles(t.Context(), userID, 25, 5)
	if err != nil {
		t.Fatalf("UserSystemRoles() error = %v", err)
	}
	if wantCount := 1; count != wantCount {
		t.Errorf("UserSystemRoles() total count = %d, want %d", count, wantCount)
	}

	want := []mdl.SystemRole{
		{Name: "test-role", Permissions: []mdl.Permission{mdl.PermissionUserRead}},
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_UserSystemRoles_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "user not found",
			roleStorer: &MockedRoleStorer{
				UserSystemRolesByExternalIDFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]pgrbac.SystemRole, int, error) {
					return nil, 0, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "user roles store error",
			roleStorer: &MockedRoleStorer{
				UserSystemRolesByExternalIDFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]pgrbac.SystemRole, int, error) {
					return nil, 0, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if _, _, err := core.UserSystemRoles(t.Context(), uuid.New(), 25, 0); !errors.Is(err, tt.want) {
				t.Errorf("UserSystemRoles() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_BootstrapAssignSystemRole(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
		AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
			return nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})

	if err := core.BootstrapAssignSystemRole(t.Context(), uuid.New(), "superadmin"); err != nil {
		t.Errorf("BootstrapAssignSystemRole() error = %v, want nil", err)
	}
}

func TestCore_BootstrapAssignSystemRole_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "user lock",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
					return dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "user or role not found",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
					return nil
				},
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "already assigned",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
					return nil
				},
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "assignment store",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
					return nil
				},
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if err := core.BootstrapAssignSystemRole(t.Context(), uuid.New(), "superadmin"); !errors.Is(err, tt.want) {
				t.Errorf("BootstrapAssignSystemRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_AssignSystemRole(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
		SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
			return pgrbac.SystemRole{Name: "test-role", PermissionNames: []string{"user:read"}}, nil
		},
		SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
			return []string{"user:read", "user:update"}, nil
		},
		AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
			return nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := contextWithAuthUser(t.Context(), uuid.New())

	if err := core.AssignSystemRole(ctx, uuid.New(), "test-role"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}
}

func TestCore_AssignSystemRole_error(t *testing.T) {
	dbErr := errors.New("db error")

	t.Run("auth session missing", func(t *testing.T) {
		core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

		if err := core.AssignSystemRole(t.Context(), uuid.New(), "test-role"); err == nil {
			t.Error("AssignSystemRole() error = nil, want error")
		}
	})

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "user lock store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
					return dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "target not found",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, nil
				},
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "role lookup store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "actor permission subset",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{PermissionNames: []string{"user:read", "user:update"}}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return []string{"user:read"}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
		},
		{
			name: "role already assigned",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, nil
				},
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "actor not found",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "actor permissions store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role assignment store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, nil
				},
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := contextWithAuthUser(t.Context(), uuid.New())

			if err := core.AssignSystemRole(ctx, uuid.New(), "test-role"); !errors.Is(err, tt.want) {
				t.Errorf("AssignSystemRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_UnassignSystemRole(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		LockSystemRoleManagementFunc: func(_ context.Context) error {
			return nil
		},
		LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
		SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
			return pgrbac.SystemRole{Name: "test-role", PermissionNames: []string{"user:read"}}, nil
		},
		SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
			return []string{"user:read", "user:update"}, nil
		},
		FullyPrivilegedUserRemainsAfterSystemRoleUnassignFunc: func(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
			return true, nil
		},
		UnassignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
			return nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := contextWithAuthUser(t.Context(), uuid.New())

	if err := core.UnassignSystemRole(ctx, uuid.New(), "test-role"); err != nil {
		t.Fatalf("UnassignSystemRole() error = %v", err)
	}
}

func TestCore_UnassignSystemRole_error(t *testing.T) {
	dbErr := errors.New("db error")

	t.Run("auth session missing", func(t *testing.T) {
		core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

		if err := core.UnassignSystemRole(t.Context(), uuid.New(), "test-role"); err == nil {
			t.Error("UnassignSystemRole() error = nil, want error")
		}
	})

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "management lock store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error {
					return dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "user lock store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
					return dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "last fully privileged system administrator",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{
						PermissionNames: []string{"system-role:assign", "system-role:unassign"},
					}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return []string{"system-role:assign", "system-role:unassign"}, nil
				},
				FullyPrivilegedUserRemainsAfterSystemRoleUnassignFunc: func(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
					return false, nil
				},
			},
			want: mdl.ErrLastFullyPrivilegedSystemAdmin,
		},
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "role lookup store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "assignment not found",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, nil
				},
				FullyPrivilegedUserRemainsAfterSystemRoleUnassignFunc: func(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
					return false, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "actor permission subset",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{PermissionNames: []string{"user:read", "user:update"}}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return []string{"user:read"}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
		},
		{
			name: "actor not found",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "actor permissions store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "fully privileged user check store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{PermissionNames: []string{"system-role:assign"}}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return []string{"system-role:assign"}, nil
				},
				FullyPrivilegedUserRemainsAfterSystemRoleUnassignFunc: func(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
					return false, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role unassignment store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				SystemPermissionsFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, nil
				},
				FullyPrivilegedUserRemainsAfterSystemRoleUnassignFunc: func(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
					return true, nil
				},
				UnassignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := contextWithAuthUser(t.Context(), uuid.New())

			if err := core.UnassignSystemRole(ctx, uuid.New(), "test-role"); !errors.Is(err, tt.want) {
				t.Errorf("UnassignSystemRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}
