package pgauth

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
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
)

func TestStore_MagicToken(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := NewStore(pool)

	userID := seedUser(t, userStore, "alice@test.com")

	cm := CreateMagicToken{
		UserExternalID: userID,
		TokenHash:      "abc123hash",
		ExpiresAt:      time.Now().Add(15 * time.Minute),
	}

	created, err := authStore.CreateMagicToken(ctx, cm)
	if err != nil {
		t.Fatalf("CreateMagicToken() error = %v", err)
	}

	if created.ID == 0 {
		t.Error("CreateMagicToken() ID = 0, want non-zero")
	}

	createdDiffOpts := cmp.Options{
		cmpopts.IgnoreFields(MagicToken{}, "ID"),
		cmpopts.EquateApproxTime(time.Minute),
	}
	wantCreated := MagicToken{
		UserExternalID: userID,
		ExpiresAt:      cm.ExpiresAt,
	}
	if diff := cmp.Diff(created, wantCreated, createdDiffOpts); diff != "" {
		t.Errorf("CreateMagicToken() diff (-got +want):\n%s", diff)
	}

	fetched, err := authStore.MagicTokenByHash(ctx, cm.TokenHash)
	if err != nil {
		t.Fatalf("MagicTokenByHash() error = %v", err)
	}

	if diff := cmp.Diff(fetched, created, cmpopts.EquateApproxTime(time.Minute)); diff != "" {
		t.Errorf("MagicTokenByHash() diff (-got +want):\n%s", diff)
	}

	if err := authStore.ConsumeMagicToken(ctx, created.ID); err != nil {
		t.Fatalf("ConsumeMagicToken() error = %v", err)
	}

	// Token must not be retrievable after consumption.
	if _, err = authStore.MagicTokenByHash(ctx, cm.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("MagicTokenByHash() after consume error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_MagicToken_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		_, err := authStore.MagicTokenByHash(ctx, "nonexistent")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("MagicTokenByHash(nonexistent) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("expired not returned", func(t *testing.T) {
		uid := seedUser(t, userStore, "expired@test.com")
		_, err := authStore.CreateMagicToken(ctx, CreateMagicToken{
			UserExternalID: uid,
			TokenHash:      "expiredhash",
			ExpiresAt:      time.Now().Add(-1 * time.Minute),
		})
		if err != nil {
			t.Fatalf("CreateMagicToken() error = %v", err)
		}

		if _, err = authStore.MagicTokenByHash(ctx, "expiredhash"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("MagicTokenByHash(expired) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("double consume", func(t *testing.T) {
		uid := seedUser(t, userStore, "double@test.com")
		tok, err := authStore.CreateMagicToken(ctx, CreateMagicToken{
			UserExternalID: uid,
			TokenHash:      "doublehash",
			ExpiresAt:      time.Now().Add(15 * time.Minute),
		})
		if err != nil {
			t.Fatalf("CreateMagicToken() error = %v", err)
		}

		if err := authStore.ConsumeMagicToken(ctx, tok.ID); err != nil {
			t.Fatalf("first ConsumeMagicToken() error = %v", err)
		}

		if err = authStore.ConsumeMagicToken(ctx, tok.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("second ConsumeMagicToken() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_RefreshToken(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := NewStore(pool)

	userID := seedUser(t, userStore, "bob@test.com")

	cr := CreateRefreshToken{
		UserExternalID: userID,
		TokenHash:      "refreshhash",
		ExpiresAt:      time.Now().Add(720 * time.Hour),
	}

	created, err := authStore.CreateRefreshToken(ctx, cr)
	if err != nil {
		t.Fatalf("CreateRefreshToken() error = %v", err)
	}

	if created.ID == 0 {
		t.Error("CreateRefreshToken() ID = 0, want non-zero")
	}

	createdDiffOptions := cmp.Options{
		cmpopts.IgnoreFields(RefreshToken{}, "ID", "CreatedAt"),
		cmpopts.EquateApproxTime(time.Minute),
	}
	wantCreated := RefreshToken{
		UserExternalID: userID,
		ExpiresAt:      cr.ExpiresAt,
	}
	if diff := cmp.Diff(created, wantCreated, createdDiffOptions); diff != "" {
		t.Errorf("CreateRefreshToken() diff (-got +want):\n%s", diff)
	}

	fetched, err := authStore.RefreshTokenByHash(ctx, cr.TokenHash)
	if err != nil {
		t.Fatalf("RefreshTokenByHash() error = %v", err)
	}

	if diff := cmp.Diff(fetched, created, cmpopts.EquateApproxTime(time.Minute)); diff != "" {
		t.Errorf("RefreshTokenByHash() diff (-got +want):\n%s", diff)
	}

	if err := authStore.RevokeRefreshToken(ctx, created.ID); err != nil {
		t.Fatalf("RevokeRefreshToken() error = %v", err)
	}

	// Token must not be retrievable after revocation.
	if _, err = authStore.RefreshTokenByHash(ctx, cr.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("RefreshTokenByHash() after revoke error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_RefreshToken_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		_, err := authStore.RefreshTokenByHash(ctx, "nonexistent")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("RefreshTokenByHash(nonexistent) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("expired not returned", func(t *testing.T) {
		uid := seedUser(t, userStore, "rexp@test.com")
		_, err := authStore.CreateRefreshToken(ctx, CreateRefreshToken{
			UserExternalID: uid,
			TokenHash:      "expiredrefresh",
			ExpiresAt:      time.Now().Add(-1 * time.Minute),
		})
		if err != nil {
			t.Fatalf("CreateRefreshToken() error = %v", err)
		}

		if _, err = authStore.RefreshTokenByHash(ctx, "expiredrefresh"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("RefreshTokenByHash(expired) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("double revoke", func(t *testing.T) {
		uid := seedUser(t, userStore, "drevoke@test.com")
		tok, err := authStore.CreateRefreshToken(ctx, CreateRefreshToken{
			UserExternalID: uid,
			TokenHash:      "doublerevoke",
			ExpiresAt:      time.Now().Add(720 * time.Hour),
		})
		if err != nil {
			t.Fatalf("CreateRefreshToken() error = %v", err)
		}

		if err := authStore.RevokeRefreshToken(ctx, tok.ID); err != nil {
			t.Fatalf("first RevokeRefreshToken() error = %v", err)
		}

		if err = authStore.RevokeRefreshToken(ctx, tok.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("second RevokeRefreshToken() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func seedUser(t *testing.T, s *pguser.Store, email string) uuid.UUID {
	t.Helper()
	u, err := s.CreateUser(t.Context(), pguser.CreateUser{Email: email, Name: "Test User"})
	if err != nil {
		t.Fatalf("seed user %q: %v", email, err)
	}
	return u.ExternalID
}
