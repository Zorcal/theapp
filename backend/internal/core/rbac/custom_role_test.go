package rbac

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

// TestCore_integration_customRoleLifecycle exercises custom-role validation and persistence
// through the core and PostgreSQL store layers.
func TestCore_integration_customRoleLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)
	roleStore := pgrbac.NewStore(pool)
	core := NewCore(roleStore, pgdb.NewTransactor(pool))

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(mdl.CustomRole{}, "ID", "CreatedAt", "UpdatedAt", "ETag"),
	}

	org := seedOrg(t, orgStore, "custom-role-integration-org")
	project := seedProject(t, ctx, orgStore, org.ID, "custom-role-integration-project")
	actor := seedUser(t, ctx, userStore, "custom-role-actor@test.com", "Custom Role Actor")
	seedSystemRoleAssignment(t, ctx, pool, actor.ID, "superadmin")
	roleCtx := mdl.ContextWithAuthSession(ctx, mdl.AuthSession{
		User:      mdl.AuthUser{UserID: actor.ExternalID},
		ProjectID: &project.ID,
		OrgID:     &org.ID,
	})

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
	actor := seedUser(t, ctx, userStore, "custom-role-assignment-actor@test.com", "Actor User")
	seedSystemRoleAssignment(t, ctx, pool, actor.ID, "superadmin")
	user := seedUser(t, ctx, userStore, "custom-role-assignment@test.com", "Target User")
	seedOrgMembership(t, ctx, pool, user.ID, org.ID)
	role := seedCustomRole(t, ctx, roleStore, org.ID, "reader", []mdl.Permission{mdl.PermissionCustomRoleRead})

	roleCtx := mdl.ContextWithAuthSession(ctx, mdl.AuthSession{
		User:      mdl.AuthUser{UserID: actor.ExternalID},
		ProjectID: &firstProject.ID,
		OrgID:     &org.ID,
	})

	assertProjectPermNames := func(projectID int, want []string) {
		t.Helper()

		projectPerms, err := roleStore.ProjectPermissions(ctx, user.ExternalID, projectID)
		if err != nil {
			t.Fatalf("ProjectPermissions(%d) error = %v", projectID, err)
		}

		testingx.AssertDiff(t, projectPerms.PermissionNames, want, cmpopts.EquateEmpty())
	}

	assertProjectRoles := func(want []mdl.CustomRole) {
		t.Helper()

		got, count, err := core.UserProjectCustomRoles(roleCtx, user.ExternalID, 50, 0)
		if err != nil {
			t.Fatalf("UserProjectCustomRoles() error = %v", err)
		}

		testingx.AssertDiff(t, got, want, cmpopts.EquateEmpty())

		if wantCount := len(want); count != wantCount {
			t.Errorf("UserProjectCustomRoles() total count = %d, want %d", count, wantCount)
		}
	}

	assertOrgRoles := func(want []mdl.CustomRole) {
		t.Helper()

		got, count, err := core.UserOrgCustomRoles(roleCtx, user.ExternalID, 50, 0)
		if err != nil {
			t.Fatalf("UserOrgCustomRoles() error = %v", err)
		}

		testingx.AssertDiff(t, got, want, cmpopts.EquateEmpty())

		if wantCount := len(want); count != wantCount {
			t.Errorf("UserOrgCustomRoles() total count = %d, want %d", count, wantCount)
		}
	}

	// Assign and unassign the role in one project.

	if err := core.AssignCustomRoleToProject(roleCtx, user.ExternalID, role.ExternalID); err != nil {
		t.Fatalf("AssignCustomRoleToProject() error = %v", err)
	}

	assertProjectPermNames(firstProject.ID, []string{"custom-role:read"})
	assertProjectRoles([]mdl.CustomRole{customRoleFromPg(role)})

	if err := core.UnassignCustomRoleFromProject(roleCtx, user.ExternalID, role.ExternalID); err != nil {
		t.Fatalf("UnassignCustomRoleFromProject() error = %v", err)
	}

	assertProjectPermNames(firstProject.ID, nil)
	assertProjectRoles([]mdl.CustomRole{})

	// Assign and unassign the role across the organization.

	if err := core.AssignCustomRoleToOrg(roleCtx, user.ExternalID, role.ExternalID); err != nil {
		t.Fatalf("AssignCustomRoleToOrg() error = %v", err)
	}

	assertProjectPermNames(firstProject.ID, []string{"custom-role:read"})
	assertProjectPermNames(secondProject.ID, []string{"custom-role:read"})
	assertOrgRoles([]mdl.CustomRole{customRoleFromPg(role)})

	if err := core.UnassignCustomRoleFromOrg(roleCtx, user.ExternalID, role.ExternalID); err != nil {
		t.Fatalf("UnassignCustomRoleFromOrg() error = %v", err)
	}

	assertProjectPermNames(firstProject.ID, nil)
	assertProjectPermNames(secondProject.ID, nil)
	assertOrgRoles([]mdl.CustomRole{})
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

