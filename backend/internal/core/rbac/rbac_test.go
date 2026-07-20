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

type noopTransactor struct{}

func (noopTransactor) RunTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestCore_integration(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	orgStore := pgorg.NewStore(pool)
	core := NewCore(pgrbac.NewStore(pool), userStore, orgStore, pgdb.NewTransactor(pool))

	diffOpts := cmp.Options{
		cmpopts.SortSlices(func(a, b mdl.Permission) bool { return a < b }),
		cmpopts.IgnoreFields(mdl.RoleCustom{}, "ID", "CreatedAt", "UpdatedAt", "ETag"),
	}

	staticRoles, err := core.StaticRoles(ctx)
	if err != nil {
		t.Fatalf("StaticRoles() error = %v", err)
	}

	wantStaticRoles := []mdl.RoleStatic{
		{Name: "superadmin", Permissions: mdl.AllPermissions},
	}

	testingx.AssertDiff(t, staticRoles, wantStaticRoles, diffOpts, cmpopts.IgnoreFields(mdl.RoleStatic{}, "ID", "CreatedAt", "UpdatedAt"))

	usr, err := userStore.CreateUser(ctx, pguser.CreateUser{Email: "alice@test.com", Name: "Alice Smith"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if err := core.AssignSystemRole(ctx, usr.ExternalID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}

	staticPermissions, staticTotalCount, err := core.StaticRolePermissions(ctx, "superadmin", 50, 0)
	if err != nil {
		t.Fatalf("StaticRolePermissions() error = %v", err)
	}
	if staticTotalCount != len(mdl.AllPermissions) {
		t.Errorf("StaticRolePermissions() totalCount = %d, want %d", staticTotalCount, len(mdl.AllPermissions))
	}
	testingx.AssertDiff(t, staticPermissions, mdl.AllPermissions, cmpopts.SortSlices(func(a, b mdl.Permission) bool { return a < b }))

	systemRoleNames, systemTotalCount, err := core.SystemRoleAssignments(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("SystemRoleAssignments() error = %v", err)
	}
	if systemTotalCount != 1 {
		t.Errorf("SystemRoleAssignments() totalCount = %d, want 1", systemTotalCount)
	}
	testingx.AssertDiff(t, systemRoleNames, []string{"superadmin"})

	org, err := orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{Name: "acme", ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	role, err := core.CreateRole(ctx, org.ID, mdl.CreateRole{Name: "viewer", Permissions: []mdl.Permission{mdl.PermissionRoleRead}})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}

	wantRole := mdl.RoleCustom{Name: "viewer", OrgID: org.ID, Permissions: []mdl.Permission{mdl.PermissionRoleRead}}
	testingx.AssertDiff(t, role, wantRole, diffOpts)

	if role.ETag == "" {
		t.Error("CreateRole() ETag is empty, want non-empty")
	}

	orgRoles, totalCount, err := core.OrgRoles(ctx, org.ID, 50, 0)
	if err != nil {
		t.Fatalf("OrgRoles() error = %v", err)
	}
	if totalCount != 1 {
		t.Errorf("OrgRoles() totalCount = %d, want 1", totalCount)
	}

	wantOrgRoles := []mdl.RoleCustom{
		{Name: "viewer", OrgID: org.ID, Permissions: []mdl.Permission{mdl.PermissionRoleRead}},
	}
	testingx.AssertDiff(t, orgRoles, wantOrgRoles, diffOpts)

	// Rename and replace the permission set entirely.
	updated, err := core.UpdateRole(ctx, org.ID, mdl.UpdateRole{
		ID:          role.ID,
		Fields:      mdl.RoleUpdateFields{Name: true, Permissions: true},
		Name:        "viewer-renamed",
		Permissions: []mdl.Permission{mdl.PermissionRoleCreate},
	})
	if err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}

	wantUpdated := mdl.RoleCustom{
		Name:        "viewer-renamed",
		OrgID:       org.ID,
		Permissions: []mdl.Permission{mdl.PermissionRoleCreate},
	}
	testingx.AssertDiff(t, updated, wantUpdated, diffOpts)

	// Partial update: only name changes, permissions carry over unchanged.
	renamedAgain, err := core.UpdateRole(ctx, org.ID, mdl.UpdateRole{
		ID:     role.ID,
		Fields: mdl.RoleUpdateFields{Name: true},
		Name:   "viewer-renamed-again",
	})
	if err != nil {
		t.Fatalf("UpdateRole() partial error = %v", err)
	}

	wantRenamedAgain := mdl.RoleCustom{
		Name:        "viewer-renamed-again",
		OrgID:       org.ID,
		Permissions: []mdl.Permission{mdl.PermissionRoleCreate},
	}
	testingx.AssertDiff(t, renamedAgain, wantRenamedAgain, diffOpts)

	// Add and remove a permission in one call — the existing permission carries over.
	modified, err := core.ModifyRolePermissions(ctx, org.ID, mdl.ModifyRolePermissions{
		ID:                role.ID,
		AddPermissions:    []mdl.Permission{mdl.PermissionRoleRead},
		RemovePermissions: []mdl.Permission{mdl.PermissionRoleCreate},
	})
	if err != nil {
		t.Fatalf("ModifyRolePermissions() error = %v", err)
	}

	wantModified := mdl.RoleCustom{
		Name:        "viewer-renamed-again",
		OrgID:       org.ID,
		Permissions: []mdl.Permission{mdl.PermissionRoleRead},
	}
	testingx.AssertDiff(t, modified, wantModified, diffOpts)

	if err := core.DeleteRole(ctx, org.ID, role.ID); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}

	if _, err := core.UpdateRole(ctx, org.ID, mdl.UpdateRole{
		ID:     role.ID,
		Fields: mdl.RoleUpdateFields{Name: true},
		Name:   "viewer-renamed",
	}); !errors.Is(err, mdl.ErrNotFound) {
		t.Errorf("UpdateRole() after delete error = %v, want %v", err, mdl.ErrNotFound)
	}
}

