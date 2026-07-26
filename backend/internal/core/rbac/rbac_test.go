package rbac

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
	if wantAssignedCount := 1; gotAssignedCount != wantAssignedCount {
		t.Errorf("UserSystemRoles() after assignment total count = %d, want %d", gotAssignedCount, wantAssignedCount)
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
	if wantUnassignedCount := 0; gotUnassignedCount != wantUnassignedCount {
		t.Errorf("UserSystemRoles() after unassignment total count = %d, want %d", gotUnassignedCount, wantUnassignedCount)
	}

	wantUnassignedRoles := []mdl.SystemRole{}

	testingx.AssertDiff(t, gotUnassignedRoles, wantUnassignedRoles)

	// Preserve the final system-role management assignment.

	if err := core.UnassignSystemRole(actorCtx, actor.ExternalID, "superadmin"); !errors.Is(err, mdl.ErrLastRoleManager) {
		t.Errorf("UnassignSystemRole() last manager error = %v, want mdl.ErrLastRoleManager", err)
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

	if wantAssignCount := 0; gotAssignCount != wantAssignCount {
		t.Errorf("UserSystemRoles() assign target total count = %d, want %d", gotAssignCount, wantAssignCount)
	}

	testingx.AssertDiff(t, gotAssignRoles, []mdl.SystemRole{})

	gotUnassignRoles, gotUnassignCount, err := core.UserSystemRoles(ctx, unassignTarget.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRoles() unassign target error = %v", err)
	}
	if wantUnassignCount := 1; gotUnassignCount != wantUnassignCount {
		t.Errorf("UserSystemRoles() unassign target total count = %d, want %d", gotUnassignCount, wantUnassignCount)
	}

	wantUnassignRoles := []mdl.SystemRole{seededSystemRole(t, "superadmin")}

	testingx.AssertDiff(t, gotUnassignRoles, wantUnassignRoles, cmp.Options{
		cmpopts.SortSlices(func(a, b mdl.Permission) bool { return a < b }),
	})
}

// TestCore_integration_customRoleLifecycle exercises custom-role validation and persistence
// through the core and PostgreSQL store layers.
func TestCore_integration_customRoleLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	roleStore := pgrbac.NewStore(pool)
	core := NewCore(roleStore, pgdb.NewTransactor(pool))

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(mdl.CustomRole{}, "ID", "CreatedAt", "UpdatedAt", "ETag"),
	}

	org := seedOrg(t, orgStore, "custom-role-integration-org")
	roleCtx := mdl.ContextWithAuthSession(ctx, mdl.AuthSession{OrgID: &org.ID})

	// Create roles with organization-scoped permissions.

	firstRole, err := core.CreateCustomRole(roleCtx, mdl.CreateCustomRole{
		Name:        "first role",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleCreate},
	})
	if err != nil {
		t.Fatalf("CreateCustomRole() error = %v", err)
	}

	testingx.AssertDiff(t, firstRole, mdl.CustomRole{
		Name:        "first role",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleCreate},
	}, diffOpts)

	secondRole, err := core.CreateCustomRole(roleCtx, mdl.CreateCustomRole{
		Name:        "second role",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
	})
	if err != nil {
		t.Fatalf("CreateCustomRole() second role error = %v", err)
	}

	// Fetch the second created role by ID.

	secondRoleFetched, err := core.CustomRoleByID(roleCtx, secondRole.ID)
	if err != nil {
		t.Fatalf("CustomRoleByID() error = %v", err)
	}

	testingx.AssertDiff(t, secondRoleFetched, secondRole)

	// Update the first created role's name and replace its permission set.

	firstRoleUpdated, err := core.UpdateCustomRole(roleCtx, mdl.UpdateCustomRole{
		ID: firstRole.ID,
		Fields: mdl.CustomRoleUpdateFields{
			Name:        true,
			Permissions: true,
		},
		Name:        "first role updated",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleUpdate},
	})
	if err != nil {
		t.Fatalf("UpdateCustomRole() error = %v", err)
	}

	testingx.AssertDiff(t, firstRoleUpdated, mdl.CustomRole{
		Name:        "first role updated",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleUpdate},
	}, diffOpts)

	// Modify the first role's permission set incrementally.

	firstRoleModified, err := core.ModifyCustomRolePermissions(roleCtx, mdl.ModifyCustomRolePermissions{
		ID:             firstRole.ID,
		AddPermissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
	})
	if err != nil {
		t.Fatalf("ModifyCustomRolePermissions() error = %v", err)
	}

	testingx.AssertDiff(t, firstRoleModified, mdl.CustomRole{
		Name: "first role updated",
		Permissions: []mdl.Permission{
			mdl.PermissionCustomRoleRead,
			mdl.PermissionCustomRoleUpdate,
		},
	}, diffOpts)

	// Delete the second role.

	if err := core.DeleteCustomRole(roleCtx, secondRole.ID); err != nil {
		t.Fatalf("DeleteCustomRole() error = %v", err)
	}

	// List the organization's roles after deleting the role under test.

	gotRoles, gotCount, err := core.CustomRoles(roleCtx, 50, 0)
	if err != nil {
		t.Fatalf("CustomRoles() after deletion error = %v", err)
	}

	if wantCount := 1; gotCount != wantCount {
		t.Errorf("CustomRoles() after deletion total count = %d, want %d", gotCount, wantCount)
	}

	testingx.AssertDiff(t, gotRoles, []mdl.CustomRole{firstRoleModified}, diffOpts)
}

