package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/auth"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
	"github.com/zorcal/theapp/backend/internal/workflows"
	"github.com/zorcal/theapp/backend/internal/workflows/dbostest"
)

func TestWorkflowCore_integration(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	dbosCtx := dbostest.New(t, ctx, pool)

	emailSender := &testingx.CaptureEmailSender{}

	cfg := auth.Config{
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

	authCore := auth.NewCore(
		pgauth.NewStore(pool),
		pguser.NewStore(pool),
		pgdb.NewTransactor(pool),
		cfg,
	)

	wc := NewWorkflowCore(authCore, emailSender, cfg, dbosCtx)
	RegisterWorkflows(dbosCtx, wc)
	dbostest.Launch(t, dbosCtx)

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

// TestWorkflowCore_RequestMagicLink_resume verifies that DBOS deduplicates on workflow ID: calling
// RequestMagicLink twice with the same ID runs the underlying workflow only once, so the caller does not
// get charged with a second token generation or a second email.
func TestWorkflowCore_RequestMagicLink_resume(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	dbosCtx := dbostest.New(t, ctx, pool)

	var tokenCalls atomic.Int32
	authCore := &MockedAuthCore{
		MagicLinkTokenFunc: func(_ context.Context, _ string) (string, error) {
			tokenCalls.Add(1)
			return "fixed-token", nil
		},
	}
	emailSender := &testingx.CaptureEmailSender{}
	cfg := testWorkflowCoreConfig()

	wc := NewWorkflowCore(authCore, emailSender, cfg, dbosCtx)
	RegisterWorkflows(dbosCtx, wc)
	dbostest.Launch(t, dbosCtx)

	idCtx := workflows.WithWorkflowID(ctx, uuid.NewString())

	if err := wc.RequestMagicLink(idCtx, "alice@test.com"); err != nil {
		t.Fatalf("RequestMagicLink() #1 error = %v, want nil", err)
	}
	if err := wc.RequestMagicLink(idCtx, "alice@test.com"); err != nil {
		t.Fatalf("RequestMagicLink() #2 error = %v, want nil", err)
	}

	if got, want := tokenCalls.Load(), int32(1); got != want {
		t.Errorf("MagicLinkToken call count = %d, want %d", got, want)
	}
	if got, want := emailSender.Count(), 1; got != want {
		t.Errorf("CaptureEmailSender.Count() = %d, want %d", got, want)
	}
}

// TestWorkflowCore_RequestMagicLink_rateLimited verifies that a rate-limited request (MagicLinkToken
// returns mdl.ErrRateLimited) succeeds without sending an email, rather than surfacing the rate limit
// as an error to the caller.
func TestWorkflowCore_RequestMagicLink_rateLimited(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	dbosCtx := dbostest.New(t, ctx, pool)

	authCore := &MockedAuthCore{
		MagicLinkTokenFunc: func(_ context.Context, _ string) (string, error) {
			return "", mdl.ErrRateLimited
		},
	}
	emailSender := &testingx.CaptureEmailSender{}
	cfg := testWorkflowCoreConfig()

	wc := NewWorkflowCore(authCore, emailSender, cfg, dbosCtx)
	RegisterWorkflows(dbosCtx, wc)
	dbostest.Launch(t, dbosCtx)

	if err := wc.RequestMagicLink(ctx, "alice@test.com"); err != nil {
		t.Fatalf("RequestMagicLink() error = %v, want nil", err)
	}

	if got, want := emailSender.Count(), 0; got != want {
		t.Errorf("CaptureEmailSender.Count() = %d, want %d", got, want)
	}
}

func TestWorkflowCore_RequestMagicLink_error(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	dbosCtx := dbostest.New(t, ctx, pool)

	wantErr := errors.New("boom")
	authCore := &MockedAuthCore{
		MagicLinkTokenFunc: func(_ context.Context, _ string) (string, error) {
			return "", wantErr
		},
	}
	emailSender := &testingx.CaptureEmailSender{}
	cfg := testWorkflowCoreConfig()

	wc := NewWorkflowCore(authCore, emailSender, cfg, dbosCtx)
	RegisterWorkflows(dbosCtx, wc)
	dbostest.Launch(t, dbosCtx)

	if err := wc.RequestMagicLink(ctx, "alice@test.com"); err == nil {
		t.Fatal("RequestMagicLink() error = nil, want error")
	}

	if got, want := emailSender.Count(), 0; got != want {
		t.Errorf("CaptureEmailSender.Count() = %d, want %d", got, want)
	}
}

func testWorkflowCoreConfig() auth.Config {
	return auth.Config{
		MagicLinkFromEmail: "noreply@test.com",
		MagicLinkBaseURL:   "http://localhost:3000/auth/verify",
		MagicLinkTTL:       15 * time.Minute,
	}
}