// TestCore_resolveOwnedCustomRole_staticRoleID proves that a static role's own external ID,
// passed to a RoleService operation that only ever resolves custom roles, surfaces as
// mdl.ErrNotFound rather than a distinct "that's a static role" error — static roles live in
// their own table (rbac.static_roles), entirely separate from the custom_roles table
// resolveOwnedCustomRole queries, so the lookup simply finds nothing.
//
// A more specific error here would require an extra existence check against rbac.static_roles
// once the primary lookup misses, purely to improve this one error message — not done here, since
// this path only matters for a caller who already holds a static role's ID (there's no
// RoleService endpoint that would ever hand one out) and mistakenly feeds it to a custom-role
// operation.
func TestCore_resolveOwnedCustomRole_staticRoleID(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	orgStore := pgorg.NewStore(pool)
	core := NewCore(pgrbac.NewStore(pool), userStore, orgStore, pgdb.NewTransactor(pool))

	staticRoles, err := core.StaticRoles(ctx)
	if err != nil {
		t.Fatalf("StaticRoles() error = %v", err)
	}
	if len(staticRoles) == 0 {
		t.Fatal("StaticRoles() = empty, want at least superadmin")
	}
	superadminID := staticRoles[0].ID

	org, err := orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{Name: "acme", ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	if _, _, err := core.RolePermissions(ctx, org.ID, superadminID, 50, 0); !errors.Is(err, mdl.ErrNotFound) {
		t.Errorf("RolePermissions(superadmin's static ID) error = %v, want %v", err, mdl.ErrNotFound)
	}
}

func TestCore_AssignRole_integration(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	orgStore := pgorg.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)
	core := NewCore(rbacStore, userStore, orgStore, pgdb.NewTransactor(pool))

	org, err := orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{Name: "acme", ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	project, err := orgStore.CreateProject(ctx, pgorg.CreateProject{OrgID: org.ID, Name: "widgets"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	otherOrg, err := orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{Name: "other", ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	usr, err := userStore.CreateUser(ctx, pguser.CreateUser{Email: "alice@test.com", Name: "Alice Smith"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	role, err := core.CreateRole(ctx, org.ID, mdl.CreateRole{Name: "editor", Permissions: []mdl.Permission{mdl.PermissionRoleRead}})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO org.org_membership (user_id, org_id) VALUES ($1, $2)`, usr.ID, org.ID); err != nil {
		t.Fatalf("seed org membership: %v", err)
	}

	// Assign at project scope.
	if err := core.AssignRole(ctx, org.ID, mdl.AssignRole{
		RoleID: role.ID,
		UserID: usr.ExternalID,
		Scope:  mdl.RoleScope{ProjectID: &project.ID},
	}); err != nil {
		t.Fatalf("AssignRole() project scope error = %v", err)
	}

	assignments, totalCount, err := core.ListRoleAssignments(ctx, org.ID, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("ListRoleAssignments() error = %v", err)
	}
	if totalCount != 1 {
		t.Errorf("ListRoleAssignments() totalCount = %d, want 1", totalCount)
	}
	wantAssignments := []mdl.RoleAssignment{
		{RoleID: role.ID, RoleName: "editor", Scope: mdl.RoleScope{ProjectID: &project.ID}},
	}
	testingx.AssertDiff(t, assignments, wantAssignments)

	// Assigning at org scope, while a project-scope grant already exists, promotes it: the
	// project-scope row is deleted and an org-scope row is inserted instead.
	if err := core.AssignRole(ctx, org.ID, mdl.AssignRole{
		RoleID: role.ID,
		UserID: usr.ExternalID,
		Scope:  mdl.RoleScope{OrgID: &org.ID},
	}); err != nil {
		t.Fatalf("AssignRole() org scope error = %v", err)
	}

	assignments, totalCount, err = core.ListRoleAssignments(ctx, org.ID, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("ListRoleAssignments() after promotion error = %v", err)
	}
	if totalCount != 1 {
		t.Errorf("ListRoleAssignments() after promotion totalCount = %d, want 1", totalCount)
	}
	wantPromoted := []mdl.RoleAssignment{
		{RoleID: role.ID, RoleName: "editor", Scope: mdl.RoleScope{OrgID: &org.ID}},
	}
	testingx.AssertDiff(t, assignments, wantPromoted)

	// Assigning at project scope, while an org-scope grant already exists, is rejected.
	if err := core.AssignRole(ctx, org.ID, mdl.AssignRole{
		RoleID: role.ID,
		UserID: usr.ExternalID,
		Scope:  mdl.RoleScope{ProjectID: &project.ID},
	}); !errors.Is(err, mdl.ErrRoleScopeConflict) {
		t.Errorf("AssignRole() project scope after org scope error = %v, want %v", err, mdl.ErrRoleScopeConflict)
	}

	// Assigning at org scope for a user who isn't a member of that org is rejected.
	if err := core.AssignRole(ctx, otherOrg.ID, mdl.AssignRole{
		RoleID: role.ID,
		UserID: usr.ExternalID,
		Scope:  mdl.RoleScope{OrgID: &otherOrg.ID},
	}); !errors.Is(err, mdl.ErrNotFound) {
		// role isn't owned by otherOrg, so this fails ownership resolution before membership is
		// ever checked — confirming NotOrgMember requires a role actually owned by otherOrg.
		t.Errorf("AssignRole() org scope, foreign role, error = %v, want %v", err, mdl.ErrNotFound)
	}

	otherRole, err := core.CreateRole(ctx, otherOrg.ID, mdl.CreateRole{Name: "other-editor", Permissions: []mdl.Permission{mdl.PermissionRoleRead}})
	if err != nil {
		t.Fatalf("CreateRole() in otherOrg error = %v", err)
	}
	if err := core.AssignRole(ctx, otherOrg.ID, mdl.AssignRole{
		RoleID: otherRole.ID,
		UserID: usr.ExternalID,
		Scope:  mdl.RoleScope{OrgID: &otherOrg.ID},
	}); !errors.Is(err, mdl.ErrNotOrgMember) {
		t.Errorf("AssignRole() org scope, non-member, error = %v, want %v", err, mdl.ErrNotOrgMember)
	}

	permissions, permTotalCount, err := core.RolePermissions(ctx, org.ID, role.ID, 50, 0)
	if err != nil {
		t.Fatalf("RolePermissions() error = %v", err)
	}
	if permTotalCount != 1 {
		t.Errorf("RolePermissions() totalCount = %d, want 1", permTotalCount)
	}
	testingx.AssertDiff(t, permissions, []mdl.Permission{mdl.PermissionRoleRead})

	// Unassign at org scope.
	if err := core.UnassignRole(ctx, org.ID, mdl.UnassignRole{
		RoleID: role.ID,
		UserID: usr.ExternalID,
		Scope:  mdl.RoleScope{OrgID: &org.ID},
	}); err != nil {
		t.Fatalf("UnassignRole() error = %v", err)
	}

	_, totalCount, err = core.ListRoleAssignments(ctx, org.ID, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("ListRoleAssignments() after unassign error = %v", err)
	}
	if totalCount != 0 {
		t.Errorf("ListRoleAssignments() after unassign totalCount = %d, want 0", totalCount)
	}
}

func TestCore_CreateRole_error(t *testing.T) {
	core := NewCore(&MockedRoleStorer{}, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

	if _, err := core.CreateRole(t.Context(), 1, mdl.CreateRole{}); !errors.Is(err, mdl.ErrValidation) {
		t.Errorf("CreateRole() error = %v, want %v", err, mdl.ErrValidation)
	}
}

func TestCore_UpdateRole(t *testing.T) {
	orgID := 1
	externalID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	roleStorer := &MockedRoleStorer{
		RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
			return pgrbac.RoleCustom{ID: 5, ExternalID: externalID, OrgID: orgID}, nil
		},
		UpdateRoleFunc: func(_ context.Context, ur pgrbac.UpdateRole) (pgrbac.RoleCustom, error) {
			return pgrbac.RoleCustom{
				ID:              ur.ID,
				ExternalID:      externalID,
				OrgID:           ur.OrgID,
				Name:            ur.Name,
				PermissionNames: ur.PermissionNames,
				ETag:            uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			}, nil
		},
	}
	core := NewCore(roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

	got, err := core.UpdateRole(t.Context(), orgID, mdl.UpdateRole{
		ID:          externalID,
		Fields:      mdl.RoleUpdateFields{Name: true, Permissions: true},
		Name:        "viewer",
		Permissions: []mdl.Permission{mdl.PermissionRoleRead},
	})
	if err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}

	want := mdl.RoleCustom{
		ID:          externalID,
		OrgID:       orgID,
		Name:        "viewer",
		Permissions: []mdl.Permission{mdl.PermissionRoleRead},
		ETag:        "11111111-1111-1111-1111-111111111111",
	}
	testingx.AssertDiff(t, got, want)
}

func TestCore_UpdateRole_partial(t *testing.T) {
	orgID := 1
	externalID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	roleStorer := &MockedRoleStorer{
		RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
			return pgrbac.RoleCustom{
				ID:              5,
				ExternalID:      externalID,
				OrgID:           orgID,
				Name:            "viewer",
				PermissionNames: []string{"role:read"},
			}, nil
		},
		UpdateRoleFunc: func(_ context.Context, ur pgrbac.UpdateRole) (pgrbac.RoleCustom, error) {
			return pgrbac.RoleCustom{
				ID:              ur.ID,
				ExternalID:      externalID,
				OrgID:           ur.OrgID,
				Name:            ur.Name,
				PermissionNames: ur.PermissionNames,
				ETag:            uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			}, nil
		},
	}
	core := NewCore(roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

	// Only Name is set — the existing permission set must carry over unchanged.
	got, err := core.UpdateRole(t.Context(), orgID, mdl.UpdateRole{
		ID:     externalID,
		Fields: mdl.RoleUpdateFields{Name: true},
		Name:   "viewer-renamed",
	})
	if err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}

	want := mdl.RoleCustom{
		ID:          externalID,
		OrgID:       orgID,
		Name:        "viewer-renamed",
		Permissions: []mdl.Permission{mdl.PermissionRoleRead},
		ETag:        "11111111-1111-1111-1111-111111111111",
	}
	testingx.AssertDiff(t, got, want)
}

func TestCore_UpdateRole_error(t *testing.T) {
	otherOrgID := 2
	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			// A static role's external ID resolves the same way: static roles live in
			// rbac.static_roles, never in rbac.custom_roles, so RoleByExternalID simply finds no
			// row for one.
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "different org",
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: otherOrgID}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

			ur := mdl.UpdateRole{ID: uuid.New(), Fields: mdl.RoleUpdateFields{Name: true, Permissions: true}, Name: "viewer", Permissions: []mdl.Permission{mdl.PermissionRoleRead}}
			if _, err := core.UpdateRole(t.Context(), 1, ur); !errors.Is(err, tt.want) {
				t.Errorf("UpdateRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_ModifyRolePermissions(t *testing.T) {
	orgID := 1
	externalID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	roleStorer := &MockedRoleStorer{
		RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
			return pgrbac.RoleCustom{
				ID:              5,
				ExternalID:      externalID,
				OrgID:           orgID,
				Name:            "viewer",
				PermissionNames: []string{"role:read"},
			}, nil
		},
		UpdateRoleFunc: func(_ context.Context, ur pgrbac.UpdateRole) (pgrbac.RoleCustom, error) {
			return pgrbac.RoleCustom{
				ID:              ur.ID,
				ExternalID:      externalID,
				OrgID:           ur.OrgID,
				Name:            ur.Name,
				PermissionNames: ur.PermissionNames,
				ETag:            uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			}, nil
		},
	}
	core := NewCore(roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

	got, err := core.ModifyRolePermissions(t.Context(), orgID, mdl.ModifyRolePermissions{
		ID:                externalID,
		AddPermissions:    []mdl.Permission{mdl.PermissionRoleCreate},
		RemovePermissions: []mdl.Permission{mdl.PermissionRoleRead},
	})
	if err != nil {
		t.Fatalf("ModifyRolePermissions() error = %v", err)
	}

	want := mdl.RoleCustom{
		ID:          externalID,
		OrgID:       orgID,
		Name:        "viewer",
		Permissions: []mdl.Permission{mdl.PermissionRoleCreate},
		ETag:        "11111111-1111-1111-1111-111111111111",
	}
	testingx.AssertDiff(t, got, want)
}

func TestCore_ModifyRolePermissions_error(t *testing.T) {
	orgID := 1
	otherOrgID := 2
	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "different org",
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: otherOrgID}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "remove_permissions leaves no permissions",
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID, PermissionNames: []string{"role:read"}}, nil
				},
			},
			want: mdl.ErrValidation,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

			m := mdl.ModifyRolePermissions{ID: uuid.New(), RemovePermissions: []mdl.Permission{mdl.PermissionRoleRead}}
			if _, err := core.ModifyRolePermissions(t.Context(), orgID, m); !errors.Is(err, tt.want) {
				t.Errorf("ModifyRolePermissions() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_DeleteRole(t *testing.T) {
	orgID := 1
	roleStorer := &MockedRoleStorer{
		RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
			return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
		},
		DeleteRoleFunc: func(_ context.Context, _, _ int) error {
			return nil
		},
	}
	core := NewCore(roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

	if err := core.DeleteRole(t.Context(), orgID, uuid.New()); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}
}

func TestCore_DeleteRole_error(t *testing.T) {
	otherOrgID := 2
	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		want       error
	}{
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "different org",
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: otherOrgID}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

			if err := core.DeleteRole(t.Context(), 1, uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("DeleteRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_StaticRoles(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		StaticRolesFunc: func(_ context.Context) ([]pgrbac.RoleStatic, error) {
			return []pgrbac.RoleStatic{
				{Name: "superadmin", PermissionNames: []string{"user:create", "user:read"}},
			}, nil
		},
	}
	core := NewCore(roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

	got, err := core.StaticRoles(t.Context())
	if err != nil {
		t.Fatalf("StaticRoles() error = %v", err)
	}

	want := []mdl.RoleStatic{
		{Name: "superadmin", Permissions: []mdl.Permission{mdl.PermissionUserCreate, mdl.PermissionUserRead}},
	}

	testingx.AssertDiff(t, got, want)
}

func TestCore_StaticRoles_error(t *testing.T) {
	dbErr := errors.New("db error")

	core := NewCore(&MockedRoleStorer{
		StaticRolesFunc: func(_ context.Context) ([]pgrbac.RoleStatic, error) {
			return nil, dbErr
		},
	}, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

	if _, err := core.StaticRoles(t.Context()); !errors.Is(err, dbErr) {
		t.Errorf("StaticRoles() error = %v, want %v", err, dbErr)
	}
}

func TestCore_AssignSystemRole(t *testing.T) {
	userStorer := &MockedUserStorer{
		UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
			return pguser.User{ID: 7}, nil
		},
	}
	roleStorer := &MockedRoleStorer{
		AssignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
			return nil
		},
	}
	core := NewCore(roleStorer, userStorer, &MockedOrgStorer{}, noopTransactor{})

	if err := core.AssignSystemRole(t.Context(), uuid.New(), "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}
}

func TestCore_AssignSystemRole_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		roleStorer *MockedRoleStorer
		userStorer *MockedUserStorer
		want       error
	}{
		{
			name:       "user not found",
			roleStorer: &MockedRoleStorer{},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "role not found",
			roleStorer: &MockedRoleStorer{
				AssignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
					return sql.ErrNoRows
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name:       "store error, user lookup",
			roleStorer: &MockedRoleStorer{},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "store error, role assignment",
			roleStorer: &MockedRoleStorer{
				AssignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
					return dbErr
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.roleStorer, tt.userStorer, &MockedOrgStorer{}, noopTransactor{})

			if err := core.AssignSystemRole(t.Context(), uuid.New(), "superadmin"); !errors.Is(err, tt.want) {
				t.Errorf("AssignSystemRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_AssignRole_error(t *testing.T) {
	orgID := 1
	otherOrgID := 2
	projectID := 10

	tests := []struct {
		name       string
		in         mdl.AssignRole
		roleStorer *MockedRoleStorer
		userStorer *MockedUserStorer
		orgStorer  *MockedOrgStorer
		want       error
	}{
		{
			name: "invalid",
			in:   mdl.AssignRole{},
			want: mdl.ErrValidation,
		},
		{
			name: "role not found",
			in:   mdl.AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{ProjectID: &projectID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "different org",
			in:   mdl.AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{ProjectID: &projectID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: otherOrgID}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "user not found",
			in:   mdl.AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{ProjectID: &projectID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "project not found",
			in:   mdl.AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{ProjectID: &projectID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			orgStorer: &MockedOrgStorer{
				ProjectByIDFunc: func(_ context.Context, _ int) (pgorg.Project, error) {
					return pgorg.Project{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "project belongs to a different org",
			in:   mdl.AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{ProjectID: &projectID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			orgStorer: &MockedOrgStorer{
				ProjectByIDFunc: func(_ context.Context, _ int) (pgorg.Project, error) {
					return pgorg.Project{ID: projectID, OrgID: otherOrgID}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "role scope conflict",
			in:   mdl.AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{ProjectID: &projectID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
				},
				OrgAssignmentExistsFunc: func(_ context.Context, _, _, _ int) (bool, error) {
					return true, nil
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			orgStorer: &MockedOrgStorer{
				ProjectByIDFunc: func(_ context.Context, _ int) (pgorg.Project, error) {
					return pgorg.Project{ID: projectID, OrgID: orgID}, nil
				},
			},
			want: mdl.ErrRoleScopeConflict,
		},
		{
			name: "org scope, different org",
			in:   mdl.AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{OrgID: &otherOrgID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "org scope, not a member",
			in:   mdl.AssignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{OrgID: &orgID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			orgStorer: &MockedOrgStorer{
				IsOrgMemberFunc: func(_ context.Context, _, _ int) (bool, error) {
					return false, nil
				},
			},
			want: mdl.ErrNotOrgMember,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roleStorer := tt.roleStorer
			if roleStorer == nil {
				roleStorer = &MockedRoleStorer{}
			}
			userStorer := tt.userStorer
			if userStorer == nil {
				userStorer = &MockedUserStorer{}
			}
			orgStorer := tt.orgStorer
			if orgStorer == nil {
				orgStorer = &MockedOrgStorer{}
			}
			core := NewCore(roleStorer, userStorer, orgStorer, noopTransactor{})

			if err := core.AssignRole(t.Context(), orgID, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("AssignRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_UnassignRole_error(t *testing.T) {
	orgID := 1
	otherOrgID := 2
	projectID := 10

	tests := []struct {
		name       string
		in         mdl.UnassignRole
		roleStorer *MockedRoleStorer
		userStorer *MockedUserStorer
		orgStorer  *MockedOrgStorer
		want       error
	}{
		{
			name: "invalid",
			in:   mdl.UnassignRole{},
			want: mdl.ErrValidation,
		},
		{
			name: "role not found",
			in:   mdl.UnassignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{ProjectID: &projectID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "user not found",
			in:   mdl.UnassignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{ProjectID: &projectID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "project belongs to a different org",
			in:   mdl.UnassignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{ProjectID: &projectID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			orgStorer: &MockedOrgStorer{
				ProjectByIDFunc: func(_ context.Context, _ int) (pgorg.Project, error) {
					return pgorg.Project{ID: projectID, OrgID: otherOrgID}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "org scope, different org",
			in:   mdl.UnassignRole{RoleID: uuid.New(), UserID: uuid.New(), Scope: mdl.RoleScope{OrgID: &otherOrgID}},
			roleStorer: &MockedRoleStorer{
				RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
					return pgrbac.RoleCustom{ID: 5, OrgID: orgID}, nil
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			want: mdl.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roleStorer := tt.roleStorer
			if roleStorer == nil {
				roleStorer = &MockedRoleStorer{}
			}
			userStorer := tt.userStorer
			if userStorer == nil {
				userStorer = &MockedUserStorer{}
			}
			orgStorer := tt.orgStorer
			if orgStorer == nil {
				orgStorer = &MockedOrgStorer{}
			}
			core := NewCore(roleStorer, userStorer, orgStorer, noopTransactor{})

			if err := core.UnassignRole(t.Context(), orgID, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("UnassignRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_ListRoleAssignments_error(t *testing.T) {
	userStorer := &MockedUserStorer{
		UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
			return pguser.User{}, sql.ErrNoRows
		},
	}
	core := NewCore(&MockedRoleStorer{}, userStorer, &MockedOrgStorer{}, noopTransactor{})

	if _, _, err := core.ListRoleAssignments(t.Context(), 1, uuid.New(), 50, 0); !errors.Is(err, mdl.ErrNotFound) {
		t.Errorf("ListRoleAssignments() error = %v, want %v", err, mdl.ErrNotFound)
	}
}

func TestCore_RolePermissions_error(t *testing.T) {
	otherOrgID := 2
	roleStorer := &MockedRoleStorer{
		RoleByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pgrbac.RoleCustom, error) {
			return pgrbac.RoleCustom{ID: 5, OrgID: otherOrgID}, nil
		},
	}
	core := NewCore(roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

	if _, _, err := core.RolePermissions(t.Context(), 1, uuid.New(), 50, 0); !errors.Is(err, mdl.ErrNotFound) {
		t.Errorf("RolePermissions() error = %v, want %v", err, mdl.ErrNotFound)
	}
}

func TestCore_StaticRolePermissions_error(t *testing.T) {
	roleStorer := &MockedRoleStorer{
		StaticRoleByNameFunc: func(_ context.Context, _ string) (pgrbac.RoleStatic, error) {
			return pgrbac.RoleStatic{}, sql.ErrNoRows
		},
	}
	core := NewCore(roleStorer, &MockedUserStorer{}, &MockedOrgStorer{}, noopTransactor{})

	if _, _, err := core.StaticRolePermissions(t.Context(), "nonexistent", 50, 0); !errors.Is(err, mdl.ErrNotFound) {
		t.Errorf("StaticRolePermissions() error = %v, want %v", err, mdl.ErrNotFound)
	}
}

func TestCore_SystemRoleAssignments_error(t *testing.T) {
	userStorer := &MockedUserStorer{
		UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
			return pguser.User{}, sql.ErrNoRows
		},
	}
	core := NewCore(&MockedRoleStorer{}, userStorer, &MockedOrgStorer{}, noopTransactor{})

	if _, _, err := core.SystemRoleAssignments(t.Context(), uuid.New(), 50, 0); !errors.Is(err, mdl.ErrNotFound) {
		t.Errorf("SystemRoleAssignments() error = %v, want %v", err, mdl.ErrNotFound)
	}
}
