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

## Authentication

The API uses passwordless magic-link authentication. There are no passwords — a user proves ownership of their email address once per session.

### Token types

| Token         | Lifetime                | Purpose                                                           |
| ------------- | ----------------------- | ----------------------------------------------------------------- |
| Access token  | 15 min                  | Bearer token for protected API calls                              |
| Refresh token | 30 days (from last use) | Rotates to a new token pair; resets the 30-day window on each use |

### Client flow

**Sign in:**

1. `POST /v1/auth/magic-link` with `{"email": "..."}` — always returns success to avoid leaking whether the address is registered.
2. User clicks the link in their email; the frontend extracts the `token` query parameter.
3. `POST /v1/auth/verify` with `{"token": "..."}` — returns an `accessToken`, `refreshToken`, and `expiresIn`.

**Authenticated requests:**

- Set `Authorization: Bearer <accessToken>` on every call to a protected endpoint.

**Access token expired (after ~15 min):**

- Call `POST /v1/auth/refresh` with `{"refreshToken": "..."}` to get a new token pair without re-authenticating. The old refresh token is invalidated immediately.

**Forced re-login:**

- If the user does not refresh within 30 days of their last refresh, the refresh token expires and `POST /v1/auth/refresh` returns 401. The client must restart the magic-link flow.
- A user who refreshes at least once every 30 days never needs to re-authenticate.

**Sign out:**

- `POST /v1/auth/revoke` with `{"refreshToken": "..."}` to end the current session.
- `POST /v1/auth/sessions/revoke-all` (requires access token) to end all active sessions for the authenticated user.

### Local development

The magic-link email is not sent when no Resend API key is configured. Instead the token is printed to the server log — search for it there and pass it directly to `POST /v1/auth/verify`.

## Local debugging

The HTTP/JSON gateway runs on port 5052 and proxies all gRPC methods. Every endpoint from the gRPC server is available as a REST call — useful for quick manual testing with `curl` or a browser.

A Swagger UI is served at `http://127.0.0.1:5052/docs` for exploring and calling endpoints directly from the browser.

The gRPC server registers reflection when `THEAPP_ENVIRONMENT=local`, so the following commands work without a `buf`-built descriptor set:

```
grpcurl -plaintext 127.0.0.1:5051 list
grpcurl -plaintext -H "Authorization: Bearer <token>" -d '{}' 127.0.0.1:5051 theapp.v1.<service>/<rpc>
```

For example, `grpcurl -plaintext -H "Authorization: Bearer <token>" -d '{}' 127.0.0.1:5051 theapp.v1.UserService/ListUsers`.

Reflection stays off elsewhere to avoid exposing the schema.