// TestCore_integration_customRoleAssignmentLifecycle exercises project- and organization-scoped
// custom-role assignments through the core and PostgreSQL store layers.
func TestCore_integration_customRoleAssignmentLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	roleStore := pgrbac.NewStore(pool)
	core := NewCore(roleStore, pgdb.NewTransactor(pool))

	org := seedOrg(t, orgStore, "custom-role-assignment-integration-org")
	firstProject := seedProject(t, ctx, orgStore, org.ID, "first")
	secondProject := seedProject(t, ctx, orgStore, org.ID, "second")
	user := seedUser(t, ctx, userStore, "custom-role-assignment@test.com", "Target User")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	role := seedCustomRole(t, ctx, roleStore, org.ID, "reader", []mdl.Permission{mdl.PermissionCustomRoleRead})
	roleCtx := mdl.ContextWithAuthSession(ctx, mdl.AuthSession{ProjectID: &firstProject.ID, OrgID: &org.ID})

	assertProjectPermNames := func(projectID int, want []string) {
		t.Helper()

		projectPerms, err := roleStore.ProjectPermissions(ctx, user.ID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions(%d) error = %v", projectID, err)
		}

		testingx.AssertDiff(t, projectPerms.PermissionNames, want, cmpopts.EquateEmpty())
	}

	// Assign and unassign the role in one project.

	if err := core.AssignCustomRoleToProject(roleCtx, user.ExternalID, role.ExternalID); err != nil {
		t.Fatalf("AssignCustomRoleToProject() error = %v", err)
	}

	assertProjectPermNames(firstProject.ID, []string{"custom-role:read"})

	if err := core.UnassignCustomRoleFromProject(roleCtx, user.ExternalID, role.ExternalID); err != nil {
		t.Fatalf("UnassignCustomRoleFromProject() error = %v", err)
	}

	assertProjectPermNames(firstProject.ID, nil)

	// Assign and unassign the role across the organization.

	if err := core.AssignCustomRoleToOrg(roleCtx, user.ExternalID, role.ExternalID); err != nil {
		t.Fatalf("AssignCustomRoleToOrg() error = %v", err)
	}

	assertProjectPermNames(firstProject.ID, []string{"custom-role:read"})
	assertProjectPermNames(secondProject.ID, []string{"custom-role:read"})

	if err := core.UnassignCustomRoleFromOrg(roleCtx, user.ExternalID, role.ExternalID); err != nil {
		t.Fatalf("UnassignCustomRoleFromOrg() error = %v", err)
	}

	assertProjectPermNames(firstProject.ID, nil)
	assertProjectPermNames(secondProject.ID, nil)
}

