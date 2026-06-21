# DBOS adoption plan

DBOS Transact (https://github.com/dbos-inc/dbos-transact-golang) is a durable-execution library that checkpoints workflow state in Postgres. When the process crashes mid-workflow, it resumes automatically from the last completed step. No external orchestration service is required — it uses the existing Postgres pool.

## Why we're adopting it

`RequestMagicLink` commits the token to the database and then sends an email as two separate operations. If the process crashes between them, the user holds an unreachable token and is rate-limited for one minute. This is documented as a known tradeoff in `core/auth/README.md`. DBOS makes step 2 automatically retry after a restart, eliminating the tradeoff.

More broadly, any future multi-step operation that must complete reliably across a failure boundary (onboarding, billing, third-party API provisioning) can be expressed as a workflow without adding infrastructure.

## Architecture: thin workflow layer, split gRPC interfaces

Core remains pure business logic with no DBOS dependency, fully testable with mocks. A new `workflows/` layer sits between gRPC and core:

```
gRPC → workflows/ → core/ → pgstores/   (durable operations)
gRPC → core/                             (direct operations)
```

The gRPC `authService` takes two explicit dependencies — one for direct operations, one for durable ones. This makes it clear at the call site which operations go through DBOS and which do not. `workflows/auth` only implements the methods that are actually workflows; it does not wrap or delegate the rest.

## What becomes a workflow

**`RequestMagicLink`** — two steps: DB transaction (store token, invalidate old ones, rate check) then email send. If the process crashes between steps, DBOS resumes and retries the email automatically.

Everything else — `VerifyMagicLink`, `RefreshAccessToken`, `RevokeRefreshToken`, `RevokeAllUserRefreshTokens` — stays as direct core calls. These are single atomic DB operations; DBOS adds overhead and replay constraints without benefit.

## Code structure

```
backend/internal/
  core/auth/              # unchanged — pure business logic, no DBOS dependency
  workflows/
    auth/
      auth.go             # WorkflowCore struct, implements WorkflowAuthCore
      auth_test.go        # integration tests against a real DB
  api/grpc/auth.go        # split into AuthCore and WorkflowAuthCore interfaces
  cmd/server/main.go      # wiring: pass both authCore and workflowAuthCore
```

## Split interfaces at the gRPC layer

`grpc/auth.go` currently has a single `AuthCore` interface. Split it into two:

```go
// AuthCore handles direct, non-durable auth operations.
// Implemented by *core/auth.Core.
type AuthCore interface {
    // VerifyMagicLink validates a magic-link token and returns a token pair.
    // Returns [mdl.ErrTokenInvalid] if the token is expired, consumed, or not found.
    VerifyMagicLink(ctx context.Context, rawToken string) (mdl.AuthTokenPair, error)
    // RefreshAccessToken rotates the refresh token and returns a new token pair.
    // Returns [mdl.ErrTokenInvalid] if the token is expired, revoked, or not found.
    RefreshAccessToken(ctx context.Context, rawToken string) (mdl.AuthTokenPair, error)
    // RevokeRefreshToken invalidates a refresh token.
    // Returns [mdl.ErrTokenInvalid] if the token is not found or already revoked.
    RevokeRefreshToken(ctx context.Context, rawToken string) error
    // RevokeAllUserRefreshTokens revokes all active refresh tokens for the user.
    RevokeAllUserRefreshTokens(ctx context.Context, userExternalID uuid.UUID) error
}

// WorkflowAuthCore handles durable auth operations backed by DBOS.
// Implemented by *workflows/auth.WorkflowCore.
type WorkflowAuthCore interface {
    RequestMagicLink(ctx context.Context, email string) error
}
```

`authService` takes both:

```go
type authService struct {
    pb.UnimplementedAuthServiceServer
    authCore         AuthCore
    workflowAuthCore WorkflowAuthCore
}
```

`RequestMagicLink` calls `s.workflowAuthCore.RequestMagicLink`; all other handlers call `s.authCore`.

The moq directive on `AuthCore` stays; add a second one for `WorkflowAuthCore`.

## WorkflowCore

`workflows/auth` only implements `WorkflowAuthCore` — the one method that is actually a workflow. It does not wrap or re-expose core.

```go
// backend/internal/workflows/auth/auth.go

package auth

import (
    "context"

    "github.com/dbos-inc/dbos-transact-golang/dbos"

    coreauth "github.com/zorcal/theapp/backend/internal/core/auth"
    "github.com/zorcal/theapp/backend/internal/email"
)

type WorkflowCore struct {
    core        *coreauth.Core
    emailSender email.Sender
    cfg         coreauth.Config
}

func NewWorkflowCore(core *coreauth.Core, emailSender email.Sender, cfg coreauth.Config) *WorkflowCore {
    return &WorkflowCore{
        core:        core,
        emailSender: emailSender,
        cfg:         cfg,
    }
}

func (w *WorkflowCore) RequestMagicLink(ctx context.Context, emailAddr string) error {
    handle, err := dbos.RunWorkflow(ctx, w.requestMagicLinkWorkflow, emailAddr)
    if err != nil {
        return err
    }
    _, err = handle.GetResult()
    return err
}

func (w *WorkflowCore) requestMagicLinkWorkflow(ctx dbos.DBOSContext, emailAddr string) (struct{}, error) {
    // Step 1: transactional DB work — rate check, invalidation, token storage.
    // Returns the raw token, or "" if rate-limited. Checkpointed on completion.
    rawToken, err := dbos.RunAsStep(ctx, func(ctx context.Context) (string, error) {
        return w.core.MagicLinkToken(ctx, emailAddr)
    }, dbos.WithStepName("store-token"))
    if err != nil || rawToken == "" {
        return struct{}{}, err
    }

    // Step 2: send email. Retried automatically if the process crashes here.
    _, err = dbos.RunAsStep(ctx, func(ctx context.Context) (struct{}, error) {
        return struct{}{}, w.sendMagicLinkEmail(ctx, emailAddr, rawToken)
    }, dbos.WithStepName("send-email"))
    return struct{}{}, err
}
```

## Changes to core/auth

`Core.RequestMagicLink` currently does DB work and email send together. Split it:

- Extract the transactional part into `Core.MagicLinkToken(ctx, emailAddr) (rawToken string, err error)`. This runs the advisory lock, rate check, token invalidation, and token creation inside a transaction. Returns the raw token, or `""` if rate-limited.
- Keep `Core.RequestMagicLink` temporarily so existing unit tests continue to pass. Remove it (along with `emailSender` from `Core`) once the workflow layer is wired and verified.

Email template rendering and the `sendMagicLinkEmail` helper move from `core/auth` to `workflows/auth`, since sending email is no longer core's responsibility.

## DBOS initialization in main.go

DBOS can share the existing `pgxpool.Pool` via `SystemDBPool` — no second connection pool is needed.

```go
// After pgPool is created and verified:

dbosCtx, err := dbos.NewDBOSContext(&dbos.Config{
    AppName:      appName,
    SystemDBPool: pgPool,
    Logger:       log,
})
if err != nil {
    return fmt.Errorf("init dbos: %w", err)
}

authWorkflowCore := workflowsauth.NewWorkflowCore(authCore, emailSender, authCoreCfg)
dbos.RegisterWorkflow(dbosCtx, authWorkflowCore.requestMagicLinkWorkflow)

if err := dbos.Launch(dbosCtx); err != nil {
    return fmt.Errorf("launch dbos: %w", err)
}
defer dbos.Destroy(dbosCtx)

srv := grpc.NewServer(grpc.ServerConfig{
    ...
    AuthCore:         authCore,         // *auth.Core — direct operations
    WorkflowAuthCore: authWorkflowCore, // *workflows/auth.WorkflowCore — durable operations
    ...
})
```

DBOS runs its own schema migrations on `Launch`. The schema is isolated in its own namespace and does not interfere with the application schema.

## Testing

The workflow layer requires integration tests against a real database — DBOS checkpoints to Postgres and cannot be mocked at the library level. Use `pgtest.New(t, ctx)` as the other integration tests do, and initialize a DBOS context pointing at that pool.

Core unit tests are unaffected — they continue to use mocked storers with no DBOS dependency.

The gRPC layer tests need a second mock for `WorkflowAuthCore`. Add a moq directive on the new interface and regenerate.

## Migration steps

1. Add `github.com/dbos-inc/dbos-transact-golang` to `go.mod`.
2. Extract `Core.MagicLinkToken(ctx, emailAddr) (string, error)` from `Core.RequestMagicLink` and keep the existing method temporarily.
3. Split `grpc.AuthCore` into `AuthCore` (without `RequestMagicLink`) and `WorkflowAuthCore` (just `RequestMagicLink`). Update `authService` to carry both fields. Regenerate moqs.
4. Create `backend/internal/workflows/auth/auth.go` with `WorkflowCore` as above.
5. Move email template rendering and `sendMagicLinkEmail` from `core/auth` to `workflows/auth`.
6. Register the workflow and initialize DBOS in `cmd/server/main.go` as above.
7. Verify locally: run the server, request a magic link, confirm the email arrives and the workflow appears in the `dbos` schema.
8. Remove `Core.RequestMagicLink` and `emailSender` from `core/auth.Core`.
9. Update `core/auth/README.md` — remove the "email sent outside transaction" tradeoff entry.
