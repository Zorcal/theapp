# backend

## Package structure

```
backend/
├── cmd/server/      # entry point — config, wiring, server startup
├── internal/        # app-specific packages
│   ├── api/         # transport layers (grpc/); transport types stay here and are converted to domain types at the boundary
│   ├── clients/     # third-party API wrappers (resend/, ...)
│   ├── core/        # business logic — see internal/core/README.md
│   ├── data/        # database infrastructure
│   └── <pkg>/       # other packages live here directly
└── pkg/             # general-purpose, app-agnostic packages
```

When a new package has no obvious home, put it directly under `internal/`. Group under a sub-directory only when there is a natural umbrella concept (as with `core/` or `clients/`). Each sub-directory with conventions has its own README.

Code belongs in `pkg/` only if it has no dependency on this application's domain and would make sense in any Go project.

## Local debugging

The gRPC server registers reflection when `THEAPP_ENVIRONMENT=local`, so these commands work without a `buf`-built descriptor set:

```
grpcurl -plaintext 127.0.0.1:5051 list
grpcurl -plaintext -d '{}' 127.0.0.1:5051 theapp.v1.<service>/<rpc>
```

For example, `grpcurl -plaintext -d '{}' 127.0.0.1:5051 theapp.v1.UserService/ListUsers`.

Reflection stays off elsewhere to avoid exposing the schema.
