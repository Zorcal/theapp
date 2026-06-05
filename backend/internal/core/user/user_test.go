package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestCore_flow(t *testing.T) {
	ctx := context.Background()
	core := NewCore(pguser.NewStore(pgtest.New(t, ctx)))

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(mdl.User{}, "ID", "ETag"),
		cmpopts.EquateApproxTime(time.Minute),
	}

	// CreateUser
	usr, err := core.CreateUser(ctx, mdl.CreateUser{Email: "alice@test.com"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	testingx.AssertDiff(t, usr, mdl.User{Email: "alice@test.com", CreatedAt: time.Now()}, diffOpts...)

	if usr.ID == (uuid.UUID{}) {
		t.Error("CreateUser() ID is zero UUID, want non-zero")
	}
	if usr.ETag == "" {
		t.Error("CreateUser() ETag is empty, want non-empty")
	}

	// ListUsers — created user appears in results
	usrs, count, err := core.ListUsers(ctx, nil, 10, 0)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}

	if count != 1 {
		t.Errorf("ListUsers() count = %d, want 1", count)
	}
	if len(usrs) != 1 {
		t.Fatalf("ListUsers() len = %d, want 1", len(usrs))
	}

	testingx.AssertDiff(t, usrs[0], usr)
}

func TestCore_CreateUser(t *testing.T) {
	now := time.Now()
	pgUsr := pguser.User{
		ExternalID: uuid.New(),
		Email:      "alice@test.com",
		CreatedAt:  now,
		ETag:       uuid.New(),
	}
	want := mdl.User{
		ID:        pgUsr.ExternalID,
		Email:     pgUsr.Email,
		CreatedAt: pgUsr.CreatedAt,
		ETag:      pgUsr.ETag.String(),
	}

	tests := []struct {
		name   string
		storer *MockedStorer
		in     mdl.CreateUser
		want   mdl.User
	}{
		{
			name: "returns converted user",
			storer: &MockedStorer{
				InsertUserFunc: func(_ context.Context, _ pguser.CreateUser) (pguser.User, error) {
					return pgUsr, nil
				},
			},
			in:   mdl.CreateUser{Email: "alice@test.com"},
			want: want,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.storer)

			got, err := core.CreateUser(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("CreateUser() error = %v", err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestCore_CreateUser_error(t *testing.T) {
	tests := []struct {
		name        string
		storer      *MockedStorer
		in          mdl.CreateUser
		wantErrStrs []string
	}{
		{
			name: "insert user error",
			storer: &MockedStorer{
				InsertUserFunc: func(_ context.Context, _ pguser.CreateUser) (pguser.User, error) {
					return pguser.User{}, errors.New("db down")
				},
			},
			in:          mdl.CreateUser{Email: "alice@test.com"},
			wantErrStrs: []string{"insert user", "db down"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.storer)

			_, err := core.CreateUser(t.Context(), tt.in)
			if err == nil {
				t.Fatalf("CreateUser() error = nil, want error")
			}

			testingx.AssertErrContains(t, err, tt.wantErrStrs...)
		})
	}
}

func TestCore_ListUsers(t *testing.T) {
	now := time.Now()
	updatedAt := now.Add(-time.Hour)

	pgAlice := pguser.User{
		ExternalID: uuid.New(),
		Email:      "alice@test.com",
		CreatedAt:  now,
		ETag:       uuid.New(),
	}
	pgBob := pguser.User{
		ExternalID: uuid.New(),
		Email:      "bob@test.com",
		CreatedAt:  now,
		UpdatedAt:  &updatedAt,
		ETag:       uuid.New(),
	}

	mdlAlice := mdl.User{
		ID:        pgAlice.ExternalID,
		Email:     pgAlice.Email,
		CreatedAt: pgAlice.CreatedAt,
		ETag:      pgAlice.ETag.String(),
	}
	mdlBob := mdl.User{
		ID:        pgBob.ExternalID,
		Email:     pgBob.Email,
		CreatedAt: pgBob.CreatedAt,
		UpdatedAt: pgBob.UpdatedAt,
		ETag:      pgBob.ETag.String(),
	}

	tests := []struct {
		name      string
		storer    *MockedStorer
		orderBys  []order.By[mdl.UserOrderByField]
		wantUsers []mdl.User
		wantCount int
	}{
		{
			name: "returns converted users and total count",
			storer: &MockedStorer{
				QueryUsersFunc: func(_ context.Context, _ []order.By[pguser.OrderByField], _, _ int) ([]pguser.User, error) {
					return []pguser.User{pgAlice, pgBob}, nil
				},
				UserCountFunc: func(_ context.Context) (int, error) {
					return 42, nil
				},
			},
			orderBys:  []order.By[mdl.UserOrderByField]{order.NewBy(mdl.UserOrderByFieldEmail, order.DirectionAsc)},
			wantUsers: []mdl.User{mdlAlice, mdlBob},
			wantCount: 42,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.storer)

			gotUsers, gotCount, err := core.ListUsers(t.Context(), tt.orderBys, 10, 0)
			if err != nil {
				t.Fatalf("ListUsers() error = %v", err)
			}

			testingx.AssertDiff(t, gotUsers, tt.wantUsers)

			if gotCount != tt.wantCount {
				t.Errorf("ListUsers() count = %d, want %d", gotCount, tt.wantCount)
			}
		})
	}
}

func TestCore_ListUsers_error(t *testing.T) {
	tests := []struct {
		name        string
		storer      *MockedStorer
		orderBys    []order.By[mdl.UserOrderByField]
		wantErrStrs []string
	}{
		{
			name:        "unknown order by field",
			storer:      &MockedStorer{},
			orderBys:    []order.By[mdl.UserOrderByField]{order.NewBy(mdl.UserOrderByField("unknown"), order.DirectionAsc)},
			wantErrStrs: []string{"convert order bys", "unknown"},
		},
		{
			name: "query users error",
			storer: &MockedStorer{
				QueryUsersFunc: func(_ context.Context, _ []order.By[pguser.OrderByField], _, _ int) ([]pguser.User, error) {
					return nil, errors.New("db down")
				},
				UserCountFunc: func(_ context.Context) (int, error) {
					return 0, nil
				},
			},
			wantErrStrs: []string{"query users", "db down"},
		},
		{
			name: "user count error",
			storer: &MockedStorer{
				QueryUsersFunc: func(_ context.Context, _ []order.By[pguser.OrderByField], _, _ int) ([]pguser.User, error) {
					return nil, nil
				},
				UserCountFunc: func(_ context.Context) (int, error) {
					return 0, errors.New("db down")
				},
			},
			wantErrStrs: []string{"user count", "db down"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := NewCore(tt.storer)

			_, _, err := core.ListUsers(t.Context(), tt.orderBys, 10, 0)
			if err == nil {
				t.Fatalf("ListUsers() error = nil, want error")
			}

			testingx.AssertErrContains(t, err, tt.wantErrStrs...)
		})
	}
}
