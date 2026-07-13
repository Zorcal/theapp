# core

The application's core business logic. Other application-specific code that isn't part of the business core lives in sibling packages under `internal/`.

## Layout

- `mdl/` — domain types shared across layers. Includes output types (e.g. `User`) and operation input types (e.g. `CreateUser`, `UpdateUser`). Input types carry exactly the fields the caller supplies — never reuse an output type as an input.
- `pgstores/` — Postgres access, one package per store (e.g. `pguser`). Each store defines its own types.
- `<pkg>/` (e.g. `user/`) — business logic. Operates on `mdl` types and composes one or more `pgstores`.

## Dependency graph

```
        ┌───────┐
        │ <pkg> │
        └───┬───┘
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

Input types implement `Validate() error`. Core methods call it before touching a store.

## Update pattern

Update input types (e.g. `UpdateUser`) pair with a companion `UserUpdateFields` struct whose boolean fields declare which values should be written. A field absent from `UserUpdateFields` is left unchanged, regardless of its value in the input type.

This is preferred over pointer fields (`*string`) because a pointer cannot distinguish "leave this field alone" from "clear this field to its zero value" — they both map to `nil`.

The `pgstores` layer mirrors the same struct, and the dynamic SQL is built from the flags at query time.

## Conversions

Each core package (e.g. `user/`) owns a `conv.go` that covers all type conversions between `mdl` and `pgstores` types. Define one function per direction — never construct a foreign type inline in a core method:

```
mdl.CreateUser  →  pguser.CreateUser   (createUserToPG)
pguser.User     →  mdl.User            (userFromPG)
```

Without dedicated conv functions, type construction scatters and there is no single place to update when a type changes.

## Storer naming

Name storer interfaces with the domain as a prefix, e.g. `UserStorer`, `AuthStorer` — never a plain `Storer`, even when a core package interfaces out only one store. This contradicts the Go idiom of relying on the package qualifier (`user.Storer` vs `user.UserStorer`), but a core package might end up interfacing out more than one store, and adding a prefixed name later means renaming the original interface and every initialization of it.

## Cross-store transactions

When a core method must write through more than one store atomically, inject a `Transactor` interface and call `RunTx`. Define the interface in the core package so it doesn't import `pgdb`:

```go
type Transactor interface {
    RunTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```

Pass the enriched `ctx` from `RunTx` into each store call — stores that use `pgdb.RunBatchTx` internally will find the transaction already in context and join it rather than opening their own:

```go
func (c *Core) PlaceOrder(ctx context.Context, co mdl.CreateOrder) error {
    return c.tx.RunTx(ctx, func(ctx context.Context) error {
        if err := c.orders.InsertOrder(ctx, co); err != nil {
            return err
        }
        return c.inventory.DecrementStock(ctx, co.ProductID, co.Quantity)
    })
}
```

Wire up `pgdb.NewTransactor(pool)` at the composition root (`main.go`). `*pgdb.Transactor` satisfies the interface structurally.

Nesting is safe: if `RunTx` is called with a context that already carries a transaction, it reuses it and leaves commit/rollback to the outer caller.

## Testing

Each core package uses two complementary layers of tests:

- **Unit tests** — mock the storer interface(s) and cover the bulk of the logic: conversions, error wrapping, edge cases. Fast, no database required.
- **Integration tests** (named `Test<Core>_integration`) — use a real database via `pgtest` and cover the happy path end-to-end through the actual store. One or two per package is enough; their job is to catch wiring mistakes that mocks cannot, not to duplicate unit test coverage.

Every new (exported) method added to a `core/` package or a `pgstores/` package requires tests. For `pgstores/`, that means integration tests against a real database following the patterns in the existing `store_test.go` files.
