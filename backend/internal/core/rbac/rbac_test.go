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
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestCore_integration(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	core := NewCore(pgrbac.NewStore(pool), userStore)

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

	wantSystemRoles := []mdl.SystemRole{
		{Name: "superadmin", Permissions: mdl.AllPermissions},
	}

	testingx.AssertDiff(t, systemRoles, wantSystemRoles, diffOpts)

	// Create user.

	usr, err := userStore.CreateUser(ctx, pguser.CreateUser{Email: "alice@test.com", Name: "Alice Smith"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	// Assign a system role and list the user's assignments.

	if err := core.AssignSystemRole(ctx, usr.ExternalID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}

	gotAssignedRoles, gotAssignedCount, err := core.UserSystemRoles(ctx, usr.ExternalID, 50, 0)
	if err != nil {
		t.Fatalf("UserSystemRoles() after assignment error = %v", err)
	}
	if wantAssignedCount := 1; gotAssignedCount != wantAssignedCount {
		t.Errorf("UserSystemRoles() after assignment total count = %d, want %d", gotAssignedCount, wantAssignedCount)
	}

	wantAssignedRoles := []mdl.SystemRole{
		{Name: "superadmin", Permissions: mdl.AllPermissions},
	}

	testingx.AssertDiff(t, gotAssignedRoles, wantAssignedRoles, diffOpts)

	// Unassign the system role and verify that the assignment was removed.

	if err := core.UnassignSystemRole(ctx, usr.ExternalID, "superadmin"); err != nil {
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
	core := NewCore(roleStorer, &MockedUserStorer{})

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
			core := NewCore(tt.roleStorer, &MockedUserStorer{})

			if _, _, err := core.SystemRoles(t.Context(), 25, 0); !errors.Is(err, dbErr) {
				t.Errorf("SystemRoles() error = %v, want %v", err, dbErr)
			}
		})
	}
}

func TestCore_UserSystemRoles(t *testing.T) {
	userID := uuid.New()
	roleStorer := &MockedRoleStorer{
		UserSystemRolesFunc: func(_ context.Context, _, _, _ int) ([]pgrbac.SystemRole, error) {
			return []pgrbac.SystemRole{
				{Name: "test-role", PermissionNames: []string{"user:read"}},
			}, nil
		},
		UserSystemRoleCountFunc: func(_ context.Context, _ int) (int, error) {
			return 1, nil
		},
	}
	userStorer := &MockedUserStorer{
		UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
			return pguser.User{ID: 7}, nil
		},
	}
	core := NewCore(roleStorer, userStorer)

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
			name:       "user lookup store error",
			roleStorer: &MockedRoleStorer{},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "user roles store error",
			roleStorer: &MockedRoleStorer{
				UserSystemRolesFunc: func(_ context.Context, _, _, _ int) ([]pgrbac.SystemRole, error) {
					return nil, dbErr
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			want: dbErr,
		},
		{
			name: "user role count store error",
			roleStorer: &MockedRoleStorer{
				UserSystemRolesFunc: func(_ context.Context, _, _, _ int) ([]pgrbac.SystemRole, error) {
					return nil, nil
				},
				UserSystemRoleCountFunc: func(_ context.Context, _ int) (int, error) {
					return 0, dbErr
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
			core := NewCore(tt.roleStorer, tt.userStorer)

			if _, _, err := core.UserSystemRoles(t.Context(), uuid.New(), 25, 0); !errors.Is(err, tt.want) {
				t.Errorf("UserSystemRoles() error = %v, want %v", err, tt.want)
			}
		})
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
	core := NewCore(roleStorer, userStorer)

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
			name: "role already assigned",
			roleStorer: &MockedRoleStorer{
				AssignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
					return pgdb.ErrAlreadyExists
				},
			},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{ID: 7}, nil
				},
			},
			want: mdl.ErrAlreadyExists,
		},
		{
			name:       "user lookup store error",
			roleStorer: &MockedRoleStorer{},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role assignment store error",
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
			core := NewCore(tt.roleStorer, tt.userStorer)

			if err := core.AssignSystemRole(t.Context(), uuid.New(), "superadmin"); !errors.Is(err, tt.want) {
				t.Errorf("AssignSystemRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_UnassignSystemRole(t *testing.T) {
	userStorer := &MockedUserStorer{
		UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
			return pguser.User{ID: 7}, nil
		},
	}
	roleStorer := &MockedRoleStorer{
		UnassignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
			return nil
		},
	}
	core := NewCore(roleStorer, userStorer)

	if err := core.UnassignSystemRole(t.Context(), uuid.New(), "test-role"); err != nil {
		t.Fatalf("UnassignSystemRole() error = %v", err)
	}
}

func TestCore_UnassignSystemRole_error(t *testing.T) {
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
			name: "assignment not found",
			roleStorer: &MockedRoleStorer{
				UnassignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
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
			name:       "user lookup store error",
			roleStorer: &MockedRoleStorer{},
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, dbErr
				},
			},
			want: dbErr,
		},
		{
			name: "role unassignment store error",
			roleStorer: &MockedRoleStorer{
				UnassignSystemRoleFunc: func(_ context.Context, _ int, _ string) error {
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
			core := NewCore(tt.roleStorer, tt.userStorer)

			if err := core.UnassignSystemRole(t.Context(), uuid.New(), "test-role"); !errors.Is(err, tt.want) {
				t.Errorf("UnassignSystemRole() error = %v, want %v", err, tt.want)
			}
		})
	}
}
