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
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestCore_integration(t *testing.T) {
	ctx := context.Background()
	core := NewCore(pguser.NewStore(pgtest.New(t, ctx)))

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

	// Users — updated user appears in results
	usrs, count, err := core.Users(ctx, mdl.UserFilter{}, nil, 10, 0)
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
			core := NewCore(tt.userStorer)

			got, err := core.UserByID(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("UserByID(%v) error = %v", tt.in, err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestCore_UserByID_error(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{
			UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
				return pguser.User{}, sql.ErrNoRows
			},
		})
		_, err := core.UserByID(t.Context(), uuid.New())
		if !errors.Is(err, mdl.ErrNotFound) {
			t.Errorf("UserByID() error = %v, want mdl.ErrNotFound", err)
		}
	})

	t.Run("store error", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{
			UserByExternalIDFunc: func(_ context.Context, _ uuid.UUID) (pguser.User, error) {
				return pguser.User{}, errors.New("db down")
			},
		})
		_, err := core.UserByID(t.Context(), uuid.New())
		if err == nil {
			t.Fatal("UserByID() error = nil, want error")
		}
		testingx.AssertErrContains(t, err, "db down")
	})
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
			core := NewCore(tt.userStorer)

			got, err := core.UpdateUser(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("UpdateUser(%v) error = %v", tt.in, err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestCore_UpdateUser_error(t *testing.T) {
	t.Run("invalid input", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{})

		_, err := core.UpdateUser(t.Context(), mdl.UpdateUser{
			ID:     uuid.New(),
			Name:   "",
			Fields: mdl.UserUpdateFields{Name: true},
		})
		if !errors.Is(err, mdl.ErrValidation) {
			t.Errorf("UpdateUser() error = %v, want mdl.ErrValidation", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{
			UpdateUserFunc: func(_ context.Context, _ pguser.UpdateUser) (pguser.User, error) {
				return pguser.User{}, sql.ErrNoRows
			},
		})
		_, err := core.UpdateUser(t.Context(), mdl.UpdateUser{
			ID:     uuid.New(),
			Name:   "Alice Updated",
			Fields: mdl.UserUpdateFields{Name: true},
		})
		if !errors.Is(err, mdl.ErrNotFound) {
			t.Errorf("UpdateUser() error = %v, want mdl.ErrNotFound", err)
		}
	})

	t.Run("store error", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{
			UpdateUserFunc: func(_ context.Context, _ pguser.UpdateUser) (pguser.User, error) {
				return pguser.User{}, errors.New("db down")
			},
		})
		_, err := core.UpdateUser(t.Context(), mdl.UpdateUser{
			ID:     uuid.New(),
			Name:   "Alice Updated",
			Fields: mdl.UserUpdateFields{Name: true},
		})
		if err == nil {
			t.Fatal("UpdateUser() error = nil, want error")
		}
		testingx.AssertErrContains(t, err, "update user", "db down")
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
			core := NewCore(tt.userStorer)

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

	t.Run("invalid input", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{})

		_, err := core.CreateUser(t.Context(), mdl.CreateUser{Email: "", Name: "Alice Smith"})
		if !errors.Is(err, mdl.ErrValidation) {
			t.Errorf("CreateUser() error = %v, want mdl.ErrValidation", err)
		}
	})

	t.Run("duplicate email", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{
			CreateUserFunc: func(_ context.Context, _ pguser.CreateUser) (pguser.User, error) {
				return pguser.User{}, pgdb.ErrAlreadyExists
			},
		})
		_, err := core.CreateUser(t.Context(), in)
		if !errors.Is(err, mdl.ErrAlreadyExists) {
			t.Errorf("CreateUser() error = %v, want mdl.ErrAlreadyExists", err)
		}
	})

	t.Run("store error", func(t *testing.T) {
		core := NewCore(&MockedUserStorer{
			CreateUserFunc: func(_ context.Context, _ pguser.CreateUser) (pguser.User, error) {
				return pguser.User{}, errors.New("db down")
			},
		})
		_, err := core.CreateUser(t.Context(), in)
		if err == nil {
			t.Fatalf("CreateUser() error = nil, want error")
		}
		testingx.AssertErrContains(t, err, "create user", "db down")
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
				UsersFunc: func(_ context.Context, _ pguser.Filter, _ []order.By[pguser.OrderByField], _, _ int) ([]pguser.User, error) {
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
					}, nil
				},
				UserCountFunc: func(_ context.Context, _ pguser.Filter) (int, error) {
					return 42, nil
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
			core := NewCore(tt.userStorer)

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
				UsersFunc: func(_ context.Context, _ pguser.Filter, _ []order.By[pguser.OrderByField], _, _ int) ([]pguser.User, error) {
					return nil, errors.New("db down")
				},
				UserCountFunc: func(_ context.Context, _ pguser.Filter) (int, error) {
					return 0, nil
				},
			},
			wantErrStrs: []string{"query users", "db down"},
		},
		{
			name: "user count error",
			userStorer: &MockedUserStorer{
				UsersFunc: func(_ context.Context, _ pguser.Filter, _ []order.By[pguser.OrderByField], _, _ int) ([]pguser.User, error) {
					return nil, nil
				},
				UserCountFunc: func(_ context.Context, _ pguser.Filter) (int, error) {
					return 0, errors.New("db down")
				},
			},
			wantErrStrs: []string{"user count", "db down"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.userStorer)

			_, _, err := core.Users(t.Context(), mdl.UserFilter{}, tt.orderBys, 10, 0)
			if err == nil {
				t.Fatalf("Users() error = nil, want error")
			}

			testingx.AssertErrContains(t, err, tt.wantErrStrs...)
		})
	}
}
