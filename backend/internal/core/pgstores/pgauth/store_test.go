package pgauth_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestStore_AuthSessionData(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	authStore := pgauth.NewStore(pool)
	orgStore := pgorg.NewStore(pool)
	userStore := pguser.NewStore(pool)

	org := seedOrganization(t, orgStore, "auth-session-org")
	project := seedProject(t, orgStore, org.ID, "ordinary-project")

	member := seedUser(t, userStore, "auth-session-member@test.com")
	seedOrgMembership(t, orgStore, member.ExternalID, org.ID)
	nonmember := seedUser(t, userStore, "auth-session-nonmember@test.com")

	tests := []struct {
		name      string
		userID    uuid.UUID
		projectID *int
		want      pgauth.AuthSessionData
	}{
		{
			name:      "system scope",
			userID:    member.ExternalID,
			projectID: nil,
			want: pgauth.AuthSessionData{
				UserExternalID: member.ExternalID,
				Email:          member.Email,
			},
		},
		{
			name:      "control project member",
			userID:    member.ExternalID,
			projectID: new(org.ControlProjectID),
			want: pgauth.AuthSessionData{
				UserExternalID:   member.ExternalID,
				Email:            member.Email,
				ProjectID:        new(org.ControlProjectID),
				OrgID:            new(org.ID),
				OrgName:          new(org.Name),
				IsControlProject: new(true),
				IsOrgMember:      new(true),
			},
		},
		{
			name:      "ordinary project nonmember",
			userID:    nonmember.ExternalID,
			projectID: new(project.ID),
			want: pgauth.AuthSessionData{
				UserExternalID:   nonmember.ExternalID,
				Email:            nonmember.Email,
				ProjectID:        new(project.ID),
				OrgID:            new(org.ID),
				OrgName:          new(org.Name),
				IsControlProject: new(false),
				IsOrgMember:      new(false),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := authStore.AuthSessionData(ctx, tt.userID, tt.projectID)
			if err != nil {
				t.Fatalf("AuthSessionData() error = %v", err)
			}

			testingx.AssertDiff(t, got, tt.want)
		})
	}
}

func TestStore_AuthSessionData_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	authStore := pgauth.NewStore(pool)
	userStore := pguser.NewStore(pool)

	user := seedUser(t, userStore, "auth-session-error@test.com")
	softDeletedUser := seedUser(t, userStore, "deleted-auth-session@test.com")
	mustSoftDeleteUser(t, userStore, softDeletedUser.ExternalID)

	tests := []struct {
		name      string
		userID    uuid.UUID
		projectID *int
		want      error
	}{
		{
			name:   "unknown user",
			userID: uuid.New(),
			want:   sql.ErrNoRows,
		},
		{
			name:   "deleted user",
			userID: softDeletedUser.ExternalID,
			want:   sql.ErrNoRows,
		},
		{
			name:      "unknown project",
			userID:    user.ExternalID,
			projectID: new(-1),
			want:      sql.ErrNoRows,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := authStore.AuthSessionData(ctx, tt.userID, tt.projectID); !errors.Is(err, tt.want) {
				t.Errorf("AuthSessionData(%v, %v) error = %v, want %v", tt.userID, tt.projectID, err, tt.want)
			}
		})
	}
}

