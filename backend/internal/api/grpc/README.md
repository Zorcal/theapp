# api/grpc

gRPC transport layer. Handlers receive protobuf requests, delegate to core interfaces, and return protobuf responses.

## Layout

- `server.go` — server construction and interceptor wiring.
- `<domain>.go` (e.g. `user.go`) — one file per domain. Defines the handler struct, the core interface it depends on, and all RPC method implementations for that domain.
- `internal/conv/` — all conversions between `pb` and `mdl` types.
- `internal/pb/` — generated protobuf code, do not edit by hand.

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

## Auth in tests

`NewServerTest` always sets `testJWTKey`. Use `srvTest.authCtx(t, ctx)` for calls to protected endpoints and plain `t.Context()` for methods listed in `publicMethods`.

## Validation

Validate request payloads at the handler level before calling into the core. Return `codes.InvalidArgument` with `errdetails.BadRequest` field violations so callers get actionable field-level feedback.