func TestCore_UserProjectCustomRoles(t *testing.T) {
	mockedRole := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "project reader",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		UpdatedAt:       new(time.Now().Add(time.Minute)),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		UserProjectCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _, _ int) ([]pgrbac.CustomRole, error) {
			return []pgrbac.CustomRole{mockedRole}, nil
		},
		UserProjectCustomRoleCountFunc: func(_ context.Context, _ uuid.UUID, _ int) (int, error) {
			return 1, nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{ProjectID: new(42)})

	got, count, err := core.UserProjectCustomRoles(ctx, uuid.New(), 25, 0)
	if err != nil {
		t.Fatalf("UserProjectCustomRoles() error = %v", err)
	}

	if want := 1; count != want {
		t.Errorf("UserProjectCustomRoles() total count = %d, want %d", count, want)
	}

	want := []mdl.CustomRole{customRoleFromPg(mockedRole)}

	testingx.AssertDiff(t, got, want)
}

func TestCore_UserProjectCustomRoles_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "roles store error",
			roleStorer: &MockedRoleStorer{
				UserProjectCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _, _ int) ([]pgrbac.CustomRole, error) {
					return nil, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "target not found",
			roleStorer: &MockedRoleStorer{
				UserProjectCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _, _ int) ([]pgrbac.CustomRole, error) {
					return nil, nil
				},
				UserProjectCustomRoleCountFunc: func(_ context.Context, _ uuid.UUID, _ int) (int, error) {
					return 0, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "count store error",
			roleStorer: &MockedRoleStorer{
				UserProjectCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _, _ int) ([]pgrbac.CustomRole, error) {
					return nil, nil
				},
				UserProjectCustomRoleCountFunc: func(_ context.Context, _ uuid.UUID, _ int) (int, error) {
					return 0, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{ProjectID: new(42)})

			if _, _, err := core.UserProjectCustomRoles(ctx, uuid.New(), 25, 0); !errors.Is(err, tt.want) {
				t.Errorf("UserProjectCustomRoles() error = %v, want %v", err, tt.want)
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

				if _, _, err := core.UserProjectCustomRoles(tt.ctx, uuid.New(), 25, 0); err == nil {
					t.Error("UserProjectCustomRoles() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_UserOrgCustomRoles(t *testing.T) {
	mockedRole := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "organization reader",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		UpdatedAt:       new(time.Now().Add(time.Minute)),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		UserOrgCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _, _ int) ([]pgrbac.CustomRole, error) {
			return []pgrbac.CustomRole{mockedRole}, nil
		},
		UserOrgCustomRoleCountFunc: func(_ context.Context, _ uuid.UUID, _ int) (int, error) {
			return 1, nil
		},
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})

	got, count, err := core.UserOrgCustomRoles(ctx, uuid.New(), 25, 0)
	if err != nil {
		t.Fatalf("UserOrgCustomRoles() error = %v", err)
	}

	if want := 1; count != want {
		t.Errorf("UserOrgCustomRoles() total count = %d, want %d", count, want)
	}

	want := []mdl.CustomRole{customRoleFromPg(mockedRole)}

	testingx.AssertDiff(t, got, want)
}

func TestCore_UserOrgCustomRoles_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "roles store error",
			roleStorer: &MockedRoleStorer{
				UserOrgCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _, _ int) ([]pgrbac.CustomRole, error) {
					return nil, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "target not found",
			roleStorer: &MockedRoleStorer{
				UserOrgCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _, _ int) ([]pgrbac.CustomRole, error) {
					return nil, nil
				},
				UserOrgCustomRoleCountFunc: func(_ context.Context, _ uuid.UUID, _ int) (int, error) {
					return 0, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "count store error",
			roleStorer: &MockedRoleStorer{
				UserOrgCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _, _ int) ([]pgrbac.CustomRole, error) {
					return nil, nil
				},
				UserOrgCustomRoleCountFunc: func(_ context.Context, _ uuid.UUID, _ int) (int, error) {
					return 0, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)})

			if _, _, err := core.UserOrgCustomRoles(ctx, uuid.New(), 25, 0); !errors.Is(err, tt.want) {
				t.Errorf("UserOrgCustomRoles() error = %v, want %v", err, tt.want)
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

				if _, _, err := core.UserOrgCustomRoles(tt.ctx, uuid.New(), 25, 0); err == nil {
					t.Error("UserOrgCustomRoles() error = nil, want error")
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

func TestCore_CreateCustomRole(t *testing.T) {
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User:      mdl.AuthUser{UserID: uuid.New()},
		ProjectID: new(7),
		OrgID:     new(42),
	})
	mockOutput := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
			return pgrbac.OrgPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
		},
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
			name: "actor or organization not found",
			in:   mdl.CreateCustomRole{Name: "project manager"},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "resolve actor permissions",
			in:   mdl.CreateCustomRole{Name: "project manager"},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "permission denied",
			in: mdl.CreateCustomRole{
				Name:        "project manager",
				Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
			},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{
						OrgID:           42,
						PermissionNames: nil, // Missing custom-role:read
					}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
		},
		{
			name: "role already exists",
			in:   mdl.CreateCustomRole{Name: "project manager"},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
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
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
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
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
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
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CreateCustomRoleFunc: func(_ context.Context, _ pgrbac.CreateCustomRole) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
				User:      mdl.AuthUser{UserID: uuid.New()},
				ProjectID: new(7),
				OrgID:     new(42),
			})
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
			{
				name: "project context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)}),
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
	actorID := uuid.New()
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User:      mdl.AuthUser{UserID: actorID},
		ProjectID: new(7),
		OrgID:     new(42),
	})
	mockOutput := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
			return pgrbac.OrgPermissions{
				OrgID:           42,
				PermissionNames: []string{"custom-role:create", "custom-role:delete"},
			}, nil
		},
		CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
			return pgrbac.CustomRole{PermissionNames: []string{"custom-role:delete"}}, nil
		},
		UpdateCustomRoleFunc: func(_ context.Context, _ pgrbac.UpdateCustomRole) (pgrbac.CustomRole, error) {
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
			name: "actor or organization not found",
			in:   mdl.UpdateCustomRole{Fields: mdl.CustomRoleUpdateFields{Permissions: true}},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "resolve actor permissions",
			in:   mdl.UpdateCustomRole{Fields: mdl.CustomRoleUpdateFields{Permissions: true}},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "permission update role not found",
			in:   mdl.UpdateCustomRole{Fields: mdl.CustomRoleUpdateFields{Permissions: true}},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "get permission update role",
			in:   mdl.UpdateCustomRole{Fields: mdl.CustomRoleUpdateFields{Permissions: true}},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "permission denied",
			in: mdl.UpdateCustomRole{
				Fields:      mdl.CustomRoleUpdateFields{Permissions: true},
				Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
			},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{
						OrgID:           42,
						PermissionNames: nil, // Missing custom-role:read
					}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
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
			name: "role already exists",
			in:   mdl.UpdateCustomRole{},
			roleStorer: &MockedRoleStorer{
				UpdateCustomRoleFunc: func(_ context.Context, _ pgrbac.UpdateCustomRole) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "constraint violated",
			in:   mdl.UpdateCustomRole{},
			roleStorer: &MockedRoleStorer{
				UpdateCustomRoleFunc: func(_ context.Context, _ pgrbac.UpdateCustomRole) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, pgdb.ErrCheckConstraintViolated
				},
			},
			want: mdl.ErrValidation,
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
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
				User:      mdl.AuthUser{UserID: uuid.New()},
				ProjectID: new(7),
				OrgID:     new(42),
			})
			core := NewCore(tt.roleStorer, immediateTransactor{})

			if _, err := core.UpdateCustomRole(ctx, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("UpdateCustomRole() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("missing auth data", func(t *testing.T) {
		tests := []struct {
			name string
			in   mdl.UpdateCustomRole
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
			{
				name: "project context missing for permission update",
				in:   mdl.UpdateCustomRole{Fields: mdl.CustomRoleUpdateFields{Permissions: true}},
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)}),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core := NewCore(&MockedRoleStorer{}, immediateTransactor{})

				if _, err := core.UpdateCustomRole(tt.ctx, tt.in); err == nil {
					t.Error("UpdateCustomRole() error = nil, want error")
				}
			})
		}
	})
}