func TestStore_MagicLinkToken(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	u := seedUser(t, userStore, "alice@test.com")

	cm := pgauth.CreateMagicLinkToken{
		UserID:    u.ID,
		TokenHash: "abc123hash",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}

	created, err := authStore.CreateMagicLinkToken(ctx, cm)
	if err != nil {
		t.Fatalf("CreateMagicLinkToken() error = %v", err)
	}

	if created.ID == 0 {
		t.Error("CreateMagicLinkToken() ID = 0, want non-zero")
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreateMagicLinkToken() CreatedAt = zero, want non-zero")
	}

	createdDiffOpts := cmp.Options{
		cmpopts.IgnoreFields(pgauth.MagicLinkToken{}, "ID", "CreatedAt"),
		cmpopts.EquateApproxTime(time.Minute),
	}
	wantCreated := pgauth.MagicLinkToken{
		UserID:         u.ID,
		UserExternalID: u.ExternalID,
		ExpiresAt:      cm.ExpiresAt,
	}
	testingx.AssertDiff(t, created, wantCreated, createdDiffOpts)

	fetched, err := authStore.MagicLinkTokenByHash(ctx, cm.TokenHash)
	if err != nil {
		t.Fatalf("MagicLinkTokenByHash() error = %v", err)
	}

	testingx.AssertDiff(t, fetched, created, cmpopts.EquateApproxTime(time.Minute))

	if err := authStore.ConsumeMagicLinkToken(ctx, created.ID); err != nil {
		t.Fatalf("ConsumeMagicLinkToken() error = %v", err)
	}

	// Token must not be retrievable after consumption.
	if _, err = authStore.MagicLinkTokenByHash(ctx, cm.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("MagicLinkTokenByHash() after consume error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_MagicLinkToken_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		if _, err := authStore.MagicLinkTokenByHash(ctx, "nonexistent"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("MagicLinkTokenByHash(nonexistent) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("expired not returned", func(t *testing.T) {
		u := seedUser(t, userStore, "expired@test.com")
		if _, err := authStore.CreateMagicLinkToken(ctx, pgauth.CreateMagicLinkToken{
			UserID:    u.ID,
			TokenHash: "expiredhash",
			ExpiresAt: time.Now().Add(-1 * time.Minute),
		}); err != nil {
			t.Fatalf("CreateMagicLinkToken() error = %v", err)
		}

		if _, err := authStore.MagicLinkTokenByHash(ctx, "expiredhash"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("MagicLinkTokenByHash(expired) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("double consume", func(t *testing.T) {
		u := seedUser(t, userStore, "double@test.com")
		tok, err := authStore.CreateMagicLinkToken(ctx, pgauth.CreateMagicLinkToken{
			UserID:    u.ID,
			TokenHash: "doublehash",
			ExpiresAt: time.Now().Add(15 * time.Minute),
		})
		if err != nil {
			t.Fatalf("CreateMagicLinkToken() error = %v", err)
		}

		if err := authStore.ConsumeMagicLinkToken(ctx, tok.ID); err != nil {
			t.Fatalf("first ConsumeMagicLinkToken() error = %v", err)
		}

		if err = authStore.ConsumeMagicLinkToken(ctx, tok.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("second ConsumeMagicLinkToken() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		if _, err := authStore.CreateMagicLinkToken(ctx, pgauth.CreateMagicLinkToken{
			UserID:    -1,
			TokenHash: "unknownuserhash",
			ExpiresAt: time.Now().Add(15 * time.Minute),
		}); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("CreateMagicLinkToken(unknown user) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("consume nonexistent", func(t *testing.T) {
		if err := authStore.ConsumeMagicLinkToken(ctx, 0); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ConsumeMagicLinkToken(nonexistent) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("consume expired", func(t *testing.T) {
		u := seedUser(t, userStore, "consume-expired-mlt@test.com")
		tok, err := authStore.CreateMagicLinkToken(ctx, pgauth.CreateMagicLinkToken{
			UserID:    u.ID,
			TokenHash: "consume-expired-mlthash",
			ExpiresAt: time.Now().Add(-1 * time.Minute),
		})
		if err != nil {
			t.Fatalf("CreateMagicLinkToken() error = %v", err)
		}

		if err := authStore.ConsumeMagicLinkToken(ctx, tok.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ConsumeMagicLinkToken(expired) error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_LatestMagicLinkTokenCreatedAt(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	u := seedUser(t, userStore, "latest-two@test.com")

	if _, err := authStore.CreateMagicLinkToken(ctx, pgauth.CreateMagicLinkToken{
		UserID:    u.ID,
		TokenHash: "latesttok-first",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateMagicLinkToken(first) error = %v", err)
	}

	second, err := authStore.CreateMagicLinkToken(ctx, pgauth.CreateMagicLinkToken{
		UserID:    u.ID,
		TokenHash: "latesttok-second",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateMagicLinkToken(second) error = %v", err)
	}

	got, err := authStore.LatestMagicLinkTokenCreatedAt(ctx, u.ID)
	if err != nil {
		t.Fatalf("LatestMagicLinkTokenCreatedAt() error = %v", err)
	}

	if !got.Equal(second.CreatedAt) && got.Before(second.CreatedAt) {
		t.Errorf("LatestMagicLinkTokenCreatedAt() = %v, want >= %v", got, second.CreatedAt)
	}
}

func TestStore_LatestMagicLinkTokenCreatedAt_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	t.Run("no tokens", func(t *testing.T) {
		u := seedUser(t, userStore, "latest-none@test.com")
		if _, err := authStore.LatestMagicLinkTokenCreatedAt(ctx, u.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("LatestMagicLinkTokenCreatedAt() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_InvalidateUserMagicLinkTokens(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	t.Run("invalidates all active tokens", func(t *testing.T) {
		u := seedUser(t, userStore, "invalidate-active@test.com")

		hashes := []string{"invhash-a", "invhash-b"}

		for i, hash := range hashes {
			if _, err := authStore.CreateMagicLinkToken(ctx, pgauth.CreateMagicLinkToken{
				UserID:    u.ID,
				TokenHash: hash,
				ExpiresAt: time.Now().Add(time.Duration(i+1) * 15 * time.Minute),
			}); err != nil {
				t.Fatalf("CreateMagicLinkToken(%q) error = %v", hash, err)
			}
		}

		if err := authStore.InvalidateUserMagicLinkTokens(ctx, u.ID); err != nil {
			t.Fatalf("InvalidateUserMagicLinkTokens() error = %v", err)
		}

		for _, hash := range hashes {
			if _, err := authStore.MagicLinkTokenByHash(ctx, hash); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("MagicLinkTokenByHash(%q) after invalidate error = %v, want sql.ErrNoRows", hash, err)
			}
		}
	})

	t.Run("expired tokens are not affected", func(t *testing.T) {
		u := seedUser(t, userStore, "invalidate-expired@test.com")

		if _, err := authStore.CreateMagicLinkToken(ctx, pgauth.CreateMagicLinkToken{
			UserID:    u.ID,
			TokenHash: "expiredinv-hash",
			ExpiresAt: time.Now().Add(-1 * time.Minute),
		}); err != nil {
			t.Fatalf("CreateMagicLinkToken(expired) error = %v", err)
		}

		// No error expected even though there are no active tokens to invalidate.
		if err := authStore.InvalidateUserMagicLinkTokens(ctx, u.ID); err != nil {
			t.Fatalf("InvalidateUserMagicLinkTokens() error = %v", err)
		}
	})

	t.Run("no-op for user with no tokens", func(t *testing.T) {
		u := seedUser(t, userStore, "invalidate-none@test.com")
		if err := authStore.InvalidateUserMagicLinkTokens(ctx, u.ID); err != nil {
			t.Fatalf("InvalidateUserMagicLinkTokens() empty error = %v", err)
		}
	})
}

func TestStore_RefreshToken(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	u := seedUser(t, userStore, "bob@test.com")

	cr := pgauth.CreateRefreshToken{
		UserID:    u.ID,
		TokenHash: "refreshhash",
		ExpiresAt: time.Now().Add(720 * time.Hour),
	}

	created, err := authStore.CreateRefreshToken(ctx, cr)
	if err != nil {
		t.Fatalf("CreateRefreshToken() error = %v", err)
	}

	if created.ID == 0 {
		t.Error("CreateRefreshToken() ID = 0, want non-zero")
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreateRefreshToken() CreatedAt = zero, want non-zero")
	}

	createdDiffOpts := cmp.Options{
		cmpopts.IgnoreFields(pgauth.RefreshToken{}, "ID", "CreatedAt"),
		cmpopts.EquateApproxTime(time.Minute),
	}
	wantCreated := pgauth.RefreshToken{
		UserID:         u.ID,
		UserExternalID: u.ExternalID,
		ExpiresAt:      cr.ExpiresAt,
	}
	testingx.AssertDiff(t, created, wantCreated, createdDiffOpts)

	consumed, err := authStore.ConsumeRefreshToken(ctx, cr.TokenHash)
	if err != nil {
		t.Fatalf("ConsumeRefreshToken() error = %v", err)
	}

	testingx.AssertDiff(t, consumed, created, cmpopts.EquateApproxTime(time.Minute))

	// Token must not be consumable after revocation.
	if _, err = authStore.ConsumeRefreshToken(ctx, cr.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("ConsumeRefreshToken() after revoke error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_RefreshToken_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	authStore := pgauth.NewStore(pool)

	t.Run("unknown user", func(t *testing.T) {
		if _, err := authStore.CreateRefreshToken(ctx, pgauth.CreateRefreshToken{
			UserID:    -1,
			TokenHash: "unknownuserrefresh",
			ExpiresAt: time.Now().Add(720 * time.Hour),
		}); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("CreateRefreshToken(unknown user) error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_ConsumeRefreshToken(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	u := seedUser(t, userStore, "consume@test.com")

	cr := pgauth.CreateRefreshToken{
		UserID:    u.ID,
		TokenHash: "consumehash",
		ExpiresAt: time.Now().Add(720 * time.Hour),
	}

	created, err := authStore.CreateRefreshToken(ctx, cr)
	if err != nil {
		t.Fatalf("CreateRefreshToken() error = %v", err)
	}

	consumed, err := authStore.ConsumeRefreshToken(ctx, cr.TokenHash)
	if err != nil {
		t.Fatalf("ConsumeRefreshToken() error = %v", err)
	}

	testingx.AssertDiff(t, consumed, created, cmpopts.EquateApproxTime(time.Minute))

	// Token must not be consumable after consumption.
	if _, err := authStore.ConsumeRefreshToken(ctx, cr.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("ConsumeRefreshToken() after consume error = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_ConsumeRefreshToken_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	t.Run("not found", func(t *testing.T) {
		if _, err := authStore.ConsumeRefreshToken(ctx, "nonexistent"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ConsumeRefreshToken(nonexistent) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("expired not consumed", func(t *testing.T) {
		u := seedUser(t, userStore, "consume-exp@test.com")
		if _, err := authStore.CreateRefreshToken(ctx, pgauth.CreateRefreshToken{
			UserID:    u.ID,
			TokenHash: "consume-expiredhash",
			ExpiresAt: time.Now().Add(-1 * time.Minute),
		}); err != nil {
			t.Fatalf("CreateRefreshToken() error = %v", err)
		}

		if _, err := authStore.ConsumeRefreshToken(ctx, "consume-expiredhash"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ConsumeRefreshToken(expired) error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("double consume", func(t *testing.T) {
		u := seedUser(t, userStore, "consume-double@test.com")
		if _, err := authStore.CreateRefreshToken(ctx, pgauth.CreateRefreshToken{
			UserID:    u.ID,
			TokenHash: "consume-doublehash",
			ExpiresAt: time.Now().Add(720 * time.Hour),
		}); err != nil {
			t.Fatalf("CreateRefreshToken() error = %v", err)
		}

		if _, err := authStore.ConsumeRefreshToken(ctx, "consume-doublehash"); err != nil {
			t.Fatalf("first ConsumeRefreshToken() error = %v", err)
		}

		if _, err := authStore.ConsumeRefreshToken(ctx, "consume-doublehash"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("second ConsumeRefreshToken() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestStore_LockUser(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	u := seedUser(t, userStore, "lockuser@test.com")

	if err := pgdb.NewTransactor(pool).RunTx(ctx, func(ctx context.Context) error {
		return authStore.LockUser(ctx, u.ID)
	}); err != nil {
		t.Fatalf("LockUser() error = %v", err)
	}
}

func TestStore_RevokeAllUserRefreshTokens(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	userStore := pguser.NewStore(pool)
	authStore := pgauth.NewStore(pool)

	t.Run("revokes all active tokens", func(t *testing.T) {
		u := seedUser(t, userStore, "revokeall@test.com")

		hashes := []string{"revokeall-a", "revokeall-b"}

		for _, hash := range hashes {
			if _, err := authStore.CreateRefreshToken(ctx, pgauth.CreateRefreshToken{
				UserID:    u.ID,
				TokenHash: hash,
				ExpiresAt: time.Now().Add(720 * time.Hour),
			}); err != nil {
				t.Fatalf("CreateRefreshToken(%q) error = %v", hash, err)
			}
		}

		if err := authStore.RevokeAllUserRefreshTokens(ctx, u.ExternalID); err != nil {
			t.Fatalf("RevokeAllUserRefreshTokens() error = %v", err)
		}

		for _, hash := range hashes {
			if _, err := authStore.ConsumeRefreshToken(ctx, hash); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("ConsumeRefreshToken(%q) after revoke-all error = %v, want sql.ErrNoRows", hash, err)
			}
		}
	})

	t.Run("no-op for user with no tokens", func(t *testing.T) {
		u := seedUser(t, userStore, "revokeall-none@test.com")
		if err := authStore.RevokeAllUserRefreshTokens(ctx, u.ExternalID); err != nil {
			t.Fatalf("RevokeAllUserRefreshTokens(no tokens) error = %v", err)
		}
	})
}

func seedUser(t *testing.T, s *pguser.Store, email string) pguser.User {
	t.Helper()
	u, err := s.CreateUser(t.Context(), pguser.CreateUser{
		Email: email,
		Name:  "Test User",
	})
	if err != nil {
		t.Fatalf("seed user %q: %v", email, err)
	}
	return u
}

func mustSoftDeleteUser(t *testing.T, userStore *pguser.Store, userID uuid.UUID) {
	t.Helper()

	if err := userStore.SoftDeleteUser(t.Context(), userID); err != nil {
		t.Fatalf("soft delete user %s: %v", userID, err)
	}
}

func seedOrganization(t *testing.T, s *pgorg.Store, name string) pgorg.Organization {
	t.Helper()

	org, err := s.CreateOrganization(t.Context(), pgorg.CreateOrganization{
		Name:               name,
		ControlProjectName: "control",
	})
	if err != nil {
		t.Fatalf("seed organization %q: %v", name, err)
	}

	return org
}

func seedProject(t *testing.T, s *pgorg.Store, orgID int, name string) pgorg.Project {
	t.Helper()

	project, err := s.CreateProject(t.Context(), pgorg.CreateProject{OrgID: orgID, Name: name})
	if err != nil {
		t.Fatalf("seed project %q: %v", name, err)
	}

	return project
}

func seedOrgMembership(t *testing.T, s *pgorg.Store, userID uuid.UUID, orgID int) {
	t.Helper()

	if err := s.AddOrganizationMember(t.Context(), userID, orgID); err != nil {
		t.Fatalf("seed organization membership: %v", err)
	}
}
