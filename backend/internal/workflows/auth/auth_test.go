package auth

import (
	"context"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	coreauth "github.com/zorcal/theapp/backend/internal/core/auth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestWorkflowCore_RequestMagicLink(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)

	dbosCtx, err := dbos.NewDBOSContext(ctx, dbos.Config{
		AppName:     "theapp-test",
		DatabaseURL: pool.Config().ConnString(),
		Logger:      testingx.NewLogger(t),
	})
	if err != nil {
		t.Fatalf("dbos.NewDBOSContext: %v", err)
	}

	emailSender := &testingx.CaptureEmailSender{}

	cfg := coreauth.Config{
		JWTKey:             []byte("test-secret"),
		JWTIssuer:          "theapp-test",
		JWTAudience:        "theapp-api-test",
		MagicLinkFromEmail: "noreply@test.com",
		MagicLinkBaseURL:   "http://localhost:3000/auth/verify",
		MagicLinkTTL:       15 * time.Minute,
		MagicLinkRateLimit: 0,
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    720 * time.Hour,
	}

	authCore := coreauth.NewCore(
		pgauth.NewStore(pool),
		pguser.NewStore(pool),
		pgdb.NewTransactor(pool),
		cfg,
	)

	wc := NewWorkflowCore(authCore, emailSender, cfg, dbosCtx)
	RegisterWorkflows(dbosCtx, wc)

	if err := dbos.Launch(dbosCtx); err != nil {
		t.Fatalf("dbos.Launch: %v", err)
	}
	t.Cleanup(func() {
		dbos.Shutdown(dbosCtx, 5*time.Second)
	})

	if err := wc.RequestMagicLink(ctx, "alice@test.com"); err != nil {
		t.Fatalf("RequestMagicLink() error = %v", err)
	}

	tok := emailSender.MagicLinkToken(t)
	if tok == "" {
		t.Fatal("RequestMagicLink() did not send an email with a token")
	}

	// Token is consumable via the auth core.
	pair, err := authCore.VerifyMagicLink(ctx, tok)
	if err != nil {
		t.Fatalf("VerifyMagicLink() error = %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("VerifyMagicLink() AccessToken is empty")
	}
}
