# core

The application's core business logic. Other application-specific code that isn't part of the business core lives in sibling packages under `internal/`.

## Layout

- `mdl/` — business models and their validation/behavior.
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
