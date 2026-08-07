package user

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
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestCore_integration(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	rbacStore := pgrbac.NewStore(pool)
	core := NewCore(userStore, rbacStore, immediateTransactor{})

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(mdl.User{}, "ID", "ETag"),
		cmpopts.EquateApproxTime(time.Minute),
	}
	updateDiffOpts := cmp.Options{
		cmpopts.IgnoreFields(mdl.User{}, "ID", "ETag", "UpdatedAt"),
		cmpopts.EquateApproxTime(time.Minute),
	}

	// CreateUser
	usr, err := core.CreateUser(ctx, mdl.CreateUser{
		Email: "alice@test.com",
		Name:  "Alice Smith",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	testingx.AssertDiff(t, usr, mdl.User{
		Email:     "alice@test.com",
		Name:      "Alice Smith",
		CreatedAt: time.Now(),
	}, diffOpts...)

	if usr.ID == (uuid.UUID{}) {
		t.Error("CreateUser() ID is zero UUID, want non-zero")
	}
	if usr.ETag == "" {
		t.Error("CreateUser() ETag is empty, want non-empty")
	}

	// UpdateUser — name is changed, updated_at is set
	updated, err := core.UpdateUser(ctx, mdl.UpdateUser{
		ID:     usr.ID,
		Name:   "Alice Jones",
		Fields: mdl.UserUpdateFields{Name: true},
	})
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	testingx.AssertDiff(t, updated, mdl.User{
		Email:     "alice@test.com",
		Name:      "Alice Jones",
		CreatedAt: time.Now(),
	}, updateDiffOpts...)
	if updated.UpdatedAt == nil {
		t.Error("UpdateUser() UpdatedAt = nil, want non-nil")
	}

	// UserByID — returns the updated user
	got, err := core.UserByID(ctx, usr.ID)
	if err != nil {
		t.Fatalf("UserByID(%v) error = %v", usr.ID, err)
	}

	testingx.AssertDiff(t, got, updated)

	// Users — updated user appears in filtered results
	usrs, count, err := core.Users(ctx, mdl.UserFilter{Name: " Alice "}, nil, 10, 0)
	if err != nil {
		t.Fatalf("Users() error = %v", err)
	}

	if count != 1 {
		t.Errorf("Users() count = %d, want 1", count)
	}
	if len(usrs) != 1 {
		t.Fatalf("Users() len = %d, want 1", len(usrs))
	}

	testingx.AssertDiff(t, usrs[0], updated)
}

func TestCore_UserByID(t *testing.T) {
	id, etag, now := uuid.New(), uuid.New(), time.Now()

	tests := []struct {
		name       string
		userStorer *MockedUserStorer
		in         uuid.UUID
		want       mdl.User
	}{
		{
			name: "returns converted user",
			userStorer: &MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{
						ExternalID: id,
						Email:      "alice@test.com",
						Name:       "Alice Smith",
						CreatedAt:  now,
						ETag:       etag,
					}, nil
				},
			},
			in: id,
			want: mdl.User{
				ID:        id,
				Email:     "alice@test.com",
				Name:      "Alice Smith",
				CreatedAt: now,
				ETag:      etag.String(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, nil, immediateTransactor{})

			got, err := core.UserByID(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("UserByID(%v) error = %v", tt.in, err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestCore_UserByID_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name    string
		mockErr error
		want    error
	}{
		{
			name:    "not found",
			mockErr: sql.ErrNoRows,
			want:    mdl.ErrNotFound,
		},
		{
			name:    "store error",
			mockErr: dbErr,
			want:    dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(&MockedUserStorer{
				UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, tt.mockErr
				},
			}, nil, immediateTransactor{})

			if _, err := core.UserByID(t.Context(), uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("UserByID() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_UserByEmail(t *testing.T) {
	id, etag, now := uuid.New(), uuid.New(), time.Now()

	tests := []struct {
		name       string
		userStorer *MockedUserStorer
		in         string
		want       mdl.User
	}{
		{
			name: "returns converted user",
			userStorer: &MockedUserStorer{
				UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
					return pguser.User{
						ExternalID: id,
						Email:      "alice@test.com",
						Name:       "Alice Smith",
						CreatedAt:  now,
						ETag:       etag,
					}, nil
				},
			},
			in: "alice@test.com",
			want: mdl.User{
				ID:        id,
				Email:     "alice@test.com",
				Name:      "Alice Smith",
				CreatedAt: now,
				ETag:      etag.String(),
			},
		},
		{
			name: "normalizes email before lookup",
			userStorer: &MockedUserStorer{
				UserByEmailFunc: func(_ context.Context, email string) (pguser.User, error) {
					return pguser.User{
						ExternalID: id,
						Email:      email,
						Name:       "Alice Smith",
						CreatedAt:  now,
						ETag:       etag,
					}, nil
				},
			},
			in: "  Alice@Test.COM  ",
			want: mdl.User{
				ID:        id,
				Email:     "alice@test.com",
				Name:      "Alice Smith",
				CreatedAt: now,
				ETag:      etag.String(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, nil, immediateTransactor{})

			got, err := core.UserByEmail(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("UserByEmail(%v) error = %v", tt.in, err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestCore_UserByEmail_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name    string
		mockErr error
		want    error
	}{
		{
			name:    "not found",
			mockErr: sql.ErrNoRows,
			want:    mdl.ErrNotFound,
		},
		{
			name:    "store error",
			mockErr: dbErr,
			want:    dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(&MockedUserStorer{
				UserByEmailFunc: func(_ context.Context, _ string) (pguser.User, error) {
					return pguser.User{}, tt.mockErr
				},
			}, nil, immediateTransactor{})

			if _, err := core.UserByEmail(t.Context(), "alice@test.com"); !errors.Is(err, tt.want) {
				t.Errorf("UserByEmail() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_UpdateUser(t *testing.T) {
	id, etag, now := uuid.New(), uuid.New(), time.Now()

	tests := []struct {
		name       string
		userStorer *MockedUserStorer
		in         mdl.UpdateUser
		want       mdl.User
	}{
		{
			name: "returns converted user",
			userStorer: &MockedUserStorer{
				UpdateUserFunc: func(_ context.Context, _ pguser.UpdateUser) (pguser.User, error) {
					return pguser.User{
						ExternalID: id,
						Email:      "alice@test.com",
						Name:       "Alice Updated",
						CreatedAt:  now,
						ETag:       etag,
					}, nil
				},
			},
			in: mdl.UpdateUser{
				ID:     id,
				Name:   "Alice Updated",
				Fields: mdl.UserUpdateFields{Name: true},
			},
			want: mdl.User{
				ID:        id,
				Email:     "alice@test.com",
				Name:      "Alice Updated",
				CreatedAt: now,
				ETag:      etag.String(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, nil, immediateTransactor{})

			got, err := core.UpdateUser(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("UpdateUser(%v) error = %v", tt.in, err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestCore_UpdateUser_error(t *testing.T) {
	tests := []struct {
		name       string
		userStorer *MockedUserStorer
		in         mdl.UpdateUser
		want       error
	}{
		{
			name:       "invalid input",
			userStorer: &MockedUserStorer{},
			in: mdl.UpdateUser{
				ID:     uuid.New(),
				Name:   "",
				Fields: mdl.UserUpdateFields{Name: true},
			},
			want: mdl.ErrValidation,
		},
		{
			name: "not found",
			userStorer: &MockedUserStorer{
				UpdateUserFunc: func(_ context.Context, _ pguser.UpdateUser) (pguser.User, error) {
					return pguser.User{}, sql.ErrNoRows
				},
			},
			in: mdl.UpdateUser{
				ID:     uuid.New(),
				Name:   "Alice Updated",
				Fields: mdl.UserUpdateFields{Name: true},
			},
			want: mdl.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, nil, immediateTransactor{})

			if _, err := core.UpdateUser(t.Context(), tt.in); !errors.Is(err, tt.want) {
				t.Errorf("UpdateUser() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("store error", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{
			UpdateUserFunc: func(_ context.Context, _ pguser.UpdateUser) (pguser.User, error) {
				return pguser.User{}, errors.New("db error")
			},
		}, nil, immediateTransactor{})
		_, err := core.UpdateUser(t.Context(), mdl.UpdateUser{
			ID:     uuid.New(),
			Name:   "Alice Updated",
			Fields: mdl.UserUpdateFields{Name: true},
		})
		if err == nil {
			t.Fatal("UpdateUser() error = nil, want error")
		}
		testingx.AssertErrContains(t, err, "update user", "db error")
	})
}

func TestCore_CreateUser(t *testing.T) {
	id, etag, now := uuid.New(), uuid.New(), time.Now()

	tests := []struct {
		name       string
		userStorer *MockedUserStorer
		in         mdl.CreateUser
		want       mdl.User
	}{
		{
			name: "returns converted user",
			userStorer: &MockedUserStorer{
				CreateUserFunc: func(_ context.Context, _ pguser.CreateUser) (pguser.User, error) {
					return pguser.User{
						ExternalID: id,
						Email:      "alice@test.com",
						Name:       "Alice Smith",
						CreatedAt:  now,
						ETag:       etag,
					}, nil
				},
			},
			in: mdl.CreateUser{
				Email: "alice@test.com",
				Name:  "Alice Smith",
			},
			want: mdl.User{
				ID:        id,
				Email:     "alice@test.com",
				Name:      "Alice Smith",
				CreatedAt: now,
				ETag:      etag.String(),
			},
		},
		{
			name: "normalizes email before storing",
			userStorer: &MockedUserStorer{
				CreateUserFunc: func(_ context.Context, cu pguser.CreateUser) (pguser.User, error) {
					return pguser.User{
						ExternalID: id,
						Email:      cu.Email,
						Name:       cu.Name,
						CreatedAt:  now,
						ETag:       etag,
					}, nil
				},
			},
			in: mdl.CreateUser{
				Email: "  Alice@Test.COM  ",
				Name:  "Alice Smith",
			},
			want: mdl.User{
				ID:        id,
				Email:     "alice@test.com",
				Name:      "Alice Smith",
				CreatedAt: now,
				ETag:      etag.String(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, nil, immediateTransactor{})

			got, err := core.CreateUser(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("CreateUser() error = %v", err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestCore_CreateUser_error(t *testing.T) {
	in := mdl.CreateUser{
		Email: "alice@test.com",
		Name:  "Alice Smith",
	}

	tests := []struct {
		name       string
		userStorer *MockedUserStorer
		in         mdl.CreateUser
		want       error
	}{
		{
			name:       "invalid input",
			userStorer: &MockedUserStorer{},
			in:         mdl.CreateUser{Email: "", Name: "Alice Smith"},
			want:       mdl.ErrValidation,
		},
		{
			name: "duplicate email",
			userStorer: &MockedUserStorer{
				CreateUserFunc: func(_ context.Context, _ pguser.CreateUser) (pguser.User, error) {
					return pguser.User{}, pgdb.ErrAlreadyExists
				},
			},
			in:   in,
			want: mdl.ErrAlreadyExists,
		},
		{
			name: "deleted email",
			userStorer: &MockedUserStorer{
				CreateUserFunc: func(_ context.Context, _ pguser.CreateUser) (pguser.User, error) {
					return pguser.User{}, pguser.ErrDeleted
				},
			},
			in:   in,
			want: mdl.ErrUserDeleted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, nil, immediateTransactor{})

			if _, err := core.CreateUser(t.Context(), tt.in); !errors.Is(err, tt.want) {
				t.Errorf("CreateUser() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("store error", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{
			CreateUserFunc: func(_ context.Context, _ pguser.CreateUser) (pguser.User, error) {
				return pguser.User{}, errors.New("db error")
			},
		}, nil, immediateTransactor{})
		_, err := core.CreateUser(t.Context(), in)
		if err == nil {
			t.Fatalf("CreateUser() error = nil, want error")
		}
		testingx.AssertErrContains(t, err, "create user", "db error")
	})
}

func TestCore_Users(t *testing.T) {
	now := time.Now()
	updatedAt := now.Add(-time.Hour)
	aliceID, aliceETag := uuid.New(), uuid.New()
	bobID, bobETag := uuid.New(), uuid.New()

	tests := []struct {
		name       string
		userStorer *MockedUserStorer
		orderBys   []order.By[mdl.UserOrderByField]
		wantUsers  []mdl.User
		wantCount  int
	}{
		{
			name: "returns converted users and total count",
			userStorer: &MockedUserStorer{
				UsersFunc: func(_ context.Context, _ pguser.Filter, _ []order.By[pguser.OrderByField], _, _ int) ([]pguser.User, int, error) {
					return []pguser.User{
						{
							ExternalID: aliceID,
							Email:      "alice@test.com",
							Name:       "Alice Smith",
							CreatedAt:  now,
							ETag:       aliceETag,
						},
						{
							ExternalID: bobID,
							Email:      "bob@test.com",
							Name:       "Bob Jones",
							CreatedAt:  now,
							UpdatedAt:  &updatedAt,
							ETag:       bobETag,
						},
					}, 42, nil
				},
			},
			orderBys: []order.By[mdl.UserOrderByField]{order.NewBy(mdl.UserOrderByFieldEmail, order.DirectionAsc)},
			wantUsers: []mdl.User{
				{
					ID:        aliceID,
					Email:     "alice@test.com",
					Name:      "Alice Smith",
					CreatedAt: now,
					ETag:      aliceETag.String(),
				},
				{
					ID:        bobID,
					Email:     "bob@test.com",
					Name:      "Bob Jones",
					CreatedAt: now,
					UpdatedAt: &updatedAt,
					ETag:      bobETag.String(),
				},
			},
			wantCount: 42,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, nil, immediateTransactor{})

			gotUsers, gotCount, err := core.Users(t.Context(), mdl.UserFilter{}, tt.orderBys, 10, 0)
			if err != nil {
				t.Fatalf("Users() error = %v", err)
			}

			testingx.AssertDiff(t, gotUsers, tt.wantUsers)

			if gotCount != tt.wantCount {
				t.Errorf("Users() count = %d, want %d", gotCount, tt.wantCount)
			}
		})
	}
}

func TestCore_Users_error(t *testing.T) {
	tests := []struct {
		name        string
		userStorer  *MockedUserStorer
		orderBys    []order.By[mdl.UserOrderByField]
		wantErrStrs []string
	}{
		{
			name:        "unknown order by field",
			userStorer:  &MockedUserStorer{},
			orderBys:    []order.By[mdl.UserOrderByField]{order.NewBy(mdl.UserOrderByField("unknown"), order.DirectionAsc)},
			wantErrStrs: []string{"unknown"},
		},
		{
			name: "query users error",
			userStorer: &MockedUserStorer{
				UsersFunc: func(_ context.Context, _ pguser.Filter, _ []order.By[pguser.OrderByField], _, _ int) ([]pguser.User, int, error) {
					return nil, 0, errors.New("db error")
				},
			},
			wantErrStrs: []string{"query users", "db error"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, nil, immediateTransactor{})

			_, _, err := core.Users(t.Context(), mdl.UserFilter{}, tt.orderBys, 10, 0)
			if err == nil {
				t.Fatalf("Users() error = nil, want error")
			}

			testingx.AssertErrContains(t, err, tt.wantErrStrs...)
		})
	}
}

func TestCore_DeleteUser(t *testing.T) {
	userStore := &MockedUserStorer{
		SoftDeleteUserFunc: func(_ context.Context, _ uuid.UUID) error { return nil },
	}
	rbacStore := &MockedRBACStorer{
		LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
		FullyPrivilegedUserRemainsAfterDeleteFunc: func(_ context.Context, _ uuid.UUID) (bool, error) {
			return true, nil
		},
	}
	core := NewCore(userStore, rbacStore, immediateTransactor{})

	if err := core.DeleteUser(t.Context(), uuid.New()); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
}

func TestCore_DeleteUser_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		userStorer *MockedUserStorer
		rbacStorer *MockedRBACStorer
		transactor Transactor
		want       error
	}{
		{
			name:       "transaction",
			userStorer: &MockedUserStorer{},
			rbacStorer: nil,
			transactor: errorTransactor{err: dbErr},
			want:       dbErr,
		},
		{
			name:       "lock system-role management",
			userStorer: &MockedUserStorer{},
			rbacStorer: &MockedRBACStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return dbErr },
			},
			transactor: immediateTransactor{},
			want:       dbErr,
		},
		{
			name:       "user not found",
			userStorer: &MockedUserStorer{},
			rbacStorer: &MockedRBACStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				FullyPrivilegedUserRemainsAfterDeleteFunc: func(_ context.Context, _ uuid.UUID) (bool, error) {
					return false, sql.ErrNoRows
				},
			},
			transactor: immediateTransactor{},
			want:       mdl.ErrNotFound,
		},
		{
			name:       "recovery check",
			userStorer: &MockedUserStorer{},
			rbacStorer: &MockedRBACStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				FullyPrivilegedUserRemainsAfterDeleteFunc: func(_ context.Context, _ uuid.UUID) (bool, error) {
					return false, dbErr
				},
			},
			transactor: immediateTransactor{},
			want:       dbErr,
		},
		{
			name:       "last fully privileged system administrator",
			userStorer: &MockedUserStorer{},
			rbacStorer: &MockedRBACStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				FullyPrivilegedUserRemainsAfterDeleteFunc: func(_ context.Context, _ uuid.UUID) (bool, error) {
					return false, nil
				},
			},
			transactor: immediateTransactor{},
			want:       mdl.ErrLastFullyPrivilegedSystemAdmin,
		},
		{
			name: "soft delete",
			userStorer: &MockedUserStorer{
				SoftDeleteUserFunc: func(_ context.Context, _ uuid.UUID) error {
					// The user was found by the recovery check earlier in the transaction, so
					// sql.ErrNoRows is deliberately preserved as an internal error.
					return sql.ErrNoRows
				},
			},
			rbacStorer: &MockedRBACStorer{
				LockSystemRoleManagementFunc: func(_ context.Context) error { return nil },
				FullyPrivilegedUserRemainsAfterDeleteFunc: func(_ context.Context, _ uuid.UUID) (bool, error) {
					return true, nil
				},
			},
			transactor: immediateTransactor{},
			want:       sql.ErrNoRows,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, tt.rbacStorer, tt.transactor)

			if err := core.DeleteUser(t.Context(), uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("DeleteUser() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCore_RestoreUser(t *testing.T) {
	now := time.Now()
	id := uuid.New()
	etag := uuid.New()
	core := NewCore(&MockedUserStorer{
		RestoreUserFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
			return pguser.User{ExternalID: id, Email: "restored@test.com", Name: "Restored", CreatedAt: now, ETag: etag}, nil
		},
	}, nil, immediateTransactor{})

	got, err := core.RestoreUser(t.Context(), id)
	if err != nil {
		t.Fatalf("RestoreUser() error = %v", err)
	}

	testingx.AssertDiff(t, got, mdl.User{ID: id, Email: "restored@test.com", Name: "Restored", CreatedAt: now, ETag: etag.String()})
}

func TestCore_RestoreUser_error(t *testing.T) {
	dbErr := errors.New("db error")

	tests := []struct {
		name       string
		userStorer *MockedUserStorer
		want       error
	}{
		{
			name: "not found",
			userStorer: &MockedUserStorer{
				RestoreUserFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, sql.ErrNoRows
				},
			},
			want: mdl.ErrNotFound,
		},
		{
			name: "store",
			userStorer: &MockedUserStorer{
				RestoreUserFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
					return pguser.User{}, dbErr
				},
			},
			want: dbErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer, nil, immediateTransactor{})

			if _, err := core.RestoreUser(t.Context(), uuid.New()); !errors.Is(err, tt.want) {
				t.Errorf("RestoreUser() error = %v, want %v", err, tt.want)
			}
		})
	}
}

type immediateTransactor struct{}

func (immediateTransactor) RunTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type errorTransactor struct{ err error }

func (tr errorTransactor) RunTx(_ context.Context, _ func(context.Context) error) error {
	return tr.err
}
