# core

The application's core business logic. Other application-specific code that isn't part of the business core lives in sibling packages under `internal/`.

## Layout

- `mdl/` — domain types shared across layers. Includes output types (e.g. `User`) and operation input types (e.g. `CreateUser`, `UpdateUser`). Input types carry exactly the fields the caller supplies — never reuse an output type as an input.
- `pgstores/` — Postgres access, one package per store (e.g. `pguser`). Each store defines its own types.
- `<core>/` (e.g. `user/`) — business logic. Operates on `mdl` types and composes one or more `pgstores`.

## Dependency graph

```
        ┌────────┐
        │ <core> │
        └───┬────┘
            │
     ┌──────┴──────┐
     ▼             ▼
  ┌─────┐    ┌──────────┐
  │ mdl │    │ pgstores │
  └─────┘    └──────────┘
```

Edges are imports. `mdl` and `pgstores` are leaves — they import neither each other nor any core.

## mdl types

`mdl/` holds two kinds of types:

- **Output types** — what the core returns: `User`, `Order`, etc.
- **Input types** — what callers pass in for mutating operations: `CreateUser`, `UpdateUser`, etc. Name them after the operation. Never reuse an output type as an input; without a dedicated input type, adding a field later forces a signature change at every call site.

## Conversions

Each core domain package (e.g. `user/`) owns a `conv.go` that covers all type conversions between `mdl` and `pgstores` types. Define one function per direction — never construct a foreign type inline in a core method:

```
mdl.CreateUser  →  pguser.CreateUser   (createUserToPG)
pguser.User     →  mdl.User            (userFromPG)
```

Without dedicated conv functions, type construction scatters and there is no single place to update when a type changes.

## Testing

Each core domain package uses two complementary layers of tests:

- **Unit tests** — mock the `Storer` interface (via moq) and cover the bulk of the logic: conversions, error wrapping, edge cases. Fast, no database required.
- **Flow tests** — use a real database via `pgtest` and cover the happy path end-to-end through the actual store. One or two per domain is enough; their job is to catch wiring mistakes that mocks cannot, not to duplicate unit test coverage.
