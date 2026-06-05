package pguser

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_InsertUser(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	got, err := store.InsertUser(ctx, CreateUser{Email: "alice@test.com"})
	if err != nil {
		t.Fatalf("InsertUser() error = %v", err)
	}

	want := User{
		Email:     "alice@test.com",
		CreatedAt: time.Now(),
		UpdatedAt: nil,
	}

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(User{}, "ExternalID", "ETag"), // Ignore generated fields
		cmpopts.EquateApproxTime(time.Minute),
	}
	testingx.AssertDiff(t, got, want, diffOpts...)

	if got.ExternalID == (uuid.UUID{}) {
		t.Error("InsertUser() ExternalID is zero UUID, want non-zero")
	}
	if got.ETag == (uuid.UUID{}) {
		t.Error("InsertUser() ETag is zero UUID, want non-zero")
	}
	if got.ExternalID == got.ETag {
		t.Errorf("InsertUser() ExternalID and ETag are equal (%v), want distinct UUIDs", got.ExternalID)
	}
}

func TestStore_UserCount(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	got, err := store.UserCount(ctx)
	if err != nil {
		t.Fatalf("UserCount() error = %v", err)
	}
	if got != 0 {
		t.Errorf("UserCount() = %d, want 0 on empty database", got)
	}

	seedUser(t, store, "alice@test.com")
	seedUser(t, store, "bob@test.com")

	got, err = store.UserCount(ctx)
	if err != nil {
		t.Fatalf("UserCount() error = %v", err)
	}
	if want := 2; got != want {
		t.Errorf("UserCount() = %d, want %d", got, want)
	}
}

func TestStore_QueryUsers(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	charlie := seedUser(t, store, "charlie@test.com")
	alice := seedUser(t, store, "alice@test.com")
	bob := seedUser(t, store, "bob@test.com")

	diffOpts := cmp.Options{
		cmpopts.EquateApproxTime(time.Minute),
	}

	tests := []struct {
		name       string
		orderBys   []order.By[OrderByField]
		pageSize   int
		pageOffset int
		want       []User
	}{
		{
			name:       "no order defaults to insert order",
			orderBys:   nil,
			pageSize:   10,
			pageOffset: 0,
			want:       []User{charlie, alice, bob},
		},
		{
			name:       "order by email asc",
			orderBys:   []order.By[OrderByField]{order.NewBy(OrderByFieldEmail, order.DirectionAsc)},
			pageSize:   10,
			pageOffset: 0,
			want:       []User{alice, bob, charlie},
		},
		{
			name:       "order by email desc",
			orderBys:   []order.By[OrderByField]{order.NewBy(OrderByFieldEmail, order.DirectionDesc)},
			pageSize:   10,
			pageOffset: 0,
			want:       []User{charlie, bob, alice},
		},
		{
			name:       "first page",
			orderBys:   []order.By[OrderByField]{order.NewBy(OrderByFieldEmail, order.DirectionAsc)},
			pageSize:   2,
			pageOffset: 0,
			want:       []User{alice, bob},
		},
		{
			name:       "second page",
			orderBys:   []order.By[OrderByField]{order.NewBy(OrderByFieldEmail, order.DirectionAsc)},
			pageSize:   2,
			pageOffset: 2,
			want:       []User{charlie},
		},
		{
			name:       "offset past end returns empty",
			orderBys:   nil,
			pageSize:   10,
			pageOffset: 10,
			want:       []User{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.QueryUsers(ctx, tt.orderBys, tt.pageSize, tt.pageOffset)
			if err != nil {
				t.Fatalf("QueryUsers() error = %v", err)
			}

			testingx.AssertDiff(t, got, tt.want, diffOpts...)
		})
	}
}

func seedUser(t *testing.T, s *Store, email string) User {
	t.Helper()

	seeded, err := s.InsertUser(t.Context(), CreateUser{Email: email})
	if err != nil {
		t.Fatalf("seed user error: %v", err)
	}

	return seeded
}
