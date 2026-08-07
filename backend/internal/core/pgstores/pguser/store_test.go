package pguser_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_UserByEmail(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := pguser.NewStore(pool)

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
	store := pguser.NewStore(pool)

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
	store := pguser.NewStore(pool)

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
	store := pguser.NewStore(pool)

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
	store := pguser.NewStore(pool)

	got, err := store.CreateUser(ctx, pguser.CreateUser{
		Email: "alice@test.com",
		Name:  "Alice Smith",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	want := pguser.User{
		Email:     "alice@test.com",
		Name:      "Alice Smith",
		CreatedAt: time.Now(),
		UpdatedAt: nil,
	}

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(pguser.User{}, "ID", "ExternalID", "ETag"), // Ignore generated fields
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

func TestStore_MarkEmailVerified(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := pguser.NewStore(pool)

	t.Run("sets email_verified_at on first call", func(t *testing.T) {
		seeded := seedUser(t, store, "alice@test.com", "Alice Smith")

		if seeded.EmailVerifiedAt != nil {
			t.Fatalf("EmailVerifiedAt = %v, want nil before verification", seeded.EmailVerifiedAt)
		}

		if err := store.MarkEmailVerified(ctx, seeded.ExternalID); err != nil {
			t.Fatalf("MarkEmailVerified() error = %v", err)
		}

		got, err := store.UserByExternalID(ctx, seeded.ExternalID)
		if err != nil {
			t.Fatalf("UserByExternalID() error = %v", err)
		}
		if got.EmailVerifiedAt == nil {
			t.Error("EmailVerifiedAt = nil, want non-nil after verification")
		}
	})

	t.Run("does not overwrite timestamp on subsequent calls", func(t *testing.T) {
		seeded := seedUser(t, store, "bob@test.com", "Bob Jones")

		if err := store.MarkEmailVerified(ctx, seeded.ExternalID); err != nil {
			t.Fatalf("MarkEmailVerified() first call error = %v", err)
		}
		first, err := store.UserByExternalID(ctx, seeded.ExternalID)
		if err != nil {
			t.Fatalf("UserByExternalID() error = %v", err)
		}

		if err := store.MarkEmailVerified(ctx, seeded.ExternalID); err != nil {
			t.Fatalf("MarkEmailVerified() second call error = %v", err)
		}
		second, err := store.UserByExternalID(ctx, seeded.ExternalID)
		if err != nil {
			t.Fatalf("UserByExternalID() error = %v", err)
		}

		if !first.EmailVerifiedAt.Equal(*second.EmailVerifiedAt) {
			t.Errorf("EmailVerifiedAt changed on second call: first = %v, second = %v", first.EmailVerifiedAt, second.EmailVerifiedAt)
		}
	})
}

func TestStore_CreateUser_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := pguser.NewStore(pool)

	t.Run("duplicate email returns ErrAlreadyExists", func(t *testing.T) {
		seedUser(t, store, "alice@test.com", "Alice Smith")

		if _, err := store.CreateUser(ctx, pguser.CreateUser{
			Email: "alice@test.com",
			Name:  "Alice Duplicate",
		}); !errors.Is(err, pgdb.ErrAlreadyExists) {
			t.Errorf("CreateUser() error = %v, want pgdb.ErrAlreadyExists", err)
		}
	})
}

func TestStore_Users(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := pguser.NewStore(pool)

	charlie := seedUser(t, store, "charlie@test.com", "Charlie Brown")
	alice := seedUser(t, store, "alice@test.com", "Alice Smith")
	bob := seedUser(t, store, "bob@test.com", "Bob Jones")

	diffOpts := cmp.Options{
		cmpopts.EquateApproxTime(time.Minute),
	}

	tests := []struct {
		name       string
		filter     pguser.Filter
		orderBys   []order.By[pguser.OrderByField]
		pageSize   int
		pageOffset int
		want       []pguser.User
		wantCount  int
	}{
		{
			name:       "no order defaults to insert order",
			pageSize:   10,
			pageOffset: 0,
			want:       []pguser.User{charlie, alice, bob},
			wantCount:  3,
		},
		{
			name:       "order by email asc",
			orderBys:   []order.By[pguser.OrderByField]{order.NewBy(pguser.OrderByFieldEmail, order.DirectionAsc)},
			pageSize:   10,
			pageOffset: 0,
			want:       []pguser.User{alice, bob, charlie},
			wantCount:  3,
		},
		{
			name:       "order by email desc",
			orderBys:   []order.By[pguser.OrderByField]{order.NewBy(pguser.OrderByFieldEmail, order.DirectionDesc)},
			pageSize:   10,
			pageOffset: 0,
			want:       []pguser.User{charlie, bob, alice},
			wantCount:  3,
		},
		{
			name:       "first page",
			orderBys:   []order.By[pguser.OrderByField]{order.NewBy(pguser.OrderByFieldEmail, order.DirectionAsc)},
			pageSize:   2,
			pageOffset: 0,
			want:       []pguser.User{alice, bob},
			wantCount:  3,
		},
		{
			name:       "second page",
			orderBys:   []order.By[pguser.OrderByField]{order.NewBy(pguser.OrderByFieldEmail, order.DirectionAsc)},
			pageSize:   2,
			pageOffset: 2,
			want:       []pguser.User{charlie},
			wantCount:  3,
		},
		{
			name:       "offset past end returns empty",
			pageSize:   10,
			pageOffset: 10,
			want:       []pguser.User{},
			wantCount:  3,
		},
		{
			name:       "filter by email prefix",
			filter:     pguser.Filter{Email: " alice "},
			pageSize:   10,
			pageOffset: 0,
			want:       []pguser.User{alice},
			wantCount:  1,
		},
		{
			name:       "filter by name prefix",
			filter:     pguser.Filter{Name: " Bob "},
			pageSize:   10,
			pageOffset: 0,
			want:       []pguser.User{bob},
			wantCount:  1,
		},
		{
			name: "filter by email and name prefix",
			filter: pguser.Filter{
				Email: " c ",
				Name:  " Charlie ",
			},
			pageSize:   10,
			pageOffset: 0,
			want:       []pguser.User{charlie},
			wantCount:  1,
		},
		{
			name:       "whitespace-only filter",
			filter:     pguser.Filter{Email: " ", Name: " "},
			pageSize:   10,
			pageOffset: 0,
			want:       []pguser.User{charlie, alice, bob},
			wantCount:  3,
		},
		{
			name:       "filter with no matches returns empty",
			filter:     pguser.Filter{Email: "nobody"},
			pageSize:   10,
			pageOffset: 0,
			want:       []pguser.User{},
			wantCount:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotCount, err := store.Users(ctx, tt.filter, tt.orderBys, tt.pageSize, tt.pageOffset)
			if err != nil {
				t.Fatalf("Users() error = %v", err)
			}

			testingx.AssertDiff(t, got, tt.want, diffOpts...)

			if gotCount != tt.wantCount {
				t.Errorf("Users(%+v, %+v, %d, %d) total count = %d, want %d", tt.filter, tt.orderBys, tt.pageSize, tt.pageOffset, gotCount, tt.wantCount)
			}
		})
	}
}

func TestStore_UpdateUser(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := pguser.NewStore(pool)

	diffOpts := cmp.Options{
		cmpopts.IgnoreFields(pguser.User{}, "ID", "ETag"),
		cmpopts.EquateApproxTime(time.Minute),
	}

	tests := []struct {
		name string
		seed pguser.CreateUser
		in   func(seeded pguser.User) pguser.UpdateUser
		want func(seeded pguser.User) pguser.User
	}{
		{
			name: "updates name",
			seed: pguser.CreateUser{
				Email: "alice@test.com",
				Name:  "Alice Smith",
			},
			in: func(seeded pguser.User) pguser.UpdateUser {
				return pguser.UpdateUser{
					ExternalID: seeded.ExternalID,
					Name:       "Alice Jones",
					Fields:     pguser.UserUpdateFields{Name: true},
				}
			},
			want: func(seeded pguser.User) pguser.User {
				return pguser.User{
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
			seed: pguser.CreateUser{
				Email: "bob@test.com",
				Name:  "Bob Smith",
			},
			in: func(seeded pguser.User) pguser.UpdateUser {
				return pguser.UpdateUser{
					ExternalID: seeded.ExternalID,
					Name:       "ignored",
					Fields:     pguser.UserUpdateFields{Name: false},
				}
			},
			want: func(seeded pguser.User) pguser.User {
				return pguser.User{
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
	store := pguser.NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		id := uuid.New()
		_, err := store.UpdateUser(ctx, pguser.UpdateUser{
			ExternalID: id,
			Name:       "Alice Jones",
			Fields:     pguser.UserUpdateFields{Name: true},
		})
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("UpdateUser(%v) error = %v, want sql.ErrNoRows", id, err)
		}
	})
}

func TestStore_GetOrCreateUserByEmail(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	store := pguser.NewStore(pool)

	t.Run("creates user when not found", func(t *testing.T) {
		got, err := store.GetOrCreateUserByEmail(ctx, "new@test.com")
		if err != nil {
			t.Fatalf("GetOrCreateUserByEmail() error = %v", err)
		}

		diffOpts := cmp.Options{
			cmpopts.IgnoreFields(pguser.User{}, "ID", "ExternalID", "CreatedAt", "UpdatedAt", "ETag"), // Ignore generated fields
		}
		want := pguser.User{Email: "new@test.com"}

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

func seedUser(t *testing.T, s *pguser.Store, email, name string) pguser.User {
	t.Helper()

	seeded, err := s.CreateUser(t.Context(), pguser.CreateUser{
		Email: email,
		Name:  name,
	})
	if err != nil {
		t.Fatalf("seed user error: %v", err)
	}

	return seeded
}
