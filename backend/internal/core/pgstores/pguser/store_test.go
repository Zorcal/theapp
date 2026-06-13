package pguser

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_UserByEmail(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	seeded := seedUser(t, store, "alice@test.com", "Alice Smith")

	got, err := store.UserByEmail(ctx, seeded.Email)
	if err != nil {
		t.Fatalf("UserByEmail(%q) error = %v", seeded.Email, err)
	}

	testingx.AssertDiff(t, got, seeded)
}

func TestStore_UserByEmail_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		_, err := store.UserByEmail(ctx, "nobody@test.com")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UserByEmail(nobody) error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_UserByExternalID(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	seeded := seedUser(t, store, "alice@test.com", "Alice Smith")

	got, err := store.UserByExternalID(ctx, seeded.ExternalID)
	if err != nil {
		t.Fatalf("UserByExternalID(%v) error = %v", seeded.ExternalID, err)
	}

	testingx.AssertDiff(t, got, seeded)
}

func TestStore_UserByExternalID_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		id := uuid.New()
		_, err := store.UserByExternalID(ctx, id)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UserByExternalID(%v) error = %v, want sql.ErrNoRows", id, err)
		}
	})
}

func TestStore_CreateUser(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	got, err := store.CreateUser(ctx, CreateUser{
		Email: "alice@test.com",
		Name:  "Alice Smith",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	want := User{
		Email:     "alice@test.com",
		Name:      "Alice Smith",
		CreatedAt: time.Now(),
		UpdatedAt: nil,
	}

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(User{}, "ID", "ExternalID", "ETag"), // Ignore generated fields
		cmpopts.EquateApproxTime(time.Minute),
	}
	testingx.AssertDiff(t, got, want, diffOpts...)

	if got.ID == 0 {
		t.Error("CreateUser() ID = 0, want non-zero")
	}
	if got.ExternalID == (uuid.UUID{}) {
		t.Error("CreateUser() ExternalID is zero UUID, want non-zero")
	}
	if got.ETag == (uuid.UUID{}) {
		t.Error("CreateUser() ETag is zero UUID, want non-zero")
	}
	if got.ExternalID == got.ETag {
		t.Errorf("CreateUser() ExternalID and ETag are equal (%v), want distinct UUIDs", got.ExternalID)
	}
}

func TestStore_UserCount(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	seedUser(t, store, "alice@test.com", "Alice Smith")
	seedUser(t, store, "bob@test.com", "Bob Jones")

	tests := []struct {
		name   string
		filter Filter
		want   int
	}{
		{
			name: "no filter counts all",
			want: 2,
		},
		{
			name:   "email prefix filter",
			filter: Filter{Email: "alice"},
			want:   1,
		},
		{
			name:   "name prefix filter",
			filter: Filter{Name: "Bob"},
			want:   1,
		},
		{
			name:   "filter with no matches",
			filter: Filter{Email: "nobody"},
			want:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.UserCount(ctx, tt.filter)
			if err != nil {
				t.Fatalf("UserCount() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("UserCount(%+v) = %d, want %d", tt.filter, got, tt.want)
			}
		})
	}
}

func TestStore_Users(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	charlie := seedUser(t, store, "charlie@test.com", "Charlie Brown")
	alice := seedUser(t, store, "alice@test.com", "Alice Smith")
	bob := seedUser(t, store, "bob@test.com", "Bob Jones")

	diffOpts := cmp.Options{
		cmpopts.EquateApproxTime(time.Minute),
	}

	tests := []struct {
		name       string
		filter     Filter
		orderBys   []order.By[OrderByField]
		pageSize   int
		pageOffset int
		want       []User
	}{
		{
			name:       "no order defaults to insert order",
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
			pageSize:   10,
			pageOffset: 10,
			want:       []User{},
		},
		{
			name:       "filter by email prefix",
			filter:     Filter{Email: "alice"},
			pageSize:   10,
			pageOffset: 0,
			want:       []User{alice},
		},
		{
			name:       "filter by name prefix",
			filter:     Filter{Name: "Bob"},
			pageSize:   10,
			pageOffset: 0,
			want:       []User{bob},
		},
		{
			name: "filter by email and name prefix",
			filter: Filter{
				Email: "c",
				Name:  "Charlie",
			},
			pageSize:   10,
			pageOffset: 0,
			want:       []User{charlie},
		},
		{
			name:       "filter with no matches returns empty",
			filter:     Filter{Email: "nobody"},
			pageSize:   10,
			pageOffset: 0,
			want:       []User{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.Users(ctx, tt.filter, tt.orderBys, tt.pageSize, tt.pageOffset)
			if err != nil {
				t.Fatalf("Users() error = %v", err)
			}

			testingx.AssertDiff(t, got, tt.want, diffOpts...)
		})
	}
}

func TestStore_UpdateUser(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(User{}, "ID", "ETag"),
		cmpopts.EquateApproxTime(time.Minute),
	}

	tests := []struct {
		name string
		seed CreateUser
		in   func(seeded User) UpdateUser
		want func(seeded User) User
	}{
		{
			name: "updates name",
			seed: CreateUser{
				Email: "alice@test.com",
				Name:  "Alice Smith",
			},
			in: func(seeded User) UpdateUser {
				return UpdateUser{
					ExternalID: seeded.ExternalID,
					Name:       "Alice Jones",
					Fields:     UserUpdateFields{Name: true},
				}
			},
			want: func(seeded User) User {
				return User{
					ExternalID: seeded.ExternalID,
					Email:      seeded.Email,
					Name:       "Alice Jones",
					CreatedAt:  seeded.CreatedAt,
					UpdatedAt:  &seeded.CreatedAt,
				}
			},
		},
		{
			name: "name not in fields leaves name unchanged",
			seed: CreateUser{
				Email: "bob@test.com",
				Name:  "Bob Smith",
			},
			in: func(seeded User) UpdateUser {
				return UpdateUser{
					ExternalID: seeded.ExternalID,
					Name:       "ignored",
					Fields:     UserUpdateFields{Name: false},
				}
			},
			want: func(seeded User) User {
				return User{
					ExternalID: seeded.ExternalID,
					Email:      seeded.Email,
					Name:       "Bob Smith",
					CreatedAt:  seeded.CreatedAt,
					UpdatedAt:  &seeded.CreatedAt,
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seeded := seedUser(t, store, tt.seed.Email, tt.seed.Name)

			got, err := store.UpdateUser(ctx, tt.in(seeded))
			if err != nil {
				t.Fatalf("UpdateUser() error = %v", err)
			}

			testingx.AssertDiff(t, got, tt.want(seeded), diffOpts...)

			if got.ETag == seeded.ETag {
				t.Error("UpdateUser() ETag unchanged, want new ETag")
			}
			if got.UpdatedAt == nil {
				t.Error("UpdateUser() UpdatedAt = nil, want non-nil")
			}
		})
	}
}

func TestStore_UpdateUser_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		id := uuid.New()
		_, err := store.UpdateUser(ctx, UpdateUser{
			ExternalID: id,
			Name:       "Alice Jones",
			Fields:     UserUpdateFields{Name: true},
		})
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UpdateUser(%v) error = %v, want sql.ErrNoRows", id, err)
		}
	})
}

