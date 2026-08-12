# api/grpc

gRPC transport layer. Handlers receive protobuf requests, delegate to core interfaces, and return protobuf responses.

## Layout

- `server.go` — server construction and interceptor wiring.
- `<service>.go` (e.g. `user.go`) — one file per gRPC service. Defines the handler struct, the core interface(s) it depends on, and all RPC method implementations for that service.
- `internal/conv/` — all conversions between `pb` and `mdl` types.
- `internal/validate/` — request validation for every RPC.
- `internal/pb/` — generated protobuf code, do not edit by hand.
- `gateway/` — HTTP/JSON reverse proxy (grpc-gateway) and OpenAPI spec endpoint.

## Core interfaces

Each file defines the local interface(s) (e.g. `UserCore`) that its handler depends on. This keeps the gRPC package decoupled from concrete core implementations and makes the handler testable with a mock.

## Conversions

All type conversions between `pb` and `mdl` belong in `internal/conv/<service>.go`. Define one function per direction — never construct a foreign type inline in a handler:

```
pb.User      →  mdl.CreateUser   (CreateUserFromPb)
mdl.User     →  pb.User          (UserToPb)
```

Without dedicated conv functions, type construction scatters across handlers and there is no single place to update when a type changes.

## Gateway

The `gateway/` package translates HTTP/JSON requests to gRPC calls using grpc-gateway and serves an OpenAPI spec at `/openapi.json`.

The spec (`gateway/openapi/openapi.swagger.json`) is generated from proto annotations — do not edit it by hand. Regenerate with `make generate` after changing `.proto` files.

Every RPC documents each HTTP error status it can return through
`openapiv2_operation.responses`, with `google.rpc.Status` as the response schema. Keep these
responses synchronized with errors returned explicitly by the handler and errors returned
implicitly by the interceptor chain. When either path introduces a new status for an RPC, update
the corresponding `.proto` operation annotation and regenerate the OpenAPI artifacts.

## Testing

### Test harnesses

Use `ServerTest` for unit tests: the server runs with mocked cores, so tests exercise only the gRPC handler layer. Because inputs and outputs are fully controlled, this is the right place for exhaustive negative-case coverage — invalid arguments, permission errors, not-found responses, and other error paths.

Use `ServerIntegrationTest` for integration tests that must cross layer boundaries. It wires real cores against a real Postgres database and is slower, so keep integration tests focused on the golden path at a high level rather than exhaustive edge cases.
Exercise and observe the behavior under test only through generated gRPC clients. Direct store or SQL access is allowed only inside seed helpers that prepare prerequisite state; do not use it to perform the action under test or verify its result.

### File conventions

One test file per service (`auth_test.go`, `user_test.go`, …). When a test exercises multiple services — for example, logging in and then updating a display name — put it in `integration_test.go` instead.

### Auth helpers

`NewServerTest` always sets `testJWTKey`. Use `authCtxForTestUser(t, t.Context())` for calls to protected endpoints and plain `t.Context()` for methods listed in `publicMethods`.

## Authorization scopes

Protected methods normally resolve permissions for the project identified by `x-project-id`, combining project-, organization-, and system-scope assignments. Organization-scoped permissions use the same project metadata to identify the active organization but resolve only organization- and system-scope assignments. A project-scoped role therefore cannot authorize an organization-wide operation. System-scoped permissions resolve only system-scope assignments and do not require project metadata.

## Validation

Every RPC has a corresponding function in `internal/validate`. The handler calls it before reading request fields or delegating to the core. Validators reject nil requests and own all checks on client input, including required fields, UUID syntax, update masks, and pagination tokens. Field violations use protobuf payload paths such as `role` and `user.id`; `request` is reserved for request messages that have no required payload field. Return `codes.InvalidArgument` with `errdetails.BadRequest` field violations so callers get actionable field-level feedback.

Handlers may rely on the validated invariants. In particular, a UUID checked by the corresponding validator can be converted with `uuid.MustParse`; a panic would indicate that the handler and validator have drifted apart.

Validator tests exhaustively cover valid, nil, and invalid requests. Handler tests keep one `validated request` case per RPC to prove that the validation boundary is invoked, while the remaining handler error cases cover core error mappings and transport behavior.

This is UX, not the enforcement boundary — the core enforces validation itself and returns `mdl.ErrValidation` (see `core/README.md`). The duplication is deliberate: handler-level checks can reference proto field names (`user.email`) and messages tailored to the API, which the core can't express without knowing about its callers. Relying on the core alone would also mean maintaining a mapping from field violations back to proto field names for every input type.

A `mdl.ErrValidation` escaping from a core call means the two layers have drifted apart; `errorUnaryInterceptor`/`errorStreamInterceptor` catch this centrally rather than per handler.

## Error responses

The gRPC status code is the primary, broad error category. Error messages are for people and logs;
clients must never parse or compare them. Prefer standard `google.rpc` details: `BadRequest` for
field violations, `RetryInfo` for retry timing, and `ResourceInfo` for resource identity.

Use `errorStatus` to attach `theapp.v1.ErrorDetail` only when clients may act differently on stable
domain conditions sharing a gRPC status. Its `code` is the machine-readable contract; `metadata`
holds occurrence-specific values, and its keys are contractual only when documented. Do not add
codes such as `NOT_FOUND`, `INVALID_ARGUMENT`, or `PERMISSION_DENIED` that repeat the status. Plain
not-found, authentication, authorization, internal, and validation errors need no application code;
represent validation with `BadRequest` field violations.

Codes describe reusable domain conditions, not endpoints or implementation details: use
`ERROR_CODE_ETAG_MISMATCH` for every resource with optimistic concurrency, not
`UPDATE_USER_FAILED` or database-specific codes. Never renumber or reuse values. Clients must
tolerate unknown values and fall back to the gRPC status.

Preserve deliberately ambiguous or security-sensitive core errors such as a shared
`mdl.ErrNotFound`. If clients need finer distinctions, model distinct domain errors in the core;
never derive codes from error messages.

Each RPC's OpenAPI response description must list its application codes and gRPC statuses, including
when grpc-gateway maps multiple gRPC statuses to one HTTP status (for example, `InvalidArgument` and
`FailedPrecondition` to HTTP 400). Regenerate protobuf and OpenAPI files after adding or using a code.

## Idempotency

`idempotencyUnaryInterceptor` (in `unary_interceptors.go`) lets a caller resume a dropped request by sending a `x-idempotency-key` header, which it turns into a DBOS workflow ID. The raw key is never used directly: it's hashed together with the authenticated user, the method, and the request payload before use, so two unrelated requests that happen to reuse the same key can never collide on the same workflow, and a caller can never receive another caller's cached result. This is also why `authUnaryInterceptor` must run before it in the chain — the derivation needs the authenticated user ID when one exists.

Streaming RPCs don't support this yet and silently ignore the header. The interceptor runs before any message is read off the stream, so there's no payload to bind the key to at that point — deriving an ID from just the user and method would reintroduce the collision problem the unary path exists to prevent. Server-streaming could support it by deferring derivation until the first `RecvMsg`, similar to how `loggingStream` wraps that call. Client-streaming and bidi have no single request to hash, so idempotent resumption doesn't map cleanly onto them regardless.