func TestCore_CustomRoles(t *testing.T) {
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
	mockOutput := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		CustomRolesFunc: func(_ context.Context, _, _, _ int) ([]pgrbac.CustomRole, error) {
			return []pgrbac.CustomRole{mockOutput}, nil
		},
		CustomRoleCountFunc: func(_ context.Context, _ int) (int, error) {
			return 3, nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})

	got, count, err := core.CustomRoles(ctx, 25, 5)
	if err != nil {
		t.Fatalf("CustomRoles() error = %v", err)
	}

	if wantCount := 3; count != wantCount {
		t.Errorf("CustomRoles() total count = %d, want %d", count, wantCount)
	}

	testingx.AssertDiff(t, got, []mdl.CustomRole{
		{
			ID:          mockOutput.ExternalID,
			Name:        mockOutput.Name,
			Permissions: permissionsFromPg(mockOutput.PermissionNames),
			CreatedAt:   mockOutput.CreatedAt,
			UpdatedAt:   mockOutput.UpdatedAt,
			ETag:        mockOutput.ETag.String(),
		},
	})
}

func TestCore_CustomRoles_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "roles store error",
			roleStorer: &MockedRoleStorer{
				CustomRolesFunc: func(_ context.Context, _, _, _ int) ([]pgrbac.CustomRole, error) {
					return nil, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role count store error",
			roleStorer: &MockedRoleStorer{
				CustomRolesFunc: func(_ context.Context, _, _, _ int) ([]pgrbac.CustomRole, error) {
					return nil, nil
				},
				CustomRoleCountFunc: func(_ context.Context, _ int) (int, error) {
					return 0, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if _, _, err := core.CustomRoles(ctx, 25, 0); !errors.Is(err, tt.want) {
				t.Errorf("CustomRoles() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "organization context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if _, _, err := core.CustomRoles(tt.ctx, 25, 0); err == nil {
					t.Error("CustomRoles() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_CustomRoleByID(t *testing.T) {
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
	mockOutput := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
			return mockOutput, nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})

	got, err := core.CustomRoleByID(ctx, mockOutput.ExternalID)
	if err != nil {
		t.Fatalf("CustomRoleByID() error = %v", err)
	}

	testingx.AssertDiff(t, got, mdl.CustomRole{
		ID:          mockOutput.ExternalID,
		Name:        mockOutput.Name,
		Permissions: permissionsFromPg(mockOutput.PermissionNames),
		CreatedAt:   mockOutput.CreatedAt,
		UpdatedAt:   mockOutput.UpdatedAt,
		ETag:        mockOutput.ETag.String(),
	})
}

func TestCore_CustomRoleByID_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "role store error",
			roleStorer: &MockedRoleStorer{
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if _, err := core.CustomRoleByID(ctx, uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("CustomRoleByID() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "organization context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if _, err := core.CustomRoleByID(tt.ctx, uuid.New()); err == nil {
					t.Error("CustomRoleByID() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_SystemRoles(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		SystemRolesFunc: func(_ context.Context, _, _ int) ([]pgrbac.SystemRole, error) {
			return []pgrbac.SystemRole{
				{Name: "superadmin", PermissionNames: []string{"user:create", "user:read"}},
			}, nil
		},
		SystemRoleCountFunc: func(_ context.Context) (int, error) {
			return 7, nil
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
				SystemRolesFunc: func(_ context.Context, _, _ int) ([]pgrbac.SystemRole, error) {
					return nil, dbErr
				},
			},
		},
		{
			name: "system role count store error",
			roleStorer: &MockedRoleStorer{
				SystemRolesFunc: func(_ context.Context, _, _ int) ([]pgrbac.SystemRole, error) {
					return nil, nil
				},
				SystemRoleCountFunc: func(_ context.Context) (int, error) {
					return 0, dbErr
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
		UserSystemRolesByExternalIDFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]pgrbac.SystemRole, error) {
			return []pgrbac.SystemRole{
				{Name: "test-role", PermissionNames: []string{"user:read"}},
			}, nil
		},
		UserSystemRoleCountByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (int, error) {
			return 1, nil
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
				UserSystemRolesByExternalIDFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]pgrbac.SystemRole, error) {
					return nil, nil
				},
				UserSystemRoleCountByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (int, error) {
					return 0, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "user roles store error",
			roleStorer: &MockedRoleStorer{
				UserSystemRolesByExternalIDFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]pgrbac.SystemRole, error) {
					return nil, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "user role count store error",
			roleStorer: &MockedRoleStorer{
				UserSystemRolesByExternalIDFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]pgrbac.SystemRole, error) {
					return nil, nil
				},
				UserSystemRoleCountByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (int, error) {
					return 0, dbErr
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

func TestCore_CreateCustomRole(t *testing.T) {
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
	mockOutput := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		CreateCustomRoleFunc: func(_ context.Context, _ pgrbac.CreateCustomRole) (pgrbac.CustomRole, error) {
			return mockOutput, nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})

	got, err := core.CreateCustomRole(ctx, mdl.CreateCustomRole{
		Name:        "project manager",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
	})
	if err != nil {
		t.Fatalf("CreateCustomRole() error = %v", err)
	}

	want := mdl.CustomRole{
		ID:          mockOutput.ExternalID,
		Name:        mockOutput.Name,
		Permissions: permissionsFromPg(mockOutput.PermissionNames),
		CreatedAt:   mockOutput.CreatedAt,
		UpdatedAt:   mockOutput.UpdatedAt,
		ETag:        mockOutput.ETag.String(),
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_CreateCustomRole_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		in         mdl.CreateCustomRole
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "model validation error",
			in: mdl.CreateCustomRole{
				Name:        "project manager",
				Permissions: []mdl.Permission{mdl.PermissionUserRead},
			},
			roleStorer: &MockedRoleStorer{},
			want:       mdl.ErrValidation,
		},
		{
			name: "role already exists",
			in:   mdl.CreateCustomRole{Name: "project manager"},
			roleStorer: &MockedRoleStorer{
				CreateCustomRoleFunc: func(_ context.Context, _ pgrbac.CreateCustomRole) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "name constraint violated",
			in:   mdl.CreateCustomRole{Name: "project manager"},
			roleStorer: &MockedRoleStorer{
				CreateCustomRoleFunc: func(_ context.Context, _ pgrbac.CreateCustomRole) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, pgdb.ErrCheckConstraintViolated
				},
			},
			want: mdl.ErrValidation,
		},
		{
			name: "organization not found",
			in:   mdl.CreateCustomRole{Name: "project manager"},
			roleStorer: &MockedRoleStorer{
				CreateCustomRoleFunc: func(_ context.Context, _ pgrbac.CreateCustomRole) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "store error",
			in:   mdl.CreateCustomRole{Name: "project manager"},
			roleStorer: &MockedRoleStorer{
				CreateCustomRoleFunc: func(_ context.Context, _ pgrbac.CreateCustomRole) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if _, err := core.CreateCustomRole(ctx, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("CreateCustomRole() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "organization context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if _, err := core.CreateCustomRole(tt.ctx, mdl.CreateCustomRole{Name: "project manager"}); err == nil {
					t.Error("CreateCustomRole() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_UpdateCustomRole(t *testing.T) {
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
	mockOutput := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		UpdateCustomRoleFunc: func(ctx context.Context, ur pgrbac.UpdateCustomRole) (pgrbac.CustomRole, error) {
			return mockOutput, nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})

	got, err := core.UpdateCustomRole(ctx, mdl.UpdateCustomRole{
		ID: uuid.New(),
		Fields: mdl.CustomRoleUpdateFields{
			Name:        true,
			Permissions: true,
		},
		Name:        "project lead",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleCreate},
	})
	if err != nil {
		t.Fatalf("UpdateCustomRole() error = %v", err)
	}

	want := mdl.CustomRole{
		ID:          mockOutput.ExternalID,
		Name:        mockOutput.Name,
		Permissions: permissionsFromPg(mockOutput.PermissionNames),
		CreatedAt:   mockOutput.CreatedAt,
		UpdatedAt:   mockOutput.UpdatedAt,
		ETag:        mockOutput.ETag.String(),
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_UpdateCustomRole_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		in         mdl.UpdateCustomRole
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "model validation error",
			in: mdl.UpdateCustomRole{
				Fields:      mdl.CustomRoleUpdateFields{Permissions: true},
				Permissions: []mdl.Permission{mdl.PermissionSystemRoleRead},
			},
			roleStorer: &MockedRoleStorer{},
			want:       mdl.ErrValidation,
		},
		{
			name: "role not found",
			in:   mdl.UpdateCustomRole{},
			roleStorer: &MockedRoleStorer{
				UpdateCustomRoleFunc: func(_ context.Context, _ pgrbac.UpdateCustomRole) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "store error",
			in:   mdl.UpdateCustomRole{},
			roleStorer: &MockedRoleStorer{
				UpdateCustomRoleFunc: func(_ context.Context, _ pgrbac.UpdateCustomRole) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if _, err := core.UpdateCustomRole(ctx, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("UpdateCustomRole() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "organization context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if _, err := core.UpdateCustomRole(tt.ctx, mdl.UpdateCustomRole{}); err == nil {
					t.Error("UpdateCustomRole() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_ModifyCustomRolePermissions(t *testing.T) {
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
	mockOutput := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		ModifyCustomRolePermissionsFunc: func(ctx context.Context, mp pgrbac.ModifyCustomRolePermissions) (pgrbac.CustomRole, error) {
			return mockOutput, nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})

	got, err := core.ModifyCustomRolePermissions(ctx, mdl.ModifyCustomRolePermissions{
		ID:                uuid.New(),
		AddPermissions:    []mdl.Permission{mdl.PermissionCustomRoleRead},
		RemovePermissions: []mdl.Permission{mdl.PermissionCustomRoleDelete},
	})
	if err != nil {
		t.Fatalf("ModifyCustomRolePermissions() error = %v", err)
	}

	want := mdl.CustomRole{
		ID:          mockOutput.ExternalID,
		Name:        mockOutput.Name,
		Permissions: permissionsFromPg(mockOutput.PermissionNames),
		CreatedAt:   mockOutput.CreatedAt,
		UpdatedAt:   mockOutput.UpdatedAt,
		ETag:        mockOutput.ETag.String(),
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_ModifyCustomRolePermissions_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		in         mdl.ModifyCustomRolePermissions
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "model validation error",
			in: mdl.ModifyCustomRolePermissions{
				AddPermissions: []mdl.Permission{mdl.PermissionUserCreate},
			},
			roleStorer: &MockedRoleStorer{},
			want:       mdl.ErrValidation,
		},
		{
			name: "role not found",
			in:   mdl.ModifyCustomRolePermissions{},
			roleStorer: &MockedRoleStorer{
				ModifyCustomRolePermissionsFunc: func(_ context.Context, _ pgrbac.ModifyCustomRolePermissions) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "store error",
			in:   mdl.ModifyCustomRolePermissions{},
			roleStorer: &MockedRoleStorer{
				ModifyCustomRolePermissionsFunc: func(_ context.Context, _ pgrbac.ModifyCustomRolePermissions) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if _, err := core.ModifyCustomRolePermissions(ctx, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("ModifyCustomRolePermissions() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "organization context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if _, err := core.ModifyCustomRolePermissions(tt.ctx, mdl.ModifyCustomRolePermissions{}); err == nil {
					t.Error("ModifyCustomRolePermissions() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_DeleteCustomRole(t *testing.T) {
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
	roleStorer := &MockedRoleStorer{
		DeleteCustomRoleFunc: func(_ context.Context, _ int, _ uuid.UUID) error {
			return nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})

	if err := core.DeleteCustomRole(ctx, uuid.New()); err != nil {
		t.Errorf("DeleteCustomRole() error = %v", err)
	}
}

func TestCore_DeleteCustomRole_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				DeleteCustomRoleFunc: func(_ context.Context, _ int, _ uuid.UUID) error {
					return sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "store error",
			roleStorer: &MockedRoleStorer{
				DeleteCustomRoleFunc: func(_ context.Context, _ int, _ uuid.UUID) error {
					return dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if err := core.DeleteCustomRole(ctx, uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("DeleteCustomRole() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "organization context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if err := core.DeleteCustomRole(tt.ctx, uuid.New()); err == nil {
					t.Error("DeleteCustomRole() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_AssignCustomRoleToProject(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error { return nil },
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{ProjectID: new(42)})

	if err := core.AssignCustomRoleToProject(ctx, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("AssignCustomRoleToProject() error = %v", err)
	}
}

func TestCore_AssignCustomRoleToProject_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "not found",
			roleStorer: &MockedRoleStorer{AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return sql.ErrNoRows
			}},
			want: mdl.ErrNotFound,
		},
		{
			name: "already assigned",
			roleStorer: &MockedRoleStorer{AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return pgdb.ErrAlreadyExists
			}},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "store",
			roleStorer: &MockedRoleStorer{AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return dbErr
			}},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{ProjectID: new(42)})

			if err := core.AssignCustomRoleToProject(ctx, uuid.New(), uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("AssignCustomRoleToProject() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "project context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if err := core.AssignCustomRoleToProject(tt.ctx, uuid.New(), uuid.New()); err == nil {
					t.Error("AssignCustomRoleToProject() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_UnassignCustomRoleFromProject(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		UnassignCustomRoleFromProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error { return nil },
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{ProjectID: new(42)})

	if err := core.UnassignCustomRoleFromProject(ctx, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("UnassignCustomRoleFromProject() error = %v", err)
	}
}

func TestCore_UnassignCustomRoleFromProject_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "not found",
			roleStorer: &MockedRoleStorer{UnassignCustomRoleFromProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return sql.ErrNoRows
			}},
			want: mdl.ErrNotFound,
		},
		{
			name: "store",
			roleStorer: &MockedRoleStorer{UnassignCustomRoleFromProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return dbErr
			}},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{ProjectID: new(42)})

			if err := core.UnassignCustomRoleFromProject(ctx, uuid.New(), uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("UnassignCustomRoleFromProject() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "project context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if err := core.UnassignCustomRoleFromProject(tt.ctx, uuid.New(), uuid.New()); err == nil {
					t.Error("UnassignCustomRoleFromProject() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_AssignCustomRoleToOrg(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error { return nil },
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})

	if err := core.AssignCustomRoleToOrg(ctx, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("AssignCustomRoleToOrg() error = %v", err)
	}
}

func TestCore_AssignCustomRoleToOrg_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "not found",
			roleStorer: &MockedRoleStorer{AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return sql.ErrNoRows
			}},
			want: mdl.ErrNotFound,
		},
		{
			name: "already assigned",
			roleStorer: &MockedRoleStorer{AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return pgdb.ErrAlreadyExists
			}},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "store",
			roleStorer: &MockedRoleStorer{AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return dbErr
			}},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})

			if err := core.AssignCustomRoleToOrg(ctx, uuid.New(), uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("AssignCustomRoleToOrg() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "organization context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if err := core.AssignCustomRoleToOrg(tt.ctx, uuid.New(), uuid.New()); err == nil {
					t.Error("AssignCustomRoleToOrg() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_UnassignCustomRoleFromOrg(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		UnassignCustomRoleFromOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error { return nil },
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})

	if err := core.UnassignCustomRoleFromOrg(ctx, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("UnassignCustomRoleFromOrg() error = %v", err)
	}
}

func TestCore_UnassignCustomRoleFromOrg_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "not found",
			roleStorer: &MockedRoleStorer{UnassignCustomRoleFromOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return sql.ErrNoRows
			}},
			want: mdl.ErrNotFound,
		},
		{
			name: "store",
			roleStorer: &MockedRoleStorer{UnassignCustomRoleFromOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
				return dbErr
			}},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})

			if err := core.UnassignCustomRoleFromOrg(ctx, uuid.New(), uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("UnassignCustomRoleFromOrg() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			ctx  context.Context //nolint:containedctx // table test, each case supplies its own fixed ctx.
		}{
			{
				name: "auth session missing",
				ctx:  context.Background(),
			},
			{
				name: "organization context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if err := core.UnassignCustomRoleFromOrg(tt.ctx, uuid.New(), uuid.New()); err == nil {
					t.Error("UnassignCustomRoleFromOrg() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_AssignSystemRole(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
		SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
			return pgrbac.SystemRole{Name: "test-role", PermissionNames: []string{"user:read"}}, nil
		},
		UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
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
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
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
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
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
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, nil
				},
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "actor permissions store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
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
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
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
		UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
			return []string{"user:read", "user:update"}, nil
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
			name: "last role manager",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{
						PermissionNames: []string{"system-role:assign", "system-role:unassign"},
					}, nil
				},
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return []string{"system-role:assign", "system-role:unassign"}, nil
				},
				SystemPermissionsRemainAfterUnassignFunc: func(_ context.Context, _ uuid.UUID, _ string, _ []string) (bool, error) {
					return false, nil
				},
			},
			want: mdl.ErrLastRoleManager,
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
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, nil
				},
				UnassignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return sql.ErrNoRows
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
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return []string{"user:read"}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
		},
		{
			name: "actor permissions store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{}, nil
				},
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "permissions remain store error",
			roleStorer: &MockedRoleStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				LockSystemRoleUserFunc:       func(_ context.Context, _ uuid.UUID) error { return nil },
				SystemRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.SystemRole, error) {
					return pgrbac.SystemRole{PermissionNames: []string{"system-role:assign"}}, nil
				},
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return []string{"system-role:assign"}, nil
				},
				SystemPermissionsRemainAfterUnassignFunc: func(_ context.Context, _ uuid.UUID, _ string, _ []string) (bool, error) {
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
				UserSystemPermissionsByExternalIDFunc: func(_ context.Context, _ uuid.UUID) ([]string, error) {
					return nil, nil
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

type immediateTransactor struct{}

func (immediateTransactor) RunTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func contextWithAuthUser(ctx context.Context, userID uuid.UUID) context.Context {
	return mdl.ContextWithAuthSession(ctx, mdl.AuthSession{User: mdl.AuthUser{UserID: userID}})
}

func seededSystemRoles() []mdl.SystemRole {
	return []mdl.SystemRole{
		{Name: "superadmin", Permissions: mdl.AllPermissions()},
	}
}

func seededSystemRole(t *testing.T, name string) mdl.SystemRole {
	t.Helper()

	roles := seededSystemRoles()

	roleIdx := slices.IndexFunc(roles, func(role mdl.SystemRole) bool { return role.Name == name })
	if roleIdx == -1 {
		t.Fatalf("slices.IndexFunc(seededSystemRoles(), %q) = -1, want an index", name)
	}

	return roles[roleIdx]
}

func seedUser(t *testing.T, ctx context.Context, store *pguser.Store, email, name string) pguser.User {
	t.Helper()

	user, err := store.CreateUser(ctx, pguser.CreateUser{Email: email, Name: name})
	if err != nil {
		t.Fatalf("seed user %q: %v", email, err)
	}

	return user
}

func seedSystemRoleAssignment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int, roleName string) {
	t.Helper()

	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName}
	const query = `
		INSERT INTO rbac.system_role_assignments (user_id, role_id)
		SELECT @user_id, id
		FROM rbac.system_roles
		WHERE name = @role_name`
	if _, err := pool.Exec(ctx, query, params); err != nil {
		t.Fatalf("seed system role assignment for user %d: %v", userID, err)
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

func seedProject(t *testing.T, ctx context.Context, orgStore *pgorg.Store, orgID int, name string) pgorg.Project {
	t.Helper()

	project, err := orgStore.CreateProject(ctx, pgorg.CreateProject{
		OrgID: orgID,
		Name:  name,
	})
	if err != nil {
		t.Fatalf("seed project %q: %v", name, err)
	}

	return project
}

func seedCustomRole(t *testing.T, ctx context.Context, roleStore *pgrbac.Store, orgID int, name string, perms []mdl.Permission) pgrbac.CustomRole {
	t.Helper()

	role, err := roleStore.CreateCustomRole(ctx, pgrbac.CreateCustomRole{
		OrgID:           orgID,
		Name:            name,
		PermissionNames: permissionsToPg(perms),
	})
	if err != nil {
		t.Fatalf("seed custom role %q: %v", name, err)
	}

	return role
}

func seedOrgMembership(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID, orgID int) {
	t.Helper()

	if _, err := pool.Exec(ctx, "INSERT INTO org.org_membership (user_id, org_id) VALUES ($1, $2)", userID, orgID); err != nil {
		t.Fatalf("seed org membership (user %d, org %d): %v", userID, orgID, err)
	}
}

func seedOrgScopedRoleWithAllPermissions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgStore *pgorg.Store, rbacStore *pgrbac.Store, userID int) {
	t.Helper()

	org, err := orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{Name: "acme", ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("seed organization: %v", err)
	}

	// Give an org-scoped custom role every permission in the database, the same set seeded for superadmin.
	role, err := rbacStore.CreateCustomRole(ctx, pgrbac.CreateCustomRole{
		OrgID:           org.ID,
		Name:            "org-scoped-admin",
		PermissionNames: permissionsToPg(mdl.AllPermissions()),
	})
	if err != nil {
		t.Fatalf("seed org-scoped role: %v", err)
	}

	const query = `
		INSERT INTO rbac.org_role_assignments (user_id, role_id, org_id)
		VALUES ($1, $2, $3)`
	if _, err := pool.Exec(ctx, query, userID, role.ID, org.ID); err != nil {
		t.Fatalf("seed org-scoped role assignment: %v", err)
	}
}
