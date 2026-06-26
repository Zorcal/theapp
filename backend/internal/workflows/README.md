# workflows

Durable, resumable operations backed by [DBOS](https://github.com/dbos-inc/dbos-transact-golang). A workflow is for orchestrating a sequence of steps that must survive a process crash mid-operation (e.g. a token is stored but the email confirming it hasn't been sent yet). Workflows call into `core/` packages for business logic and data access — they never talk to databases directly.

## Layout

- `workflows.go` — shared context helpers (`WithWorkflowID`, `WorkflowID`) for propagating a caller-supplied workflow ID down to whichever workflow ends up running.
- `<pkg>/` (e.g. `auth/`) — one package per business area, grouping all the workflows for that area. Defines a `WorkflowCore`, its workflow(s) and step(s), and any other assets the workflows need.
- `dbostest/` — test helpers for constructing and launching a `dbos.DBOSContext` in tests.

## WorkflowCore pattern

Each package exposes a `WorkflowCore` struct wrapping a `dbos.DBOSContext` plus whatever non-durable dependencies its workflows call into. Core dependencies should be interfaced out rather than depending on the concrete core type.

`NewWorkflowCore` only constructs the value. Registering its workflows with DBOS is a separate, explicit step (`RegisterWorkflows`) — it mutates the shared `dbosCtx` and DBOS panics if workflows are registered after `Launch`, so keeping it visible at the call site makes that ordering easy to audit.

## Steps

Never inline an anonymous function at the `RunAsStep` call site — provide constructors for steps instead.

## Idempotency

A caller may attach a workflow ID to the context via `WithWorkflowID`. A workflow method reads it back with `WorkflowID(ctx)` and passes it to `dbos.RunWorkflow` via `dbos.WithWorkflowID`, so retrying with the same ID resumes or replays the original execution instead of running twice. It is the caller's responsibility to derive this ID safely — `workflows` itself only stores and retrieves whatever it's given.

## Testing

Use `dbostest.New(t, ctx, pool)` to build a `dbos.DBOSContext` against a `pgtest` pool, register the test's workflows on it, then call `dbostest.Launch(t, dbosCtx)` — it starts DBOS and registers the shutdown cleanup.

Each package uses two complementary layers of tests:

- **Unit tests** — mock the core interface(s) and cover step logic, error handling, and the resume-on-workflow-ID behavior with a real DBOS context but mocked cores.
- **Integration tests** (named `Test<WorkflowCore>_integration`) — wire the real core against a real Postgres database and DBOS context, covering the happy path end-to-end. One or two per package is enough; their job is to catch wiring mistakes that mocks cannot, not to duplicate unit test coverage.