func TestCore_ModifyCustomRolePermissions(t *testing.T) {
	actorID := uuid.New()
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User:      mdl.AuthUser{UserID: actorID},
		ProjectID: new(7),
		OrgID:     new(42),
	})
	mockOutput := pgrbac.CustomRole{
		ID:              1,
		ExternalID:      uuid.New(),
		Name:            "project manager",
		PermissionNames: []string{"custom-role:read"},
		CreatedAt:       time.Now(),
		ETag:            uuid.New(),
	}
	roleStorer := &MockedRoleStorer{
		OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
			return pgrbac.OrgPermissions{
				OrgID:           42,
				PermissionNames: []string{"custom-role:read", "custom-role:delete"},
			}, nil
		},
		CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
			return pgrbac.CustomRole{PermissionNames: []string{"custom-role:delete"}}, nil
		},
		ModifyCustomRolePermissionsFunc: func(_ context.Context, _ pgrbac.ModifyCustomRolePermissions) (pgrbac.CustomRole, error) {
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
			name: "actor or organization not found",
			in:   mdl.ModifyCustomRolePermissions{},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "resolve actor permissions",
			in:   mdl.ModifyCustomRolePermissions{},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role not found",
			in:   mdl.ModifyCustomRolePermissions{},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "get role",
			in:   mdl.ModifyCustomRolePermissions{},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "permission denied",
			in: mdl.ModifyCustomRolePermissions{
				AddPermissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
			},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{
						OrgID:           42,
						PermissionNames: nil, // Missing custom-role:read
					}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
		},
		{
			name: "permission missing during update",
			in:   mdl.ModifyCustomRolePermissions{},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, nil
				},
				ModifyCustomRolePermissionsFunc: func(_ context.Context, _ pgrbac.ModifyCustomRolePermissions) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "store",
			in:   mdl.ModifyCustomRolePermissions{},
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, nil
				},
				ModifyCustomRolePermissionsFunc: func(_ context.Context, _ pgrbac.ModifyCustomRolePermissions) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
				User:      mdl.AuthUser{UserID: uuid.New()},
				ProjectID: new(7),
				OrgID:     new(42),
			})
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
			{
				name: "project context missing",
				ctx:  mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{OrgID: new(42)}),
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
	actorID := uuid.New()
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User:      mdl.AuthUser{UserID: actorID},
		ProjectID: new(42),
	})
	mockedRole := pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}
	roleStorer := &MockedRoleStorer{
		ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
			return pgrbac.ProjectPermissions{
				OrgID:           7,
				PermissionNames: []string{"custom-role:read", "custom-role:update"},
			}, nil
		},
		CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
			return mockedRole, nil
		},
		AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error { return nil },
	}
	core := NewCore(roleStorer, immediateTransactor{})

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
			name: "actor or project not found",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "resolve actor permissions",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{OrgID: 7}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "get role",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{OrgID: 7}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "permission denied",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{
						OrgID:           7,
						PermissionNames: []string{"custom-role:read"},
					}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{
						PermissionNames: []string{"custom-role:read", "custom-role:update"},
					}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
		},
		{
			name: "assignment target not found",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{OrgID: 7, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "already assigned",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{OrgID: 7, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "store",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{OrgID: 7, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
				User:      mdl.AuthUser{UserID: uuid.New()},
				ProjectID: new(42),
			})
			core := NewCore(tt.roleStorer, immediateTransactor{})

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
		ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
			return pgrbac.ProjectPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
		},
		CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
			return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
		},
		UnassignCustomRoleFromProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error { return nil },
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User:      mdl.AuthUser{UserID: uuid.New()},
		ProjectID: new(42),
	})

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
			name: "actor or project not found",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "resolve actor permissions",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "get role",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "permission denied",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{
						OrgID:           42,
						PermissionNames: nil, // Missing custom-role:read
					}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
		},
		{
			name: "assignment not found",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				UnassignCustomRoleFromProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "store",
			roleStorer: &MockedRoleStorer{
				ProjectPermissionsFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.ProjectPermissions, error) {
					return pgrbac.ProjectPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				UnassignCustomRoleFromProjectFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
				User:      mdl.AuthUser{UserID: uuid.New()},
				ProjectID: new(42),
			})

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
	actorID := uuid.New()
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User:      mdl.AuthUser{UserID: actorID},
		ProjectID: new(7),
		OrgID:     new(42),
	})
	mockedRole := pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}
	roleStorer := &MockedRoleStorer{
		OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
			return pgrbac.OrgPermissions{
				OrgID:           42,
				PermissionNames: []string{"custom-role:read", "custom-role:update"},
			}, nil
		},
		CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
			return mockedRole, nil
		},
		AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error { return nil },
	}
	core := NewCore(roleStorer, immediateTransactor{})

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
			name: "actor or organization not found",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "resolve actor permissions",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "get role",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "permission denied",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{
						OrgID:           42,
						PermissionNames: []string{"custom-role:read"}, // Missing custom-role:update
					}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{
						PermissionNames: []string{"custom-role:read", "custom-role:update"},
					}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
		},
		{
			name: "assignment target not found",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "already assigned",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return pgdb.ErrAlreadyExists
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "store",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
				User:      mdl.AuthUser{UserID: uuid.New()},
				ProjectID: new(7),
				OrgID:     new(42),
			})
			core := NewCore(tt.roleStorer, immediateTransactor{})

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
				name: "project context missing",
				ctx: mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
					User:  mdl.AuthUser{UserID: uuid.New()},
					OrgID: new(42),
				}),
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
		OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
			return pgrbac.OrgPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
		},
		CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
			return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
		},
		UnassignCustomRoleFromOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error { return nil },
	}
	core := NewCore(roleStorer, immediateTransactor{})
	ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
		User:      mdl.AuthUser{UserID: uuid.New()},
		ProjectID: new(7),
		OrgID:     new(42),
	})

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
			name: "actor or organization not found",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "resolve actor permissions",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "get role",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "permission denied",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{
						OrgID:           42,
						PermissionNames: nil, // Missing custom-role:read
					}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
			},
			want: mdl.ErrPermissionDenied,
		},
		{
			name: "assignment not found",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				UnassignCustomRoleFromOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "store",
			roleStorer: &MockedRoleStorer{
				OrgPermissionsByProjectIDFunc: func(_ context.Context, _ uuid.UUID, _ int) (pgrbac.OrgPermissions, error) {
					return pgrbac.OrgPermissions{OrgID: 42, PermissionNames: []string{"custom-role:read"}}, nil
				},
				CustomRoleByExternalIDFunc: func(_ context.Context, _ int, _ uuid.UUID) (pgrbac.CustomRole, error) {
					return pgrbac.CustomRole{PermissionNames: []string{"custom-role:read"}}, nil
				},
				UnassignCustomRoleFromOrgFunc: func(_ context.Context, _, _ uuid.UUID, _ int) error {
					return dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, immediateTransactor{})
			ctx := mdl.ContextWithAuthSession(t.Context(), mdl.AuthSession{
				User:      mdl.AuthUser{UserID: uuid.New()},
				ProjectID: new(7),
				OrgID:     new(42),
			})

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
				name: "project context missing",
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