func TestStore_GetOrCreateUserByEmail(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := NewStore(pool)

	t.Run("creates user when not found", func(t *testing.T) {
		got, err := store.GetOrCreateUserByEmail(ctx, "new@test.com")
		if err != nil {
			t.Fatalf("GetOrCreateUserByEmail() error = %v", err)
		}

		diffOpts := cmp.Options{
			cmpopts.IgnoreFields(User{}, "ID", "ExternalID", "CreatedAt", "UpdatedAt", "ETag"), // Ignore generated fields
		}
		want := User{Email: "new@test.com"}
		testingx.AssertDiff(t, got, want, diffOpts)
	})

	t.Run("returns existing user without modification", func(t *testing.T) {
		seeded := seedUser(t, store, "existing@test.com", "Existing User")

		got, err := store.GetOrCreateUserByEmail(ctx, seeded.Email)
		if err != nil {
			t.Fatalf("GetOrCreateUserByEmail() error = %v", err)
		}

		testingx.AssertDiff(t, got, seeded)
	})
}

func seedUser(t *testing.T, s *Store, email, name string) User {
	t.Helper()

	seeded, err := s.CreateUser(t.Context(), CreateUser{
		Email: email,
		Name:  name,
	})
	if err != nil {
		t.Fatalf("seed user error: %v", err)
	}

	return seeded
}
