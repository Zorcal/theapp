# api/grpc

gRPC transport layer. Handlers receive protobuf requests, delegate to core interfaces, and return protobuf responses.

## Layout

- `server.go` — server construction and interceptor wiring.
- `<service>.go` (e.g. `user.go`) — one file per gRPC service. Defines the handler struct, the core interface(s) it depends on, and all RPC method implementations for that service.
- `internal/conv/` — all conversions between `pb` and `mdl` types.
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

## Testing

### Test harnesses

Use `ServerTest` for unit tests: the server runs with mocked cores, so tests exercise only the gRPC handler layer. Because inputs and outputs are fully controlled, this is the right place for exhaustive negative-case coverage — invalid arguments, permission errors, not-found responses, and other error paths.

Use `ServerIntegrationTest` for integration tests that must cross layer boundaries. It wires real cores against a real Postgres database and is slower, so keep integration tests focused on the golden path at a high level rather than exhaustive edge cases.

### File conventions

One test file per service (`auth_test.go`, `user_test.go`, …). When a test exercises multiple services — for example, logging in and then updating a display name — put it in `integration_test.go` instead.

### Auth helpers

`NewServerTest` always sets `testJWTKey`. Use `authCtxForTestUser(t, t.Context())` for calls to protected endpoints and plain `t.Context()` for methods listed in `publicMethods`.

## Validation

Validate request payloads at the handler level before calling into the core. Return `codes.InvalidArgument` with `errdetails.BadRequest` field violations so callers get actionable field-level feedback.

## Idempotency

`idempotencyUnaryInterceptor` (in `unary_interceptors.go`) lets a caller resume a dropped request by sending a `x-idempotency-key` header, which it turns into a DBOS workflow ID. The raw key is never used directly: it's hashed together with the authenticated user, the method, and the request payload before use, so two unrelated requests that happen to reuse the same key can never collide on the same workflow, and a caller can never receive another caller's cached result. This is also why `authUnaryInterceptor` must run before it in the chain — the derivation needs the authenticated user ID when one exists.

Streaming RPCs don't support this yet and silently ignore the header. The interceptor runs before any message is read off the stream, so there's no payload to bind the key to at that point — deriving an ID from just the user and method would reintroduce the collision problem the unary path exists to prevent. Server-streaming could support it by deferring derivation until the first `RecvMsg`, similar to how `loggingStream` wraps that call. Client-streaming and bidi have no single request to hash, so idempotent resumption doesn't map cleanly onto them regardless.
