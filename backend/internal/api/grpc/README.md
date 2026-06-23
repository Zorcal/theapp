# api/grpc

gRPC transport layer. Handlers receive protobuf requests, delegate to core interfaces, and return protobuf responses.

## Layout

- `server.go` — server construction and interceptor wiring.
- `<domain>.go` (e.g. `user.go`) — one file per domain. Defines the handler struct, the core interface it depends on, and all RPC method implementations for that domain.
- `internal/conv/` — all conversions between `pb` and `mdl` types.
- `internal/pb/` — generated protobuf code, do not edit by hand.
- `gateway/` — HTTP/JSON reverse proxy (grpc-gateway) and OpenAPI spec endpoint.

## Core interfaces

Each domain file defines a local interface (e.g. `UserCore`) that the handler depends on. This keeps the gRPC package decoupled from concrete core implementations and makes the handler testable with a mock.

Place the `//go:generate moq` directive on the same file that defines the interface.

## Conversions

All type conversions between `pb` and `mdl` belong in `internal/conv/<domain>.go`. Define one function per direction — never construct a foreign type inline in a handler:

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

Use `ServerIntegrationTest` for happy-path and flow tests that must cross layer boundaries — e.g. verifying that requesting a magic link actually delivers a token, or that a rotated refresh token cannot be reused. It wires real cores against a real Postgres database and is slower, so keep integration tests focused on the golden path rather than exhaustive edge cases.

### File conventions

One test file per domain (`auth_test.go`, `user_test.go`, …). When a test exercises multiple domains — for example, logging in and then updating a display name — put it in `integration_test.go` instead.

### Auth helpers

`NewServerTest` always sets `testJWTKey`. Use `authCtxForTestUser(t, t.Context())` for calls to protected endpoints and plain `t.Context()` for methods listed in `publicMethods`.

## Validation

Validate request payloads at the handler level before calling into the core. Return `codes.InvalidArgument` with `errdetails.BadRequest` field violations so callers get actionable field-level feedback.
